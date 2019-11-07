package client

import (
	"fmt"
	"time"

	"github.com/dakaraj/gatling-to-influxdb/logger"
	infc "github.com/influxdata/influxdb1-client/v2"
	"github.com/spf13/cobra"
)

var (
	c infc.Client
	l = logger.GetLogger()
)

// SetUpInfluxConnection checks if connection with InfluxDB is successful
func SetUpInfluxConnection(cmd *cobra.Command) error {
	username, _ := cmd.Flags().GetString("username")
	password, _ := cmd.Flags().GetString("password")
	address, _ := cmd.Flags().GetString("address")
	var err error
	c, err = infc.NewHTTPClient(infc.HTTPConfig{
		Addr:      address,
		Username:  username,
		Password:  password,
		UserAgent: "g2i-http-client-" + cmd.Version,
		Timeout:   time.Second * 60,
	})
	if err != nil {
		return err
	}
	_, _, err = c.Ping(time.Second * 10)
	if err != nil {
		return fmt.Errorf("Connection with InfluxDB at %s could not be established", address)
	}
	l.Printf("Connection with InfluxDB at %s successfully established\n", address)

	return nil
}
