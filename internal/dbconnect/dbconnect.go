package dbconnect

import (
	"database/sql"
	"log"
	"sync"
	"time"

	"github.com/ariadne-tools/ariadne-daemon/internal/logger"
	"github.com/ariadne-tools/ariadne-daemon/internal/terminator"
	_ "github.com/mattn/go-sqlite3"
)

type Query struct {
	Base string
	Args []interface{}
}

type DbConnector struct {
	sync.Mutex
	DB     *sql.DB
	update time.Duration
	qry    chan Query
	wg     *sync.WaitGroup
}

func NewDbConnector(filename string, updatePeriod time.Duration, wg *sync.WaitGroup) *DbConnector {

	dbConn, _ := sql.Open("sqlite3", filename)
	qry := make(chan Query)

	conn := DbConnector{DB: dbConn, update: updatePeriod, qry: qry, wg: wg}
	if updatePeriod != 0 {
		go conn.__dbWriterPeriodic()
		wg.Add(1)
	}

	return &conn
}

// TODO: ezt szebben, errorokkal?
func (conn *DbConnector) Query(query string, args ...interface{}) [][]interface{} {

	res := [][]interface{}{}
	conn.Lock()
	if rows, err := conn.DB.Query(query, args...); err != nil {
		log.Fatal("query -> ", conn.DB, ", ", query, ", ", args, ", ", err)
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
func (conn *DbConnector) QueryRow(query string, args ...interface{}) []interface{} {
	r := conn.Query(query, args...)
	if len(r) == 0 {
		return []interface{}{}
	} else if len(r) == 1 {
		return r[0]
	} else {
		log.Fatal("dbConnector.queryRow -> got more than one row, expected at most one")
		return []interface{}{}
	}
}

func (conn *DbConnector) Exec(q string, args ...interface{}) {

	qry := Query{q, args}

	if conn.update == 0 {
		conn.__dbWriterInstant(qry)
	} else {
		conn.qry <- qry
	}
}

// this function should not be called directly
func (conn *DbConnector) __dbWriterPeriodic() {

	commitSignal := make(chan struct{})

	go func() {
		for range time.Tick(conn.update) {
			commitSignal <- struct{}{}
		}
	}()

	for {
		tx, errBeg := conn.DB.Begin()
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
				logger.DebugLog("__dbWriterPeriodic -> starting commit")
				if errComm := tx.Commit(); errComm != nil {
					log.Fatal("__dbWriterPeriodic -> commit error:", errComm)
				} else {
					logger.DebugLog("__dbWriterPeriodic -> commit done")
					conn.Unlock()
					break TransactionLoop
				}
			case <-terminator.StopSig:
				conn.Lock()
				if errComm := tx.Commit(); errComm != nil {
					log.Fatal("__dbWriterPeriodic -> commit error:", errComm)
				} else {
					logger.DebugLog("__dbWriterPeriodic -> everything's committed successfully, exiting...")
					conn.Unlock()
					conn.wg.Done()
					return
				}
			}
		}
	}
}

// this function should not be called directly
func (conn *DbConnector) __dbWriterInstant(qry Query) {

	conn.Lock()
	if _, err := conn.DB.Exec(qry.Base, qry.Args...); err != nil {
		log.Fatal("__dbWriterInstant -> ", err)
	}
	conn.Unlock()
}

func WatchedDirs(db *DbConnector) map[int]string {

	watched := make(map[int]string)

	dirs := db.Query("SELECT id, path_to_dir FROM watched_dirs_states")
	for _, dir := range dirs {
		id := int(dir[0].(int64))
		path := dir[1].(string)
		watched[id] = path
	}
	return watched
}

func WatchedIds(db *DbConnector) map[int]struct{} {

	m := make(map[int]struct{})

	for id := range WatchedDirs(db) {
		m[id] = struct{}{}
	}
	logger.DebugLog("watchedIds -> the following ids have to be handled:", m)
	return m
}
