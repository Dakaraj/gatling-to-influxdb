# gatling-to-influxdb

## Why

Main pont of this application, is that standard Gatling to InfluxDB integration is very limited. It only writes aggregated data to the database, resulting in double aggregation when designing graph visualization. This tool writes raw data based on the log file provided by Gatling, though you need it to be enabled through `data.writers = [file]` in `gatling.conf` (enabled by default).

Querying raw data may have a significant impact on your database performance, be aware of that. But you can easily set up some continuous queries for regularly accessed data and leave raw data for more detailed analysis.

## Features

This app provides additional tags to aggregate or filter by:

- `testId` - provided via `--test-id` (`-t`) key
- `nodeName` - uses server `hostname`, added automatically

Added separate group data with raw duration - requests only, - and total duration - including timers.

Measurements `requests` and `groups` contain `userId` field, no idea where to use it for now, though.

Measurement `tests` is useful for setting up annotations in Grafana, contains test start / end times with description.

Measurement `users` contains snapshots of user activity per scenario aggregated for each 5 seconds.

## Usage

Application takes only one required positional argument - path to Gatling results directory. Usually something like `my-project/target/gatling` (for `sbt` projects) which contains directories like `simulations-20200731115117240`.

To get help on application usage use `--help` (`-h`) key. It will provide all existing keys and simple examples.

`g2i` needs to be started before gatling test. A detached mode is available using `--detached` (`-d`) key that will launch application in background. On successful start it will print PID of started process for later use, like interrupting a process, which will finish all the work left and safely exit.

Application writes a log with all errors encountered, by default it is located at `./log/g2i.log`, so any issues with application can be traced there. Log file path can be customized using `--log` (`-l`) key.

By default `g2i` looks for InfluxDB at `http://localhost:8086` but it can be easily changed using `--address` (`-a`) key with another HTTP address. UDP connection is not implemented yet, leave a feedback if this feature is really required.

Default database name is `gatling`, it can be changed using `--database` (`-b`) key following another name.

If database uses authentication, credentials can be provided using `--username` and `--password` (`-u` and `-p` respectfully) keys.

Integrating to CI can be done by running a set of commands like this (example uses SBT):

```bash
echo "Cleaning old stuff" && \
sbt clean && \
echo "Starting g2i in detached mode, saving PID in variable" && \
G2IPID=$(g2i ./target/gatling -a http://localhost:8086 -u root -p root -b gatling -t "MySimulation-$BUILD_NUMBER" -d | awk '{print $2}') && \
echo "Compile and launch simulation" && \
sbt compile "gatling:testOnly simulations.MySimulation"; \
echo "Waiting for parser to safely finish all its work" && \
sleep 10 && \
echo "Sending interrupt signal to g2i process" && \
kill -INT $G2IPID && \
echo "Waiting for process to stop safely" && \
sleep 10 && \
echo "Exiting"
```

## Warning

For now `g2i` requires read/write access to InfluxDB, it is a workaround for checking if connection is successful.

Application can be used for parsing an existing log file, but some tricky approach is required for it. Easier way will be implemented in later versions.

No unit tests are written for application as of right now and it's absolutely not production ready or battle tested yet. But you can try it anyway :)

It was also only tested on HTTP requests, no WS or other protocols were used, so if you have logs containing some data for non-HTTP protocols I'll be glad if you provide it (obfuscate data if need to) for analysis.

It also may not keep up its parser with high-rate scenarios like over 4000-5000+ requests per second, as it sends batched requests to server in consecutive, not concurrent fashion, can be changed quite easily. But for that high rate multiple load generators are usually used, so should not be a problem for now.

## Building application

For building latest version yourself, go version 1.13 or older is required with enabled modules support `GO111MODULE=on` (default behavior). Binary will be stored in \$GOBIN directory, this variable should be in path to be able to run application after installing from anywhere.

```bash
git clone git@github.com:Dakaraj/gatling-to-influxdb.git
cd gatling-to-influxdb
go mod download
go install g2i.go
```

## Distribution and Contribution

This application is licensed under MIT license meaning you are free to distribute or modify it without any restrictions.

Contribute as usual by forking and making a pull request with changes, or just create an issue and I will contact you regarding details. You can also contact me on [Telegram](https://t.me/Dakaraj).
