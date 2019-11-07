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
	"os/exec"
	"strings"

	"github.com/dakaraj/gatling-to-influxdb/client"
	"github.com/dakaraj/gatling-to-influxdb/logger"
	"github.com/dakaraj/gatling-to-influxdb/parser"
	"github.com/spf13/cobra"
)

var (
	detached bool

	l = logger.GetLogger()
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use: "g2i target/directory/path",
	Example: `g2i ./target

Will first wait for directory to appear if not exists yet.
Then will search for the latest results directory or wait for it to appear.
Next will search for simulation.log file to appear and start processing it.`,
	Short: "Write Gatling logs directly to InfluxDB",
	Long: `This application allows writing raw Gatling load testing
tool logs directly to InfluxDB avoiding unnecessary
complications of Graphite protocol.`,
	Version: "v0.0.1",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		if detached {
			newArgs := make([]string, 0, len(os.Args))
			for i, a := range os.Args {
				if strings.HasPrefix(a, "-d") || i == 0 {
					continue
				}
				newArgs = append(newArgs, a)
			}
			command := exec.Command(os.Args[0], newArgs...)
			if err := command.Run(); err != nil {
				return err
			}
			pid := command.Process.Pid
			fmt.Println("Started background process with [PID]:", pid)
			os.Exit(0)
		}
		l.Println("Starting application...")
		return client.SetUpInfluxConnection(cmd)
	},
	Args: cobra.ExactArgs(1),
	Run:  parser.RunMain,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		l.Fatalln(err)
	}
}

func init() {
	rootCmd.Flags().BoolVarP(&detached, "detach", "d", false, "Run application in background. Returns [PID] on start")
	rootCmd.Flags().StringP("address", "a", "http://localhost:8086", "HTTP address and port of InfluxDB instance")
	rootCmd.Flags().StringP("username", "u", "", "Username credential for InfluxDB instance")
	rootCmd.Flags().StringP("password", "p", "", "Password credential for InfluxDB instance")
	rootCmd.Flags().StringP("database", "b", "gatling", "Name of the database in InfluxDB")
}
