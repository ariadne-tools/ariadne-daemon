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
		"OFF":   0b00000000,
		"FATAL": 0b00000001,
		"ERROR": 0b00000010,
		"WARN":  0b00000100,
		"INFO":  0b00001000,
		"DEBUG": 0b00010000,
		"TRACE": 0b00100000,
		"ALL":   0b00111111,
	}
)

func levelGiven(s string) LogLevel {
	f := func(c rune) bool {
		return c == '|'
	}
	fields := strings.FieldsFunc(strings.ToUpper(s), f)
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
	loglevel = levelGiven(s)
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

func FileExists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		return !os.IsNotExist(err)
	}
	return true
}

func CreateFile(name string) error {
	if f, err := os.Create(name); err != nil {
		return err
	} else {
		f.Close()
		return nil
	}
}

func loggerLog(level LogLevel, args ...interface{}) {
	if level&loglevel == level {
		lgr.Println(args...)
	}
}

func BasicLog(args ...interface{}) {
	loggerLog(LOGLEVEL["INFO"], args...)
}

func DebugLog(args ...interface{}) {
	loggerLog(LOGLEVEL["DEBUG"], args...)
}
