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

package client

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/dakaraj/gatling-to-influxdb/logger"
	infc "github.com/influxdata/influxdb1-client/v2"
	"github.com/spf13/cobra"
)

type syncUsers struct {
	m sync.Mutex
	u map[string]int
}

type testInfo struct {
	testID         string
	simulationName string
	description    string
	nodeName       string
}

func (s *syncUsers) GetSnapshot() map[string]int {
	m := make(map[string]int)
	s.m.Lock()
	for k, v := range s.u {
		m[k] = v
	}
	s.m.Unlock()

	return m
}

var (
	c         infc.Client
	l         = logger.GetLogger()
	dbName    string
	info      testInfo
	lastPoint = time.Now()

	// users is a thread safe map for storing current snapshot of users
	// amount generated
	users = syncUsers{
		m: sync.Mutex{},
		u: make(map[string]int),
	}

	// pc is a channel to send all point from parser to
	pc = make(chan *infc.Point, 100)

	// TODO: parameterize later
	maxPoints        = 5000
	writeDataTimeout = 10
)

// SendPoint sends point to the channel listened by metrics consumer
func SendPoint(p *infc.Point) {
	pc <- p
}

// IncUsersKey increments amount of users for given scenario
func IncUsersKey(scenario string) {
	users.m.Lock()
	users.u[scenario]++
	users.m.Unlock()
}

// DecUsersKey decrements amount of users for given scenario
func DecUsersKey(scenario string) {
	users.m.Lock()
	defer users.m.Unlock()
	if users.u[scenario] == 1 {
		delete(users.u, scenario)
		return
	}
	users.u[scenario]--

}

func sendBatch(points []*infc.Point) {
	bp, _ := infc.NewBatchPoints(infc.BatchPointsConfig{
		Precision: "ms", // test how it behaves on exact values
		Database:  dbName,
	})
	bp.AddPoints(points)
	err := c.Write(bp)
	if err != nil {
		l.Printf("Error sending points batch to InfluxDB: %v\n", err)
	}
	// Debug:
	l.Printf("Successfully written %d points to DB\n", len(points))
}

func gatherUsersSnapshots(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	ticker := time.NewTicker(time.Second * 5)
	defer ticker.Stop()
GatherLoop:
	for {
		select {
		case t := <-ticker.C:
			snap := users.GetSnapshot()
			if len(snap) > 0 {
				for k, v := range snap {
					p, _ := infc.NewPoint(
						"users",
						map[string]string{
							"scenario": k,
							"testId":   info.testID,
						},
						map[string]interface{}{
							"active":   v,
							"nodeName": info.nodeName,
						},
						t,
					)
					pc <- p
				}
			}
		case <-ctx.Done():
			break GatherLoop
		}
	}
}

func gatherPointMetrics(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	var points []*infc.Point
	infoRequired := true

	timer := time.NewTimer(time.Second * time.Duration(writeDataTimeout))
GatherLoop:
	for {
		select {
		case <-timer.C:
			if len(points) > 0 {
				// TODO: Maybe add some retry mechanic later
				sendBatch(points)
				points = make([]*infc.Point, 0, 5000)
			}
			timer.Reset(time.Second * time.Duration(writeDataTimeout))
		case p := <-pc:
			// this check searches for test start point and extracts test info from it
			if infoRequired {
				if p.Name() == "testStartEnd" {
					fields, _ := p.Fields()
					info = testInfo{
						testID:         p.Tags()["testId"],
						simulationName: p.Tags()["simulationName"],
						description:    fields["description"].(string),
						nodeName:       fields["nodeName"].(string),
					}
				}
				infoRequired = false
			}
			points = append(points, p)
			if len(points) == maxPoints {
				sendBatch(points)
				points = make([]*infc.Point, 0, 5000)
				timer.Reset(time.Second * time.Duration(writeDataTimeout))
			}
			lastPoint = time.Now()
		case <-ctx.Done():
			if len(points) > 0 {
				sendBatch(points)
				points = make([]*infc.Point, 0, 5000)
			}
			break GatherLoop
		}
	}
}

func sendClosingPoint() {
	// If info struct is empty, then parsing of file did not start,
	// so there is no need to send closing point
	if info.testID == "" {
		return
	}

	// Before application stop send a point that signifies a test finishing point
	p, _ := infc.NewPoint(
		"testStartEnd",
		map[string]string{
			"action":         "finish",
			"testId":         info.testID,
			"simulationName": info.simulationName,
		},
		map[string]interface{}{
			"description": info.description,
			"nodeName":    info.nodeName,
		},
		// Just to be sure adding +5 secods to the time when last point was received
		lastPoint.Add(time.Second*5),
	)
	sendBatch([]*infc.Point{p})
}

// StartProcessing starts consumers that receive points from parser and send to
// InfluxDB server
func StartProcessing(ctx context.Context, owg *sync.WaitGroup) {
	defer owg.Done()

	l.Println("Starting consumers for parser results")
	wg := &sync.WaitGroup{}

	// start requests consumer
	usCtx, usCancel := context.WithCancel(context.Background())
	gpmCtx, gpmCancel := context.WithCancel(context.Background())
	wg.Add(2)
	go gatherUsersSnapshots(usCtx, wg)
	go gatherPointMetrics(gpmCtx, wg)

	<-ctx.Done()

	l.Println("Stopping all consumers...")
	usCancel()
	gpmCancel() // This should be the last one

	wg.Wait()
	sendClosingPoint()
	l.Println("Finishing process")
}

// SetUpInfluxConnection checks if connection with InfluxDB is successful
func SetUpInfluxConnection(cmd *cobra.Command) error {
	username, _ := cmd.Flags().GetString("username")
	password, _ := cmd.Flags().GetString("password")
	address, _ := cmd.Flags().GetString("address")
	dbName, _ = cmd.Flags().GetString("database")
	var err error
	c, err = infc.NewHTTPClient(infc.HTTPConfig{
		Addr:      address,
		Username:  username,
		Password:  password,
		UserAgent: fmt.Sprintf("g2i-http-client-%s(%s)", cmd.Version, runtime.Version()),
		Timeout:   time.Second * 60,
	})
	if err != nil {
		return err
	}
	_, _, err = c.Ping(time.Second * 10)
	if err != nil {
		return fmt.Errorf("Connection with InfluxDB at %s could not be established. Error: %v", address, err)
	}
	res, err := c.Query(infc.NewQuery("SHOW MEASUREMENTS", dbName, "ms"))
	if err != nil {
		return fmt.Errorf("Connection with InfluxDB at %s could not be established. Error: %v", address, err)
	}
	if err := res.Error(); err != nil {
		return fmt.Errorf("Test query failed with error: %v", err)
	}
	l.Printf("Connection with InfluxDB at %s successfully established\n", address)

	return nil
}
