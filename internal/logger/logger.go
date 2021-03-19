package logger

import (
	"log"
	"os"
)

var (
	verbosity int
	logfile   *os.File    = os.Stdout
	lgr       *log.Logger = log.New(logfile, "", log.LstdFlags)
)

func Init(filepath string, v int) {
	verbosity = v
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

func BasicLog(args ...interface{}) {
	if verbosity >= 0 {
		lgr.Println(args...)
	}
}

func DebugLog(args ...interface{}) {
	if verbosity >= 1 {
		lgr.Println(args...)
	}
}
