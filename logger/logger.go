package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"sync"
)

const logFileName = "g2i.log"

var (
	// logger is a single local logger implementation
	logger *log.Logger
	once   sync.Once
)

// initLogger sets up a new instance of logger that writes to file and STDOUT
func initLogger() {
	once.Do(func() {
		pwd, err := os.Getwd()
		if err != nil {
			panic(err)
		}
		file, err := os.OpenFile(logFileName, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
		if err != nil {
			panic(fmt.Errorf("Cannot create log file at %s/%s Error: %v", pwd, logFileName, err))
		}
		w := io.MultiWriter(os.Stdout, file)
		logger = log.New(w, "", log.Ldate|log.Ltime|log.LUTC)
	})
}

// GetLogger returns instance of application logger
func GetLogger() *log.Logger {
	return logger
}

func init() {
	initLogger()
}
