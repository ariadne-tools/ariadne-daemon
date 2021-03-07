package logger

import "log"

var debugLog bool = true

func DebugLog(args ...interface{}) {
	if debugLog {
		log.Println(args...)
	}
}
