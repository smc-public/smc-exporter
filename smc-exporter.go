// Copyright (c) 2024 Sustainable Metal Cloud
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of
// this software and associated documentation files (the "Software"), to deal in
// the Software without restriction, including without limitation the rights to
// use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
// the Software, and to permit persons to whom the Software is furnished to do so,
// subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
// FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
// COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
// IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
// CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

package main

import (
	"flag"
	"fmt"
	"os"
	sprom "smc-exporter/collector"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	versioncollector "github.com/prometheus/client_golang/prometheus/collectors/version"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/version"
	log "github.com/sirupsen/logrus"
)

func main() {
	var port string
	var interval int
	var showVersion bool
	var TLSEnabled bool
	var crtfile string
	var keyfile string
	flag.StringVar(&port, "port", "2112", "Port to expose metrics on")
	flag.IntVar(&interval, "interval", 10, "Interval used to update metrics")
	flag.BoolVar(&showVersion, "version", false, "Show application version")
	flag.BoolVar(&TLSEnabled, "TLSEnabled", false, "Enable TLS")
	flag.StringVar(&crtfile, "crtfile", "/etc/smc-exporter/tls.crt", "Define Crt file location")
	flag.StringVar(&keyfile, "keyfile", "/etc/smc-exporter/tls.key", "Define Key file location")
	logLevel := flag.String("loglevel", "info", "Set the log level: trace, debug, info, warn, error, fatal, panic")
	flag.Parse()

	level, err := log.ParseLevel(strings.ToLower(*logLevel))
	if err != nil {
		log.Fatalf("Invalid log level: %s", *logLevel)
	}
	log.SetLevel(level)

	if showVersion {
		fmt.Println(version.Print("smc_exporter"))
		os.Exit(0)
	}

	router := gin.Default()
	reg := prometheus.NewRegistry()
	reg.MustRegister(versioncollector.NewCollector("smc_exporter"))

	// NIC Module Collector
	nm := sprom.NewNicModuleCollector(PREFIX + "_nic_module")
	// Start collection loop (prometheus scrape is async)
	go func() {
		for {
			nm.UpdateMetrics()
			time.Sleep(time.Duration(interval) * time.Second)
		}
	}()
	reg.MustRegister(nm)

	// PCI Device Collector
	pd, err := sprom.NewPCIDeviceCollector(PREFIX)
	if err != nil {
		log.Errorf("Error creating PCIDeviceCollector: %s", err)
	} else if err = reg.Register(pd); err != nil {
		log.Errorf("Error registering PCIDeviceCollector: %s", err)
	}

	// Expose /metrics endpoint
	sh := SmcPrometheusHandler(reg)
	router.GET("/metrics", sh)
	log.Println("Starting smc-exporter on port "+port, "version", version.Info())
	log.Info("Build context", "build_context", version.BuildContext())
	if TLSEnabled {
		if err := router.RunTLS(":"+port, crtfile, keyfile); err != nil {
			log.Errorf("Error starting server: %v\n", err)
		}
	} else if err := router.Run(":" + port); err != nil {
		log.Errorf("Error starting server: %v\n", err)
	}
}

const (
	PREFIX = "smc"
)

func SmcPrometheusHandler(reg prometheus.Gatherer) gin.HandlerFunc {
	h := promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}
