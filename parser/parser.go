/*
Copyright © 2019 Anton Kramarev

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
*/

package parser

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"time"

	c "github.com/dakaraj/gatling-to-influxdb/client"

	infc "github.com/influxdata/influxdb1-client/v2"

	"github.com/dakaraj/gatling-to-influxdb/logger"
)

const (
	// defines how long will file parser wait for new line before
	// stopping process (minutes)
	waitTime = 1
)

var (
	l        = logger.GetLogger()
	re       = regexp.MustCompile(`^.+?-(\d{14})\d{3}$`)
	start    = time.Now().Unix()
	nodeName string

	errFound = errors.New("Found")
	logDir   string
	testID   string

	tabSep = []byte{9}

	// regular expression patterns for matching log strings
	userLine    = regexp.MustCompile(`^USER\s`)
	requestLine = regexp.MustCompile(`^REQUEST\s`)
	groupLine   = regexp.MustCompile(`GROUP\s`)
	runLine     = regexp.MustCompile(`^RUN\s`)

	parserStopped = make(chan struct{})
)

func lookupTargetDir(ctx context.Context, dir string) error {
	for {
		// This block checks if stop signal is received from user
		// and stops further lookup
		select {
		case <-ctx.Done():
			return errors.New("Process stopped by user")
		default:
		}

		fInfo, err := os.Stat(dir)
		if err != nil && !os.IsNotExist(err) {
			return err
		} else if os.IsNotExist(err) {
			time.Sleep(time.Second * 5)
			continue
		}

		if !fInfo.IsDir() {
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
func lookupResultsDir(ctx context.Context, dir string) error {
	l.Println("Searching for results directory...")
	for {
		// This block checks if stop signal is received from user
		// and stops further lookup
		select {
		case <-ctx.Done():
			return errors.New("Process stopped by user")
		default:
		}

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

func waitForLog(ctx context.Context) error {
	for {
		// This block checks if stop signal is received from user
		// and stops further lookup
		select {
		case <-ctx.Done():
			return errors.New("Process stopped by user")
		default:
		}

		fInfo, err := os.Stat(logDir + "/simulation.log")
		if err != nil && !os.IsNotExist(err) {
			return err
		} else if os.IsNotExist(err) {
			time.Sleep(time.Second * 5)
			continue
		}

		// WARNING: second part of this check may fail on Windows. Not tested
		if fInfo.Mode().IsRegular() && fInfo.Mode().Perm() == 420 {
			abs, _ := filepath.Abs(logDir + "/simulation.log")
			l.Printf("Found %s\n", abs)
			break
		}

		return errors.New("Something wrong happened when attempting to open simulation.log")
	}

	return nil
}

func timeFromUnixBytes(ub []byte) time.Time {
	timeStamp, _ := strconv.ParseInt(string(ub), 10, 64)
	return time.Unix(0, timeStamp*1000000)
}

func userLineProcess(lb []byte) {
	split := bytes.Split(lb, tabSep)
	scenario := string(split[1])

	switch status := string(split[3]); status {
	case "START":
		c.IncUsersKey(scenario)
	case "END":
		c.DecUsersKey(scenario)
	}
}

func requestLineProcess(lb []byte) {
	split := bytes.Split(lb, tabSep)

	userID, _ := strconv.ParseInt(string(split[1]), 10, 32)
	start, _ := strconv.ParseInt(string(split[4]), 10, 64)
	stop, _ := strconv.ParseInt(string(split[5]), 10, 64)
	point, err := infc.NewPoint(
		"rawRequests",
		map[string]string{
			"requestName": string(split[3]),
			"groups":      string(split[2]),
			"result":      string(split[6]),
			"testId":      testID,
		},
		map[string]interface{}{
			"userId":       int(userID),
			"duration":     int(stop - start),
			"nodeName":     nodeName,
			"errorMessage": string(bytes.TrimSpace(split[7])),
		},
		timeFromUnixBytes(split[5]),
	)
	if err != nil {
		l.Printf("Error creating new point: %v\n", err)
	}
	c.SendPoint(point)
}

func groupLineProcess(lb []byte) {
	split := bytes.Split(lb, tabSep)
	// printByteSlices(split)

	userID, _ := strconv.ParseInt(string(split[1]), 10, 32)
	start, _ := strconv.ParseInt(string(split[3]), 10, 64)
	stop, _ := strconv.ParseInt(string(split[4]), 10, 64)
	duration, _ := strconv.ParseInt(string(split[5]), 10, 32)
	point, err := infc.NewPoint(
		"rawGroups",
		map[string]string{
			"name":   string(split[2]),
			"result": string(split[6][:2]),
			"testId": testID,
		},
		map[string]interface{}{
			"userId":        int(userID),
			"totalDuration": int(stop - start),
			"duration":      int(duration),
			"nodeName":      nodeName,
		},
		timeFromUnixBytes(split[4]),
	)
	if err != nil {
		l.Printf("Error creating new point: %v\n", err)
	}
	c.SendPoint(point)
}

func runLineProcess(lb []byte) {
	split := bytes.Split(lb, tabSep)

	point, _ := infc.NewPoint(
		"testStartEnd",
		map[string]string{
			"action":         "start",
			"testId":         testID,
			"simulationName": string(split[1]),
		},
		map[string]interface{}{
			"description": string(split[4]),
			"nodeName":    nodeName,
		},
		timeFromUnixBytes(split[3]),
	)
	c.SendPoint(point)
}

func stringProcessor(line []byte) error {
	switch {
	case requestLine.Match(line):
		requestLineProcess(line)
	case userLine.Match(line):
		userLineProcess(line)
	case groupLine.Match(line):
		groupLineProcess(line)
	case runLine.Match(line):
		runLineProcess(line)
	}

	return nil
}

func parseLoop(ctx context.Context, file *os.File) {
	r := bufio.NewReader(file)
	var buf []byte
	startWait := time.Now()
ParseLoop:
	for {
		select {
		case <-ctx.Done():
			l.Println("Parser received closing signal. Processing stopped")
			break ParseLoop
		default:
			b, err := r.ReadBytes('\n')
			if err == io.EOF {
				if time.Now().After(startWait.Add(time.Duration(waitTime) * time.Minute)) {
					l.Printf("No new lines found for %d minutes. Stopping application...", waitTime)
					break ParseLoop
				}
				buf = append(buf, b...)
				time.Sleep(time.Second * 2)
				continue
			}
			if err != nil && err != io.EOF {
				l.Printf("Unexpected error encountered while parsing file: %v", err)
			}
			buf = append(buf, b...)
			stringProcessor(buf)
			buf = []byte{}
			startWait = time.Now()
		}
	}
	parserStopped <- struct{}{}
}

func parseStart(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	l.Println("Starting log file parser...")
	file, err := os.Open(logDir + "/simulation.log")
	if err != nil {
		l.Fatalf("Failed to read simulation.log file: %v\n", err)
	}
	defer file.Close()

	parseLoop(ctx, file)
}

// RunMain performs main application logic
func RunMain(ctx context.Context, tID, dir string) {
	nodeName, _ = os.Hostname()
	testID = tID
	l.Printf("Searching for gatling directory at %s", dir)
	gatlingDir := dir + "/gatling"
	if err := lookupTargetDir(ctx, gatlingDir); err != nil {
		l.Fatalf("Target directory lookup failed with error: %v\n", err)
	}
	if err := lookupResultsDir(ctx, gatlingDir); err != nil {
		l.Fatalf("Error happened while searching for results directory: %v\n", err)
	}
	if err := waitForLog(ctx); err != nil {
		l.Fatalf("Failed waiting for simulation.log with error: %v\n", err)
	}
	wg := &sync.WaitGroup{}
	pCtx, pCancel := context.WithCancel(context.Background())
	cCtx, cCancel := context.WithCancel(context.Background())
	wg.Add(2)
	go parseStart(pCtx, wg)
	go c.StartProcessing(cCtx, wg)
FinisherLoop:
	for {
		select {
		case <-ctx.Done():
			pCancel()
		case <-parserStopped:
			cCancel()
			break FinisherLoop
		}
	}
	wg.Wait()
}
