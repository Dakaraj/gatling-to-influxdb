package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
)

var (
	// logger is a single local logger implementation
	logger *log.Logger
	once   sync.Once
)

// InitLogger sets up a new instance of logger that writes to file and STDOUT
func InitLogger(fileName string) error {
	// pwd, err := os.Getwd()
	// if err != nil {
	// 	return fmt.Errorf("Failed to get current directory: %w", err)
	// }
	p, err := filepath.Abs(fileName)
	if err != nil {
		return fmt.Errorf("Failed to build absolute path for log file: %w", err)
	}
	dir := filepath.Dir(p)
	err = os.MkdirAll(dir, 0755)
	if err != nil {
		return fmt.Errorf("Failed to create log dirrectory: %w", err)
	}

	file, err := os.OpenFile(fileName, os.O_WRONLY|os.O_CREATE|os.O_APPEND|os.O_SYNC, 0644)
	if err != nil {
		return fmt.Errorf("Cannot create log file at %s: %w", fileName, err)
	}
	w := io.MultiWriter(os.Stdout, file)
	logger = log.New(w, "", log.Ldate|log.Ltime|log.LUTC)

	return nil
}

// GetLogger returns instance of application logger
func GetLogger() *log.Logger {
	return logger
}
