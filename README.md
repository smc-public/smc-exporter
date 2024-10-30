# SMC prometheus exporter
## Building
Pre-requisites:
- go 1.21 or later
```
go mod tidy
go build smc-exporter.go
```
## Installation
After binary is built, you can simply copy it and use the example systemd service file to run as a service. eg.
```
cp smc-exporter /usr/local/bin
cp smc-exporter.service /usr/lib/systemd/system
systemctl daemon-reload
systemctl start smc-exporter
```
This will give you a systemd service (smc-exporter) with the exporter running on the default port, 2112. If you want to use a different port, you will need to override the systemd unit or modify the service file before copying with the flag `-port {portnumber}`. 

