package client

import (
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
	c        infc.Client
	l        = logger.GetLogger()
	dbName   string
	testInfo = true
	info     struct {
		testID         string
		simulationName string
		description    string
		nodeName       string
	}
	lastPoint = time.Now()

	// users is a thread safe map for storing current snapshot of users
	// amount generated. TODO: Make this var non-exported
	users = syncUsers{
		m: sync.Mutex{},
		u: make(map[string]int),
	}

	// pc is a channel to send Request and Group metrics to
	// TODO: Make this var non-exported
	pc = make(chan *infc.Point, 100)

	// maybe parameterize later
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
		l.Fatalf("Error sending points batch to InfluxDB: %v\n", err) // TODO: change from Fatal to Printf (DEBUGGING)
	}
	// Debug:
	l.Printf("Successfully written %d points to DB\n", len(points))
}

func gatherUsersSnapshots(stop <-chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()

	ticker := time.NewTicker(time.Second * 5)
	defer ticker.Stop()
	for {
		select {
		case t := <-ticker.C:
			snap := users.GetSnapshot()
			// fmt.Printf("%#v\n", snap) // TODO: Remove
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
		case <-stop:
			break
		}
	}
}

func gatherPointMetrics(stop <-chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()
	var points []*infc.Point

	timer := time.NewTimer(time.Second * time.Duration(writeDataTimeout))
	for {
		select {
		case <-timer.C:
			if len(points) > 0 {
				sendBatch(points)
				points = make([]*infc.Point, 0, 5000)
			}
			timer.Reset(time.Second * time.Duration(writeDataTimeout))
		case p := <-pc:
			// fmt.Println("Got point", p.Name()) // TODO: Remove
			// this check searches for test start point and extracts test info from it
			if testInfo {
				if p.Name() == "testStartEnd" {
					info.testID = p.Tags()["testId"]
					info.simulationName = p.Tags()["simulatiomnName"]
					fields, _ := p.Fields()
					info.description = fields["description"].(string)
					info.nodeName = fields["nodeName"].(string)
				}
				testInfo = false
			}
			points = append(points, p)
			if len(points) == maxPoints {
				sendBatch(points)
				points = make([]*infc.Point, 0, 5000)
				timer.Reset(time.Second * time.Duration(writeDataTimeout))
			}
			lastPoint = time.Now()
		case <-stop:
			if len(points) > 0 {
				sendBatch(points)
				points = make([]*infc.Point, 0, 5000)
			}
			break
		}
	}
}

func sendClosingPoint() {
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
		time.Now(),
	)
	sendBatch([]*infc.Point{p})
}

func startProcessing(stop <-chan struct{}) {
	// TODO: start all consumers
	wg := &sync.WaitGroup{}

	// start requests consumer
	cancelUS := make(chan struct{})
	cancelGPM := make(chan struct{})
	wg.Add(2)
	go gatherUsersSnapshots(cancelUS, wg)
	go gatherPointMetrics(cancelGPM, wg)

	<-stop

	l.Println("Stopping all consumers...")
	cancelUS <- struct{}{}
	cancelGPM <- struct{}{} // This should be the last one

	wg.Wait()
	sendClosingPoint()
	l.Println("Finishing process")
}

// SetUpInfluxConnection checks if connection with InfluxDB is successful
func SetUpInfluxConnection(cmd *cobra.Command, stop <-chan struct{}) error {
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

	l.Println("Starting consumers for parser results")
	go startProcessing(stop)

	return nil
}
