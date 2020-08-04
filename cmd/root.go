/*
Copyright © 2020 Anton Kramarev

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
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/dakaraj/gatling-to-influxdb/influx"
	l "github.com/dakaraj/gatling-to-influxdb/logger"
	"github.com/dakaraj/gatling-to-influxdb/parser"
	"github.com/spf13/cobra"
)

var (
	ctx    context.Context
	cancel context.CancelFunc
)

func preRunSetup(cmd *cobra.Command, args []string) error {
	// // Workaround for a mandatory testid (t) flag
	// if t, _ := cmd.Flags().GetString("test-id"); t == "" {
	// 	fmt.Print("Test identifier is not provided. Please provide some value with --testid (-t) flag\n\n")
	// 	cmd.Help()
	// 	os.Exit(1)
	// }
	// // End of workaround

	// Check if InfluxDB connection is successfull before going to detached mode
	err := influx.InitInfluxConnection(cmd)
	if err != nil {
		return fmt.Errorf("Failed to establish successful database connection: %w", err)
	}

	// If detached state is requested, filter out corresponding flags and start new process
	// returning with same arguments printing its PID. Then close the initial process
	if d, _ := cmd.Flags().GetBool("detached"); d {
		newArgs := make([]string, 0, len(os.Args)-2)
		for _, a := range os.Args[1:] {
			if a == "-d" || a == "--detached" {
				continue
			}
			newArgs = append(newArgs, a)
		}

		command := exec.Command(os.Args[0], newArgs...)
		if err := command.Start(); err != nil {
			return fmt.Errorf("Failed to start a detached process: %w", err)
		}
		pid := command.Process.Pid
		fmt.Printf("[PID]\t%d\n", pid)
		os.Exit(0)
	}

	// catcher of SIGINT SIGTERM signals
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
		sig := <-c
		l.Infof("Received signal %v. Stopping application...\n", sig)
		cancel()
	}()

	l.Infoln("Starting application...")

	return nil
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use: "g2i [path/to/results/dir]",
	Example: `g2i ./target/gatling -t "some-test-id" -d -l "/var/log/g2i.log"

Will first check InfluxDB connection.
Then will search for the latest results directory or wait for it to appear.
Next will search for simulation.log file to appear and start processing it.`,
	Short: "Write Gatling logs directly to InfluxDB",
	Long: `This application allows writing raw Gatling load testing
tool logs directly to InfluxDB avoiding unnecessary
complications of Graphite protocol.`,
	Version: "v0.1.0",
	PreRunE: preRunSetup,
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		parser.RunMain(cmd, args[0])
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	// Initiating logger before any other processes start
	logPath, _ := rootCmd.Flags().GetString("log")
	err := l.InitLogger(logPath)
	if err != nil {
		log.Fatalf("Failed to init application logger: %v\n", err)
	}

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		l.Errorln(err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().BoolP("help", "h", false, "Display this help for g2i application")
	rootCmd.Flags().BoolP("version", "v", false, "Display current g2i application version")
	rootCmd.Flags().BoolP("detached", "d", false, "Run application in background. Returns [PID] on start")
	rootCmd.Flags().StringP("address", "a", "http://localhost:8086", "HTTP address and port of InfluxDB instance")
	rootCmd.Flags().StringP("username", "u", "", "Username credential for InfluxDB instance")
	rootCmd.Flags().StringP("password", "p", "", "Password credential for InfluxDB instance")
	rootCmd.Flags().StringP("database", "b", "gatling", "Database name in InfluxDB")
	rootCmd.Flags().StringP("log", "l", "./log/g2i.log", "File path to application log file")
	rootCmd.Flags().StringP("test-id", "t", "", "Unique test identifier")
	rootCmd.Flags().UintP("stop-timeout", "s", 60, "Time (seconds) to exit if no new log lines found")
	rootCmd.Flags().UintP("max-batch-size", "m", 5000, "Max points batch size to sent to InfluxDB")

	// set up global context
	ctx, cancel = context.WithCancel(context.Background())
}
