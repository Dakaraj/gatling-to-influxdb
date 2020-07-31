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
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/dakaraj/gatling-to-influxdb/influx"
	"github.com/dakaraj/gatling-to-influxdb/logger"
	"github.com/dakaraj/gatling-to-influxdb/types"

	// infc "github.com/influxdata/influxdb1-client/v2"
	"github.com/spf13/cobra"
)

const (
	oneMillisecond        = 1_000_000
	simulationLogFileName = "simulation.log"
	// Constant amounts of elements per log line
	runLineLen     = 6
	requestLineLen = 8
	groupLineLen   = 7
	userLineLen    = 6
	errorLineLen   = 3
)

var (
	l                    *log.Logger
	resultDirNamePattern = regexp.MustCompile(`^.+?-(\d{14})\d{3}$`)
	start                = time.Now().Unix()
	nodeName             string

	errFound         = errors.New("Found")
	errStoppedByUser = errors.New("Process stopped by user")
	errFatal         = errors.New("Fatal error")
	logDir           string
	testID           string
	simulationName   string
	waitTime         uint

	tabSep = []byte{9}

	// regular expression patterns for matching log strings
	userLine    = regexp.MustCompile(`^USER\s`)
	requestLine = regexp.MustCompile(`^REQUEST\s`)
	groupLine   = regexp.MustCompile(`GROUP\s`)
	runLine     = regexp.MustCompile(`^RUN\s`)
	errorLine   = regexp.MustCompile(`^ERROR\s`)

	parserStopped = make(chan struct{})
)

func lookupTargetDir(ctx context.Context, dir string) error {
	for {
		// This block checks if stop signal is received from user
		// and stops further lookup
		select {
		case <-ctx.Done():
			return errStoppedByUser
		default:
		}

		fInfo, err := os.Stat(dir)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("Target path %s exists but there is an error: %w", dir, err)
		}
		if os.IsNotExist(err) {
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
	if info.IsDir() && resultDirNamePattern.MatchString(info.Name()) {
		dateString := resultDirNamePattern.FindStringSubmatch(info.Name())[1]
		t, _ := time.Parse("20060102150405", dateString)
		if t.Unix() > start {
			logDir = path
			l.Printf("Found log directory at %s", logDir)

			return errFound
		}
	}

	return nil
}

// logic is the following: at the start of the application current timestamp is saved
// then traversing over all directories inside target dir is initiated.
// Every dir name is matched against pattern, if found - date time from dir name
// is parsed and  result timestamp is matched against application start time.
// Function stops as soon as matched date time is higher then initial one
func lookupResultsDir(ctx context.Context, dir string) error {
	l.Println("Searching for results directory...")
	for {
		// This block checks if stop signal is received from user
		// and stops further lookup
		select {
		case <-ctx.Done():
			return errStoppedByUser
		default:
		}

		err := filepath.Walk(dir, walkFunc)
		if err == errFound {
			break
		}
		if err != nil {
			return err
		}

		time.Sleep(time.Second * 5)
	}

	return nil
}

func waitForLog(ctx context.Context) error {
	l.Println("Searching for " + simulationLogFileName + " file...")
	for {
		// This block checks if stop signal is received from user
		// and stops further lookup
		select {
		case <-ctx.Done():
			return errStoppedByUser
		default:
		}

		fInfo, err := os.Stat(logDir + "/" + simulationLogFileName)
		if err != nil && !os.IsNotExist(err) {
			return err
		} else if os.IsNotExist(err) {
			time.Sleep(time.Second * 5)
			continue
		}

		// WARNING: second part of this check may fail on Windows. Not tested
		if fInfo.Mode().IsRegular() && (runtime.GOOS == "windows" || fInfo.Mode().Perm() == 420) {
			abs, _ := filepath.Abs(logDir + "/" + simulationLogFileName)
			l.Printf("Found %s\n", abs)
			break
		}

		return errors.New("Something wrong happened when attempting to open " + simulationLogFileName)
	}

	return nil
}

func timeFromUnixBytes(ub []byte) (time.Time, error) {
	timeStamp, err := strconv.ParseInt(string(ub), 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("Failed to parse timestamp as integer: %w", err)
	}
	// A workaround that adds random amount of microseconds to the timestamp
	// so db entries will (should) not be overwritten
	return time.Unix(0, timeStamp*oneMillisecond+rand.Int63n(oneMillisecond)), nil
}

func userLineProcess(lb []byte) error {
	split := bytes.Split(lb, tabSep)
	if len(split) != userLineLen {
		return errors.New("USER line contains unexpected amount of values")
	}
	scenario := string(split[1])
	timestamp, err := timeFromUnixBytes(split[4])
	if err != nil {
		return err
	}

	influx.SendUserLineData(types.NewUserLineData(timestamp, scenario, string(split[3])))

	// TODO change logic completely
	// Don't use last element, ot trim beforehand
	// switch status := string(split[3]); status {
	// case "START":
	// 	influx.IncUsersKey(scenario)
	// case "END":
	// 	influx.DecUsersKey(scenario)
	// }

	return nil
}

func requestLineProcess(lb []byte) error {
	split := bytes.Split(lb, tabSep)
	if len(split) != requestLineLen {
		return errors.New("REQUEST line contains unexpected amount of values")
	}

	userID, err := strconv.ParseInt(string(split[1]), 10, 32)
	if err != nil {
		return fmt.Errorf("Failed to parse userID in line as integer: %w", err)
	}
	start, err := strconv.ParseInt(string(split[4]), 10, 64)
	if err != nil {
		return fmt.Errorf("Failed to parse request start time in line as integer: %w", err)
	}
	end, err := strconv.ParseInt(string(split[5]), 10, 64)
	if err != nil {
		return fmt.Errorf("Failed to parse request end time in line as integer: %w", err)
	}
	timestamp, err := timeFromUnixBytes(split[5])
	if err != nil {
		return err
	}

	point, err := influx.NewPoint(
		"requests",
		map[string]string{
			"name":       string(split[3]),
			"groups":     string(split[2]),
			"result":     string(split[6]),
			"simulation": simulationName,
			"testId":     testID,
			"nodeName":   nodeName,
		},
		map[string]interface{}{
			"userId":       int(userID),
			"duration":     int(end - start),
			"errorMessage": string(bytes.TrimSpace(split[7])),
		},
		timestamp,
	)
	if err != nil {
		return fmt.Errorf("Error creating new point with request data: %w", err)
	}

	influx.SendPoint(point)

	return nil
}

func groupLineProcess(lb []byte) error {
	split := bytes.Split(lb, tabSep)
	if len(split) != groupLineLen {
		return errors.New("GROUP line contains unexpected amount of values")
	}

	userID, err := strconv.ParseInt(string(split[1]), 10, 32)
	if err != nil {
		return fmt.Errorf("Failed to parse userID in line as integer: %w", err)
	}
	start, err := strconv.ParseInt(string(split[3]), 10, 64)
	if err != nil {
		return fmt.Errorf("Failed to parse group start time in line as integer: %w", err)
	}
	end, err := strconv.ParseInt(string(split[4]), 10, 64)
	if err != nil {
		return fmt.Errorf("Failed to parse group end time in line as integer: %w", err)
	}
	rawDuration, err := strconv.ParseInt(string(split[5]), 10, 32)
	if err != nil {
		return fmt.Errorf("Failed to parse group raw duration in line as integer: %w", err)
	}
	timestamp, err := timeFromUnixBytes(split[5])
	if err != nil {
		return err
	}

	point, err := influx.NewPoint(
		"groups",
		map[string]string{
			"name":       string(split[2]),
			"result":     string(split[6][:2]),
			"simulation": simulationName,
			"testId":     testID,
			"nodeName":   nodeName,
		},
		map[string]interface{}{
			"userId":        int(userID),
			"totalDuration": int(end - start),
			"rawDuration":   int(rawDuration),
		},
		timestamp,
	)
	if err != nil {
		return fmt.Errorf("Error creating new point with group data: %w", err)
	}

	influx.SendPoint(point)

	return nil
}

// This method should be called first when parsing started as it is based
// on information from the header row
func runLineProcess(lb []byte) error {
	split := bytes.Split(lb, tabSep)
	if len(split) != runLineLen {
		return errors.New("RUN line contains unexpected amount of values")
	}

	simulationName = string(split[1])
	description := string(split[4])
	testStartTime, err := timeFromUnixBytes(split[3])
	if err != nil {
		return err
	}

	// This will initialize required data for influx client
	influx.InitTestInfo(testID, simulationName, description, nodeName, testStartTime)

	point, err := influx.NewPoint(
		"tests",
		map[string]string{
			"action":     "start",
			"simulation": simulationName,
			"testId":     testID,
			"nodeName":   nodeName,
		},
		map[string]interface{}{
			"description": description,
		},
		testStartTime,
	)
	if err != nil {
		return fmt.Errorf("Error creating new point with test start data: %w", err)
	}

	influx.SendPoint(point)

	return nil
}

func errorLineProcess(lb []byte) error {
	split := bytes.Split(lb, tabSep)
	if len(split) != errorLineLen {
		return errors.New("ERROR line contains unexpected amount of values")
	}
	timestamp, err := timeFromUnixBytes(bytes.TrimSpace(split[2]))
	if err != nil {
		return err
	}

	point, err := influx.NewPoint(
		"errors",
		map[string]string{
			"testId":     testID,
			"nodeName":   nodeName,
			"simulation": simulationName,
		},
		map[string]interface{}{
			"errorMessage": string(split[1]),
		},
		timestamp,
	)
	if err != nil {
		return fmt.Errorf("Error creating new point with error data: %w", err)
	}

	influx.SendPoint(point)

	return nil
}

func stringProcessor(lineBuffer []byte) error {
	switch {
	case requestLine.Match(lineBuffer):
		return requestLineProcess(lineBuffer)
	case groupLine.Match(lineBuffer):
		return groupLineProcess(lineBuffer)
	case userLine.Match(lineBuffer):
		return userLineProcess(lineBuffer)
	case errorLine.Match(lineBuffer):
		return errorLineProcess(lineBuffer)
	case runLine.Match(lineBuffer):
		err := runLineProcess(lineBuffer)
		if err != nil {
			err = fmt.Errorf("%v: %w", err, errFatal)
		}
		return err
	default:
		return fmt.Errorf("Unknown line type encountered")
	}
}

func fileProcessor(ctx context.Context, file *os.File) {
	r := bufio.NewReader(file)
	buf := new(bytes.Buffer)
	startWait := time.Now()
ParseLoop:
	for {
		// This block checks if stop signal is received from user
		// and stops further processing
		select {
		case <-ctx.Done():
			l.Println("Parser received closing signal. Processing stopped")
			break ParseLoop
		default:
		}

		b, err := r.ReadBytes('\n')
		if err == io.EOF {
			// If no new lines read for more than value provided by 'stop-timeout' key then processing is stopped
			if time.Now().After(startWait.Add(time.Duration(waitTime) * time.Second)) {
				l.Printf("No new lines found for %d seconds. Stopping application...", waitTime)
				break ParseLoop
			}
			// All new data is stored in buffer until next loop
			buf.Write(b)
			time.Sleep(time.Second)
			continue
		}
		if err != nil {
			l.Printf("Unexpected error encountered while parsing file: %v", err)
		}

		buf.Write(b)
		err = stringProcessor(buf.Bytes())
		if err != nil {
			l.Printf("String processing failed: %v", err)
			if errors.Is(err, errFatal) {
				l.Println("Log parser caught an error that can't be handled. Stopping application...")
				break ParseLoop
			}
		}
		// Clean buffer after processing preparing for a new loop
		buf.Reset()
		// Reset a timeout timer
		startWait = time.Now()
	}
	parserStopped <- struct{}{}
}

func parseStart(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	l.Println("Starting log file parser...")
	file, err := os.Open(logDir + "/" + simulationLogFileName)
	if err != nil {
		l.Fatalf("Failed to read %s file: %v\n", simulationLogFileName, err)
	}
	defer file.Close()

	fileProcessor(ctx, file)
}

// RunMain performs main application logic
func RunMain(cmd *cobra.Command, dir string) {
	// Getting logger for package
	l = logger.GetLogger()

	testID, _ = cmd.Flags().GetString("test-id")
	waitTime, _ = cmd.Flags().GetUint("stop-timeout")
	rand.Seed(time.Now().UnixNano())
	nodeName, _ = os.Hostname()

	l.Printf("Searching for directory at %s", dir)
	abs, err := filepath.Abs(dir)
	if err != nil {
		l.Fatalf("Failed to construct an absolute path for %s: %v", dir, err)
	}

	if err := lookupTargetDir(cmd.Context(), abs); err != nil {
		l.Fatalf("Target directory lookup failed with error: %v\n", err)
	}

	if err := lookupResultsDir(cmd.Context(), abs); err != nil {
		l.Fatalf("Error happened while searching for results directory: %v\n", err)
	}

	if err := waitForLog(cmd.Context()); err != nil {
		l.Fatalf("Failed waiting for %s with error: %v\n", simulationLogFileName, err)
	}

	wg := &sync.WaitGroup{}
	pCtx, pCancel := context.WithCancel(context.Background())
	cCtx, cCancel := context.WithCancel(context.Background())

	wg.Add(2)
	go parseStart(pCtx, wg)
	go influx.StartProcessing(cCtx, wg)

FinisherLoop:
	for {
		select {
		case <-cmd.Context().Done():
			pCancel()
		case <-parserStopped:
			cCancel()
			// Closing context if not canceled before
			pCancel()
			break FinisherLoop
		}
	}
	wg.Wait()
}
