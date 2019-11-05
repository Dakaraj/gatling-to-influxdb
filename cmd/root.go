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

package cmd

import (
	"fmt"
	"os"

	"github.com/dakaraj/gatling-to-influxdb/validation"
	"github.com/spf13/cobra"
)

var (
	detached bool
	address  string
	port     uint16
	username string
	password string
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "g2i path/to/logfile",
	Short: "Write Gatling logs directly to InfluxDB",
	Long: `This application allows writing raw Gatling load testing
tool logs directly to InfluxDB avoiding unnecessary
complications of Graphite protocol.`,
	Version: "v0.0.1",
	Args:    validation.ValidateArgs,
	Run:     func(cmd *cobra.Command, args []string) {},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().BoolVarP(&detached, "detach", "d", false, "Run application in background. Returns [PID] on start")
	rootCmd.Flags().StringVarP(&address, "address", "a", "http://localhost", "HTTP address of InfluxDB instance")
	rootCmd.Flags().Uint16VarP(&port, "port", "p", 8088, "Port of InfluxDB instance")
	rootCmd.Flags().StringVar(&username, "username", "", "Username credential for InfluxDB instance")
	rootCmd.Flags().StringVar(&password, "password", "", "password credential for InfluxDB instance")
}
