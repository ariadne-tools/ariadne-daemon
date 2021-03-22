package prochandler

import (
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"

	"github.com/ariadne-tools/ariadne-daemon/internal/dbconnect"
	"github.com/ariadne-tools/ariadne-daemon/internal/logger"
	"github.com/rjeczalik/notify"
)

const UPDATE_BUFFER = 65536

type ProcHandler struct {
	DirId     int
	Watcheddb *dbconnect.DbConnector
	Filesdb   *dbconnect.DbConnector
	DoneID    chan int
}

func (ph *ProcHandler) Handle() {

	logger.DebugLog("procHandler -> process handling started for dir_id:", ph.DirId)

	c := make(chan notify.EventInfo, UPDATE_BUFFER)

	events := make([]notify.EventInfo, 0, UPDATE_BUFFER)
	var m sync.Mutex

	watchedDir := ph.getWatchedDir()
	if err := notify.Watch(path.Join(watchedDir, "..."), c, notify.All); err != nil {
		logger.InfoLog("WARNING: Handling file system events failed for the following dir: ", err)
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
			logger.DebugLog("procHandler.handle -> process handling done for dir_id:", ph.DirId)
			return
		case "updating":
			ph.update(&events, &m)
		}

	}
}

func (ph *ProcHandler) getState() string {

	stateSlice := ph.Watcheddb.QueryRow("SELECT state FROM watched_dirs_states WHERE id=?", ph.DirId)

	if state, ok := stateSlice[0].(string); ok != true {
		log.Fatal("getState -> cannot convert state to string")
		return ""
	} else {
		return state
	}
}

func (ph *ProcHandler) getWatchedDir() string {

	pathSlice := ph.Watcheddb.QueryRow("SELECT path_to_dir FROM watched_dirs_states WHERE id=?", ph.DirId)

	if path, ok := pathSlice[0].(string); ok != true {
		log.Fatal("getState -> cannot convert state to string")
		return ""
	} else {
		return path
	}
}

func (ph *ProcHandler) index() {

	logger.DebugLog("index -> indexing started for dir_id", ph.DirId)

	// Remove from the db the rows whom files does not exist
	rows := ph.Filesdb.Query("SELECT path_to_file, fname FROM files WHERE dir_id=?", ph.DirId)

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
				logger.DebugLog("index", ph.DirId, "-> removing not existing file's index", path, fname)
				ph.Filesdb.Exec("DELETE FROM files WHERE path_to_file=? AND fname=?", path, fname)
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
					logger.InfoLog("index -> ", err)
					return nil
				} else {
					dir, file := filepath.Split(path)
					size := info.Size()
					mtimens := info.ModTime().UnixNano()
					isDir := info.IsDir()

					logger.DebugLog("index", ph.DirId, "-> file inserted/updated: ", dir, file)
					ph.Filesdb.Exec("INSERT into files (dir_id, path_to_file, fname, size, mtime_ns, is_dir) VALUES (?,?,?,?,?,?)"+
						"ON CONFLICT(path_to_file, fname) DO UPDATE SET size = ?, mtime_ns = ?",
						ph.DirId, dir, file, size, mtimens, isDir, size, mtimens)

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
		ph.Watcheddb.Exec("UPDATE watched_dirs SET state_id=? WHERE id=?", 3, ph.DirId)
	}
	logger.DebugLog("index -> indexing done for dir_id", ph.DirId)
}

func (ph *ProcHandler) wipe() {
	logger.DebugLog("wipe -> wipe for ", ph.DirId, " id started")
	ph.Filesdb.Exec("DELETE FROM files WHERE dir_id=?", ph.DirId)
	ph.DoneID <- ph.DirId
	ph.Watcheddb.Exec("DELETE FROM watched_dirs WHERE id =?", ph.DirId)
	logger.DebugLog("wipe -> wipe for id", ph.DirId, "done")
}

func (ph *ProcHandler) update(events *[]notify.EventInfo, m *sync.Mutex) {

	m.Lock()
	if len(*events) > 0 {
		event := (*events)[0]
		*events = (*events)[1:]
		m.Unlock()
		logger.DebugLog("update -> the following event handled: ", event)
		path, fname := filepath.Split(event.Path())

		switch event.Event() {
		case notify.Remove:
			ph.Filesdb.Exec("DELETE FROM files WHERE path_to_file=? AND fname=?", path, fname)
		default:
			if fileStat, err := os.Stat(event.Path()); err != nil {
				// the file was deleted or it's permission changed since event was recorded
				ph.Filesdb.Exec("DELETE FROM files WHERE path_to_file=? AND fname=?", path, fname)
			} else {
				size := fileStat.Size()
				mtimens := fileStat.ModTime().UnixNano()
				isDir := fileStat.IsDir()

				ph.Filesdb.Exec("INSERT into files (dir_id, path_to_file, fname, size, mtime_ns, is_dir) VALUES (?,?,?,?,?,?)"+
					"ON CONFLICT(path_to_file, fname) DO UPDATE SET size = ?, mtime_ns = ?",
					ph.DirId, path, fname, size, mtimens, isDir, size, mtimens)
			}
		}
	} else {
		m.Unlock()
		time.Sleep(20 * time.Millisecond)
	}

}
