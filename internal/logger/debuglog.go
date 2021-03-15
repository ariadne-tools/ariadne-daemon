package logger

import "log"

var LogLevel int = 0

func DebugLog(args ...interface{}) {
	if LogLevel == 1 {
		log.Println(args...)
	}
}
