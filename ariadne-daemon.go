package main

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/rpc"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/rjeczalik/notify"
)

var debugLog bool = true
var stopSig chan struct{} = make(chan struct{}, STOPSIG_SIZE)

const (
	VERSION       = "0.3.1"
	FILESDB       = "files.db"
	WATCHEDDIRSDB = "watched_dirs.db"
	COMMIT_FREQ   = 2 * time.Second
	UPDATE_BUFFER = 65536
	STOPSIG_SIZE  = 65536
)

type Query struct {
	Base string
	Args []interface{}
}

func DebugLog(args ...interface{}) {
	if debugLog {
		log.Println(args...)
	}
}

type dbConnector struct {
	sync.Mutex
	db     *sql.DB
	update time.Duration
	qry    chan Query
	wg     *sync.WaitGroup
}

func newDbConnector(filename string, updatePeriod time.Duration, wg *sync.WaitGroup) *dbConnector {

	dbConn, _ := sql.Open("sqlite3", filename)
	qry := make(chan Query)

	conn := dbConnector{db: dbConn, update: updatePeriod, qry: qry, wg: wg}
	if updatePeriod != 0 {
		go conn.__dbWriterPeriodic()
		wg.Add(1)
	}

	return &conn
}

// TODO: ezt szebben, errorokkal?
func (conn *dbConnector) query(query string, args ...interface{}) [][]interface{} {

	res := [][]interface{}{}
	conn.Lock()
	if rows, err := conn.db.Query(query, args...); err != nil {
		log.Fatal("query -> ", conn.db, ", ", query, ", ", args, ", ", err)
	} else {
		defer rows.Close()
		cols, _ := rows.Columns() // don't have to check for error, 'cos rows are not closed per se

		for rows.Next() {
			r := make([]interface{}, len(cols))
			rp := make([]interface{}, len(cols))

			for i := range r {
				rp[i] = &r[i]
			}

			err := rows.Scan(rp...)
			if err != nil {
				log.Fatal("query -> ", err)
			} else {
				res = append(res, r)
			}
		}
	}

	conn.Unlock()
	return res
}

// TODO: ezt szebben, errorokkal?
func (conn *dbConnector) queryRow(query string, args ...interface{}) []interface{} {
	r := conn.query(query, args...)
	if len(r) == 0 {
		return []interface{}{}
	} else if len(r) == 1 {
		return r[0]
	} else {
		log.Fatal("dbConnector.queryRow -> got more than one row, expected at most one")
		return []interface{}{}
	}
}

func (conn *dbConnector) exec(q string, args ...interface{}) {

	qry := Query{q, args}

	if conn.update == 0 {
		conn.__dbWriterInstant(qry)
	} else {
		conn.qry <- qry
	}
}

// this function should not be called directly
func (conn *dbConnector) __dbWriterPeriodic() {

	commitSignal := make(chan struct{})

	go func() {
		for _ = range time.Tick(conn.update) {
			commitSignal <- struct{}{}
		}
	}()

	for {
		tx, errBeg := conn.db.Begin()
		if errBeg != nil {
			log.Fatal("__dbWriterPeriodic -> ", errBeg)
		}
		defer tx.Rollback() // The rollback will be ignored if the tx has been committed later in the function.

	TransactionLoop:
		for {
			select {
			case q := <-conn.qry:
				if _, err := tx.Exec(q.Base, q.Args...); err != nil {
					log.Fatal("__dbWriterPeriodic", err)
				}
			case <-commitSignal:
				conn.Lock()
				DebugLog("__dbWriterPeriodic -> starting commit")
				if errComm := tx.Commit(); errComm != nil {
					log.Fatal("__dbWriterPeriodic -> commit error:", errComm)
				} else {
					DebugLog("__dbWriterPeriodic -> commit done")
					conn.Unlock()
					break TransactionLoop
				}
			case <-stopSig:
				conn.Lock()
				if errComm := tx.Commit(); errComm != nil {
					log.Fatal("__dbWriterPeriodic -> commit error:", errComm)
				} else {
					DebugLog("__dbWriterPeriodic -> everything's committed successfully, exiting...")
					conn.Unlock()
					conn.wg.Done()
					return
				}
			}
		}
	}
}

// this function should not be called directly
func (conn *dbConnector) __dbWriterInstant(qry Query) {

	conn.Lock()
	if _, err := conn.db.Exec(qry.Base, qry.Args...); err != nil {
		log.Fatal("__dbWriterInstant -> ", err)
	}
	conn.Unlock()
}

func watchedDirs(db *dbConnector) map[int]string {

	watched := make(map[int]string)

	dirs := db.query("SELECT id, path_to_dir FROM watched_dirs_states")
	for _, dir := range dirs {
		id := int(dir[0].(int64))
		path := dir[1].(string)
		watched[id] = path
	}
	return watched
}

func watchedIds(db *dbConnector) map[int]struct{} {

	m := make(map[int]struct{})

	for id, _ := range watchedDirs(db) {
		m[id] = struct{}{}
	}
	DebugLog("watchedIds -> the following ids have to be handled:", m)
	return m
}

func isValidDir(path string) bool {
	if info, err := os.Stat(path); err != nil {
		return false
	} else {
		return info.IsDir()
	}
}

type procHandler struct {
	dirId     int
	watcheddb *dbConnector
	filesdb   *dbConnector
	doneID    chan int
}

func (ph *procHandler) handle() {

	DebugLog("procHandler -> process handling started for dir_id:", ph.dirId)

	c := make(chan notify.EventInfo, UPDATE_BUFFER)

	events := make([]notify.EventInfo, 0, UPDATE_BUFFER)
	var m sync.Mutex

	watchedDir := ph.getWatchedDir()
	if err := notify.Watch(path.Join(watchedDir, "..."), c, notify.All); err != nil {
		log.Println("WARNING: Handling file system events failed for the following dir: ", err)
	} else {
		defer notify.Stop(c)
	}
	// start of gathering file system events
	go func() {
		for {
			if len(c) == cap(c) {
				log.Fatal("procHandler -> Buffer is full for ", watchedDir)
			} else {
				ei := <-c
				m.Lock()
				events = append(events, ei)
				m.Unlock()
			}
		}
	}()

	for {
		switch ph.getState() {
		case "indexing":
			ph.index()
		case "wiping":
			ph.wipe()
			DebugLog("procHandler.handle -> process handling done for dir_id:", ph.dirId)
			return
		case "updating":
			ph.update(&events, &m)
		}

	}
}

func (ph *procHandler) getState() string {

	stateSlice := ph.watcheddb.queryRow("SELECT state FROM watched_dirs_states WHERE id=?", ph.dirId)

	if state, ok := stateSlice[0].(string); ok != true {
		log.Fatal("getState -> cannot convert state to string")
		return ""
	} else {
		return state
	}
}

func (ph *procHandler) getWatchedDir() string {

	pathSlice := ph.watcheddb.queryRow("SELECT path_to_dir FROM watched_dirs_states WHERE id=?", ph.dirId)

	if path, ok := pathSlice[0].(string); ok != true {
		log.Fatal("getState -> cannot convert state to string")
		return ""
	} else {
		return path
	}
}

func (ph *procHandler) index() {

	DebugLog("index -> indexing started for dir_id", ph.dirId)

	// Remove from the db the rows whom files does not exist
	rows := ph.filesdb.query("SELECT path_to_file, fname FROM files WHERE dir_id=?", ph.dirId)

	for _, row := range rows {
		switch ph.getState() {
		case "wiping":
			return
		default:
			var path, fname string

			if p, ok := row[0].(string); ok != true {
				log.Fatal("index -> cannot convert path_to_file to string")
			} else {
				path = p
			}

			if f, ok := row[1].(string); ok != true {
				log.Fatal("index -> cannot convert fname to string")
			} else {
				fname = f
			}

			if _, err := os.Stat(filepath.Join(path, fname)); os.IsNotExist(err) {
				DebugLog("index", ph.dirId, "-> removing not existing file's index", path, fname)
				ph.filesdb.exec("DELETE FROM files WHERE path_to_file=? AND fname=?", path, fname)
			}
		}
	}

	// Walk recursively on filepath, and insert/update files found
	watchedRoot := ph.getWatchedDir()
	err := filepath.Walk(watchedRoot,
		func(path string, info os.FileInfo, err error) error {
			switch ph.getState() {
			case "wiping":
				return io.EOF
			default:
				if err != nil {
					log.Println("index -> ", err)
					return nil
				} else {
					dir, file := filepath.Split(path)
					size := info.Size()
					mtimens := info.ModTime().UnixNano()
					isDir := info.IsDir()

					DebugLog("index", ph.dirId, "-> file inserted/updated: ", dir, file)
					ph.filesdb.exec("INSERT into files (dir_id, path_to_file, fname, size, mtime_ns, is_dir) VALUES (?,?,?,?,?,?)"+
						"ON CONFLICT(path_to_file, fname) DO UPDATE SET size = ?, mtime_ns = ?",
						ph.dirId, dir, file, size, mtimens, isDir, size, mtimens)

					return nil
				}
			}
		})

	if err == io.EOF {
		// the directory marked for 'wiping'
		return
	} else if err != nil {
		log.Fatal("index -> ", err)
	}

	if state := ph.getState(); state == "indexing" {
		//TODO: 3-at lecserelni
		ph.watcheddb.exec("UPDATE watched_dirs SET state_id=? WHERE id=?", 3, ph.dirId)
	}
	DebugLog("index -> indexing done for dir_id", ph.dirId)
}

func (ph *procHandler) wipe() {
	DebugLog("wipe -> wipe for ", ph.dirId, " id started")
	ph.filesdb.exec("DELETE FROM files WHERE dir_id=?", ph.dirId)
	ph.doneID <- ph.dirId
	ph.watcheddb.exec("DELETE FROM watched_dirs WHERE id =?", ph.dirId)
	DebugLog("wipe -> wipe for id", ph.dirId, "done")
}

func (ph *procHandler) update(events *[]notify.EventInfo, m *sync.Mutex) {

	m.Lock()
	if len(*events) > 0 {
		event := (*events)[0]
		*events = (*events)[1:]
		m.Unlock()
		DebugLog("update -> the following event handled: ", event)
		path, fname := filepath.Split(event.Path())

		switch event.Event() {
		case notify.Remove:
			ph.filesdb.exec("DELETE FROM files WHERE path_to_file=? AND fname=?", path, fname)
		default:
			if fileStat, err := os.Stat(event.Path()); err != nil {
				// the file was deleted or it's permission changed since event was recorded
				ph.filesdb.exec("DELETE FROM files WHERE path_to_file=? AND fname=?", path, fname)
			} else {
				size := fileStat.Size()
				mtimens := fileStat.ModTime().UnixNano()
				isDir := fileStat.IsDir()

				ph.filesdb.exec("INSERT into files (dir_id, path_to_file, fname, size, mtime_ns, is_dir) VALUES (?,?,?,?,?,?)"+
					"ON CONFLICT(path_to_file, fname) DO UPDATE SET size = ?, mtime_ns = ?",
					ph.dirId, path, fname, size, mtimens, isDir, size, mtimens)
			}
		}
	} else {
		m.Unlock()
		time.Sleep(20 * time.Millisecond)
	}

}

func procHandlerGenerator(watcheddb *dbConnector, filesdb *dbConnector, wg *sync.WaitGroup) {
	wg.Add(1)
	defer wg.Done()

	handled_ids := make(map[int]struct{})
	doneID := make(chan int)

	for _ = range time.Tick(1 * time.Second) {
		select {
		case <-stopSig:
			DebugLog("procHandlerGenerator -> exiting...")
			return
		case id := <-doneID:
			delete(handled_ids, id)
			DebugLog("processTracker -> process with id", id, "removed:", handled_ids)
		default:
			for dir_id, dirPath := range watchedDirs(watcheddb) {
				if isValidDir(dirPath) {
					// if this directory wasn't handled before
					if _, in := handled_ids[dir_id]; !in {
						DebugLog("procHandlerGenerator -> new procHandler created with id:", dir_id)
						handled_ids[dir_id] = struct{}{}
						ph := procHandler{dir_id, watcheddb, filesdb, doneID}
						go ph.handle()
					}
				} else {
					// the dir was deleted since last run, so delete it from the dbs as well
					log.Printf("WARNING: The directory '%s' disappeared since last run, so now it's removed from Ariadne as well!\n", dirPath)
					watcheddb.exec("UPDATE watched_dirs SET state_id=? WHERE id=?", 2, dir_id)
				}
			}
		}
	}
}

func terminator() {

	for i := 0; i != STOPSIG_SIZE; i++ {
		stopSig <- struct{}{}
	}
	return

}

type RemoteCall struct {
	watcheddb *dbConnector
	filesdb   *dbConnector
}

type FileProperties struct {
	Path_to_file string
	Fname        string
	Size         int
	Mtime_ns     int
	IsDir        bool
}

func (r RemoteCall) Search(searchString string, files *[]FileProperties) error {
	rows := r.filesdb.query("SELECT path_to_file,fname,size,mtime_ns,is_dir FROM files WHERE fname LIKE '%'||?||'%'", searchString)
	for _, row := range rows {
		path_to_file := row[0].(string)
		fname := row[1].(string)
		size := int(row[2].(int64))
		mtime_ns := int(row[3].(int64))

		is_dir_int := row[4].(int64)
		is_dir := is_dir_int != 0

		*files = append(*files, FileProperties{path_to_file, fname, size, mtime_ns, is_dir})
	}
	return nil
}

func (r RemoteCall) StopDaemon(x struct{}, y *struct{}) error {
	terminator()
	return nil
}

func (r RemoteCall) Add(dirpaths []string, added *[]string) error {

	q := r.watcheddb.query("SELECT path_to_dir FROM watched_dirs")
	dirsAdded := make([]string, 0)

	for _, v := range q {
		dirsAdded = append(dirsAdded, v[0].(string))
	}

	for _, dirpath := range dirpaths {

		alreadyAdded := false
		for _, added := range dirsAdded {
			if added == dirpath {
				alreadyAdded = true
			}
		}
		if !alreadyAdded {
			r.watcheddb.exec("INSERT into watched_dirs (path_to_dir, state_id) VALUES (?,?)", dirpath, 1)
			*added = append(*added, dirpath)
		}
	}
	return nil
}

func (r RemoteCall) Remove(dirIds []int, removed *[]int) error {
	watched := watchedIds(r.watcheddb)
	for _, dirId := range dirIds {
		if _, in := watched[dirId]; in == true {
			r.watcheddb.exec("UPDATE watched_dirs SET state_id=? WHERE id=?", 2, dirId) //TODO: 2-t cserelni
			*removed = append(*removed, dirId)
		}
	}
	return nil
}

type WatchedDirsState struct {
	Id    int
	Path  string
	State string
}

func (r RemoteCall) WatchedDirs(_ struct{}, watched *[]WatchedDirsState) error {
	dirs := r.watcheddb.query("SELECT * FROM watched_dirs_states")
	for _, dir := range dirs {
		id := int(dir[0].(int64))
		path := dir[1].(string)
		state := dir[2].(string)
		*watched = append(*watched, WatchedDirsState{id, path, state})
	}
	return nil
}

func main() {

	fmt.Printf("Welcome to Ariadne daemon version %s!\n", VERSION)

	// set dir to the path of the executable
	var dir string
	if ex, err := os.Executable(); err != nil {
		log.Fatal(err)
	} else {
		dir = path.Dir(ex)
	}

	wg := new(sync.WaitGroup)

	filesDbConn := newDbConnector(path.Join(dir, FILESDB), COMMIT_FREQ, wg)
	watchedDbConn := newDbConnector(path.Join(dir, WATCHEDDIRSDB), 0, wg)
	defer filesDbConn.db.Close()
	defer watchedDbConn.db.Close()

	// set all the dirs for full index
	watchedDbConn.exec("UPDATE watched_dirs SET state_id=?", 1)

	// setting up rpc
	remoteFiles := RemoteCall{watcheddb: watchedDbConn, filesdb: filesDbConn}
	rpc.Register(remoteFiles)
	rpc.HandleHTTP()
	http.HandleFunc("/", func(res http.ResponseWriter, req *http.Request) {
		io.WriteString(res, "Ariadne's RPC server live!")
	})
	go http.ListenAndServe(":9000", nil)

	go procHandlerGenerator(watchedDbConn, filesDbConn, wg)

	wg.Wait()
	DebugLog("main -> Daemon exiting, bye!")
}
