package cmd

import (
	"time"

	client "github.com/influxdata/influxdb1-client/v2"
	"github.com/spf13/cobra"
)

// VerifyInfluxDBConnection checks if connection with InfluxDB is successful
func verifyInfluxDBConnection(cmd *cobra.Command, args []string) error {
	c, err := client.NewHTTPClient(client.HTTPConfig{
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
		return err
	}

	return nil
}
