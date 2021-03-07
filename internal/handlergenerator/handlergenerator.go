package handlergenerator

import (
	"log"
	"os"
	"sync"
	"time"

	"github.com/ariadne-tools/ariadne-daemon/internal/dbconnect"
	"github.com/ariadne-tools/ariadne-daemon/internal/logger"
	"github.com/ariadne-tools/ariadne-daemon/internal/prochandler"
	"github.com/ariadne-tools/ariadne-daemon/internal/terminator"
)

func isValidDir(path string) bool {
	if info, err := os.Stat(path); err != nil {
		return false
	} else {
		return info.IsDir()
	}
}

// ProcHandlerGenerator a comment...
func ProcHandlerGenerator(watcheddb *dbconnect.DbConnector, filesdb *dbconnect.DbConnector, wg *sync.WaitGroup) {
	wg.Add(1)
	defer wg.Done()

	handledIds := make(map[int]struct{})
	doneID := make(chan int)

	for range time.Tick(1 * time.Second) {
		select {
		case <-terminator.StopSig:
			logger.DebugLog("procHandlerGenerator -> exiting...")
			return
		case id := <-doneID:
			delete(handledIds, id)
			logger.DebugLog("processTracker -> process with id", id, "removed:", handledIds)
		default:
			for dirID, dirPath := range dbconnect.WatchedDirs(watcheddb) {
				if isValidDir(dirPath) {
					// if this directory wasn't handled before
					if _, in := handledIds[dirID]; !in {
						logger.DebugLog("procHandlerGenerator -> new procHandler created with id:", dirID)
						handledIds[dirID] = struct{}{}
						ph := prochandler.ProcHandler{dirID, watcheddb, filesdb, doneID}
						go ph.Handle()
					}
				} else {
					// the dir was deleted since last run, so delete it from the dbs as well
					log.Printf("WARNING: The directory '%s' disappeared since last run, so now it's removed from Ariadne as well!\n", dirPath)
					watcheddb.Exec("UPDATE watched_dirs SET state_id=? WHERE id=?", 2, dirID)
				}
			}
		}
	}
}
