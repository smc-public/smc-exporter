# SMC prometheus exporter
## Building
Pre-requisites:
- go 1.21 or later
```
go mod tidy
go build smc-exporter.go
```
## Pre-built Binary
A pre-built binary for linux amd64 servers is available to download in [Releases](https://github.com/smc-public/smc-exporter/releases)

## Installation
After a binary is built or downloaded, you can simply copy it and use the example systemd service file to run as a service. eg.
```
cp smc-exporter /usr/local/bin/smc-exporter
cp smc-exporter.service /usr/lib/systemd/system
systemctl daemon-reload
systemctl enable smc-exporter
systemctl start smc-exporter
```
This will give you a systemd service (smc-exporter) with the exporter running on the default port, 2112. If you want to use a different port, you will need to override the systemd unit or modify the service file before copying with the flag `-port {portnumber}`. 
## Uninstalling
To uninstall, stop and remove the service and remove the executeable.  eg.
```
systemctl stop smc-exporter
systemctl disable smc-exporter
rm /usr/lib/systemd/system/smc-exporter.service
systemctl daemon-reload
systemctl reset-failed
rm /usr/local/bin/smc-exporter
```

## Metrics information
Currently smc-exporter only collects metrics for HCA transceivers running in Infiniband or Ethernet mode. The following metrics are collected:
- state
- physical state
- speed
- bias current
- voltage
- power Rx (for each lane)
- power Tx (for each lane)
- wavelength
- transfer distance
