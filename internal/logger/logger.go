package logger

import (
	"log"
	"os"
	"strings"
)

type LogLevel int

var (
	loglevel LogLevel
	logfile  *os.File    = os.Stdout
	lgr      *log.Logger = log.New(logfile, "", log.LstdFlags)

	LOGLEVEL = map[string]LogLevel{
		"off":   0b00000000,
		"fatal": 0b00000001,
		"error": 0b00000010,
		"warn":  0b00000100,
		"info":  0b00001000,
		"debug": 0b00010000,
		"trace": 0b00100000,
		"all":   0b00111111,
	}
)

func levelArg(s string) LogLevel {
	f := func(c rune) bool {
		return c == '|'
	}
	fields := strings.FieldsFunc(strings.ToLower(s), f)
	var level LogLevel

	for _, field := range fields {
		if l, in := LOGLEVEL[field]; !in {
			log.Fatal("Bad log level given: ", field)
		} else {
			level |= l
		}
	}

	return level
}

func Init(filepath string, s string) {
	loglevel = levelArg(s)
	if filepath != "" {
		var err error
		// If the file doesn't exist, create it, or append to existed
		if logfile, err = os.OpenFile(filepath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err != nil {
			log.Fatal(err)
		} else {
			lgr.SetOutput(logfile)
		}
	}
}

func loggerLog(levelName string, args ...interface{}) {
	if LOGLEVEL[levelName]&loglevel != 0 {
		l := []interface{}{"[" + levelName + "]"}
		for _, a := range args {
			l = append(l, a)
		}
		lgr.Println(l...)
	}
}

func InfoLog(args ...interface{}) {
	loggerLog("info", args...)
}

func DebugLog(args ...interface{}) {
	loggerLog("debug", args...)
}
