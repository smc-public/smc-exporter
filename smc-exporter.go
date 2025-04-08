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
	sprom "smc-exporter/collector"
	"strings"
	"time"
	"fmt"
	"os"

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
	flag.StringVar(&port, "port", "2112", "Port to expose metrics on")
	flag.IntVar(&interval, "interval", 10, "Interval used to update metrics")
	flag.BoolVar(&showVersion, "version", false, "Show application version")
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
	se := NewSmcExporter()
	// Start collection loop (prometheus scrape is async)
	go func() {
		for {
			se.NicModule.UpdateMetrics()
			time.Sleep(time.Duration(interval) * time.Second)
		}
	}()
	go func() {
		for {
			se.NicModule.UpdateSlotInfo()
			time.Sleep(time.Duration(300) * time.Second)
		}
	}()
	reg.MustRegister(se)
	sh := SmcPrometheusHandler(reg)
	router.GET("/metrics", sh)
	log.Println("Starting smc-exporter on port " + port, "version", version.Info())
	log.Info("Build context", "build_context", version.BuildContext())
	if err := router.Run(":" + port); err != nil {
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

// Overarching exporter with sub collectors
type SmcExporter struct {
	NicModule *sprom.NicModuleCollector
}

func NewSmcExporter() *SmcExporter {
	return &SmcExporter{
		NicModule: sprom.NewNicModuleCollector(PREFIX + "_nic_module"),
	}
}

func (s *SmcExporter) Collect(ch chan<- prometheus.Metric) {
	s.NicModule.Collect(ch)
}

func (s *SmcExporter) Describe(ch chan<- *prometheus.Desc) {
	s.NicModule.Describe(ch)
}
