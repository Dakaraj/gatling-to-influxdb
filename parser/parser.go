package parser

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/dakaraj/gatling-to-influxdb/logger"
	"github.com/spf13/cobra"
)

const (
	// defines how long will file parser wait for new line before
	// stopping process (minutes)
	waitTime = 5
)

var (
	l     = logger.GetLogger()
	re    = regexp.MustCompile(`^.+?-(\d{14})\d{3}$`)
	start = time.Now().Unix()

	errFound = errors.New("Found")
	logDir   string
)

func lookupTargetDir(dir string) error {
	for {
		finfo, err := os.Stat(dir)
		if err != nil && !os.IsNotExist(err) {
			return err
		} else if os.IsNotExist(err) {
			time.Sleep(time.Second * 5)
			continue
		}

		if !finfo.IsDir() {
			return fmt.Errorf("Was expecting directory at %s, but found a file", dir)
		}

		abs, _ := filepath.Abs(dir)
		l.Printf("Target directory found at %s", abs)
		break
	}

	return nil
}

func walkFunc(path string, info os.FileInfo, err error) error {
	if info.IsDir() && re.MatchString(path) {
		dateString := re.FindStringSubmatch(path)[1]
		t, _ := time.Parse("20060102150405", dateString)
		if t.Unix() > start {
			logDir = path
			abs, _ := filepath.Abs(logDir)
			l.Printf("Found log directory at %s", abs)
			return errFound
		}
	}
	if info.IsDir() && !re.MatchString(path) && info.Name() != "gatling" {
		return filepath.SkipDir
	}

	return nil
}

// logic is the following: at the start of the script we log a time
// then search for all directories, parse their names and compare
// unix seconds from names with the initial value, and stop when
// higher value is found, marking it as a target
func lookupResultsDir(dir string) error {
	l.Println("Searching for results directory...")
	for {
		err := filepath.Walk(dir, walkFunc)
		if err == errFound {
			break
		}
		if err != nil && err != errFound {
			return err
		}
		time.Sleep(time.Second * 5)
	}

	return nil
}

func waitForLog() error {
	for {
		finfo, err := os.Stat(logDir + "/simulation.log")
		if err != nil && !os.IsNotExist(err) {
			return err
		} else if os.IsNotExist(err) {
			time.Sleep(time.Second * 5)
			continue
		}

		if finfo.Mode().IsRegular() && finfo.Mode().Perm() == 420 {
			l.Printf("Found simulation.log at %s", logDir)
			break
		}

		return errors.New("Something wrong happened when attempting to open simulation.log")
	}

	return nil
}

func parseLoop(file *os.File) error {
	r := bufio.NewReader(file)
	var buf []byte
	startWait := time.Now()
	for {
		b, err := r.ReadBytes('\n')
		if err == io.EOF {
			if time.Now().After(startWait.Add(time.Duration(waitTime) * time.Minute)) {
				l.Printf("No new lines found for %d minutes. Stopping application...", waitTime)
				break
			}
			buf = append(buf, b...)
			// l.Printf("Encountered EOF. Waiting for more data\n")
			time.Sleep(time.Second * 2)
			continue
		}
		if err != nil && err != io.EOF {
			return fmt.Errorf("Unexpected error encountered while parsing file: %v", err)
		}
		buf = append(buf, b...)
		// TODO: STRING PROCESSING HERE
		buf = []byte{}
		startWait = time.Now()
	}

	return nil
}

func parseStart() error {
	file, err := os.Open(logDir + "/simulation.log")
	if err != nil {
		return err
	}
	defer file.Close()

	if err = parseLoop(file); err != nil {
		return err
	}

	return nil
}

// RunMain performs main application logic
func RunMain(cmd *cobra.Command, args []string) {
	l.Printf("Searching for gatling directory at %s", args[0])
	dir := args[0] + "/gatling"
	if err := lookupTargetDir(dir); err != nil {
		l.Fatalf("Target directory lookup failed with error: %v\n", err)
	}
	if err := lookupResultsDir(dir); err != nil {
		l.Fatalf("Error happened while searching for results directory: %v\n", err)
	}
	if err := waitForLog(); err != nil {
		l.Fatalf("Failed waiting for simulation.log with error: %v\n", err)
	}
	if err := parseStart(); err != nil {
		l.Fatalf("Failed reading simulation.log: %v\n", err)
	}
}
