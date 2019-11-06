package parser

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/dakaraj/gatling-to-influxdb/logger"
	"github.com/spf13/cobra"
)

var (
	l     = logger.GetLogger()
	re    = regexp.MustCompile(`.+?-(\d{14})`)
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

// RunMain performs main application logic
func RunMain(cmd *cobra.Command, args []string) {
	l.Println("Starting application...")
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

}
