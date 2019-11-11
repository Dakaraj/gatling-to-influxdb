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

var (
	c      infc.Client
	l      = logger.GetLogger()
	dbName string

	// PointsChannel is a channel to send Request and Group metrics to
	PointsChannel = make(chan *infc.Point, 100)

	maxPoints = 5000
)

func sendPoints(points []*infc.Point) {
	fmt.Println("Writing to DB")
	bp, _ := infc.NewBatchPoints(infc.BatchPointsConfig{
		Precision: "ms", // test how it behaves on exact values
		Database:  dbName,
	})
	bp.AddPoints(points)
	err := c.Write(bp)
	if err != nil {
		l.Fatalf("Error sending points batch to InfluxDB: %v\n", err) // TODO: change from Fatal to Printf (DEBUGGING)
	}
	fmt.Println("Write successful")
}

func gatherPointMetrics(stop <-chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()
	points := make([]*infc.Point, 0, maxPoints)
	timer := time.NewTimer(time.Second * 30)
	for {
		select {
		case <-timer.C:
			if len(points) > 0 {
				sendPoints(points)
				points = make([]*infc.Point, 0, 5000)
			}
			timer.Reset(time.Second * 30)
		case p := <-PointsChannel:
			points = append(points, p)
			if len(points) == maxPoints {
				sendPoints(points)
				points = make([]*infc.Point, 0, 5000)
			}
		case <-stop:
			if len(points) > 0 {
				sendPoints(points)
				// don't know if it will help to clear some stuff, but whatever
				points = make([]*infc.Point, 0, 5000)
			}
			break
		}
	}
}

func startProcessing(stop <-chan struct{}) {
	// TODO: start all consumers
	// colect cancellation channels for all routines
	wg := &sync.WaitGroup{}
	cancelChans := make([]chan struct{}, 3)

	// start requests consumer
	cancelChans = append(cancelChans, make(chan struct{}))
	wg.Add(1)
	go gatherPointMetrics(cancelChans[0], wg)

	<-stop

	fmt.Println("Stopping all consumers...")
	for _, ch := range cancelChans {
		ch <- struct{}{}
	}
	wg.Wait()
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
