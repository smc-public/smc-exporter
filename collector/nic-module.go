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

package collector

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

type SlotInfo struct {
	Designation string
	BusAddress  string
	SlotNumber  string
}

type DeviceInfo struct {
	pciAddress string
	mode       string
	caName     string
	netDev     string
	pkey       string
}

type PortMetrics struct {
	mode         string
	caname       string
	netdev       string
	serial       string
	hostname     string
	systemserial string
	vendor       string
	partNumber   string
	slot         string
	port         string
	pkey         string

	state            float64
	physicalState    float64
	moduleState      float64
	dataPathState    []float64
	speed            float64
	width            float64
	biasCurrent      []float64
	temperature      float64
	voltage          float64
	wavelength       float64
	transferDistance float64
	rxPower          []float64
	txPower          []float64
	snrMedia         []float64
	snrHost          []float64
	attenuation      map[string]float64
	effectiveBer     float64
	effectiveErrors  float64
	rawBer           float64
	rawErrors        []float64
	fecErrors        []float64
	symbolBer        float64
	symbolErrors     float64
	linkDown         float64
	linkRecovery     float64
	lastClearTime    float64
}

type NicModuleCollector struct {
	cachedMetricsReads  chan readCachedMetricsRequest
	cachedMetricsWrites chan []PortMetrics

	netInfoDesc          *prometheus.Desc
	stateDesc            *prometheus.Desc
	physicalStateDesc    *prometheus.Desc
	moduleStateDesc      *prometheus.Desc
	dataPathStateDesc    *prometheus.Desc
	speedDesc            *prometheus.Desc
	widthDesc            *prometheus.Desc
	biasCurrentDesc      *prometheus.Desc
	temperatureDesc      *prometheus.Desc
	voltageDesc          *prometheus.Desc
	wavelengthDesc       *prometheus.Desc
	transferDistanceDesc *prometheus.Desc
	rxPowerDesc          *prometheus.Desc
	txPowerDesc          *prometheus.Desc
	snrMediaDesc         *prometheus.Desc
	snrHostDesc          *prometheus.Desc
	attenuationDesc      *prometheus.Desc
	effectiveBerDesc     *prometheus.Desc
	effectiveErrorsDesc  *prometheus.Desc
	rawBerDesc           *prometheus.Desc
	rawErrorsDesc        *prometheus.Desc
	fecErrorsDesc        *prometheus.Desc
	symbolBerDesc        *prometheus.Desc
	symbolErrorsDesc     *prometheus.Desc
	linkDownDesc         *prometheus.Desc
	linkRecoveryDesc     *prometheus.Desc
	lastClearTimeDesc    *prometheus.Desc
}

type readCachedMetricsRequest struct {
	resp chan []PortMetrics
}

type runMlxlinkResponse struct {
	result PortMetrics
	error  bool
}

var stateValues = map[string]float64{
	"Disable":         0,
	"Port PLL Down":   1,
	"Polling":         2,
	"Active":          3,
	"Close port":      4,
	"Physical LinkUp": 5,
	"Sleep":           6,
	"Rx disable":      7,
	"Signal detect":   8,
	"Receiver detect": 9,
	"Sync peer":       10,
	"Negotiation":     11,
	"Training":        12,
	"SubFSM active":   13,
}

var speed2bps = map[string]float64{
	// IB
	"IB-SDR":   10000000000,
	"IB-DDR":   20000000000,
	"IB-QDR":   40000000000,
	"IB-FDR10": 40000000000,
	"IB-FDR":   56000000000,
	"IB-EDR":   100000000000,
	"IB-HDR":   200000000000,
	"IB-NDR":   400000000000,
	"IB-XDR":   800000000000,
	// Eth
	"BaseTx100M": 100000000,
	"BaseT1000M": 1000000000,
	"BaseT10M":   10000000,
	"CX":         1000000000,
	"KX":         1000000000,
	"CX4":        10000000000,
	"KX4":        10000000000,
	"BaseT10G":   10000000000,
	"10GbE":      10000000000,
	"20GbE":      20000000000,
	"25GbE":      25000000000,
	"40GbE":      40000000000,
	"50GbE":      50000000000,
	"56GbE":      56000000000,
	"100GbE":     100000000000,
	// Ext Eth
	"100M": 100000000,
	"1G":   1000000000,
	"2.5G": 2500000000,
	"5G":   5000000000,
	"10G":  10000000000,
	"40G":  40000000000,
	"25G":  25000000000,
	"50G":  50000000000,
	"100G": 100000000000,
	"200G": 200000000000,
	"400G": 400000000000,
	"800G": 800000000000,
	"10M":  10000000,
}

var physicalStateValues = map[string]float64{
	"Disabled":                   0,
	"Initializing":               1,
	"Recover Config":             2,
	"Config Test":                3,
	"Wait Remote Test":           4,
	"Wait Config Enhanced":       5,
	"Config Idle":                6,
	"LinkUp":                     7,
	"ETH_AN_FSM_ENABLE":          10,
	"ETH_AN_FSM_XMIT_DISABLE":    11,
	"ETH_AN_FSM_ABILITY_DETECT":  12,
	"ETH_AN_FSM_ACK_DETECT":      13,
	"ETH_AN_FSM_COMPLETE_ACK":    14,
	"ETH_AN_FSM_AN_GOOD_CHECK":   15,
	"ETH_AN_FSM_NEXT_PAGE_WAIT":  17,
	"ETH_AN_FSM_LINK_STAT_CHECK": 18,
	"ETH_AN_FSM_EXTRA_TUNE":      9,
	"ETH_AN_FSM_FIX_REVERSALS":   10,
	"ETH_AN_FSM_IB_FAIL":         11,
	"ETH_AN_FSM_POST_LOCK_TUNE":  12,
}

var moduleStateValues = map[string]float64{
	"N/A":          0,
	"LowPwr state": 1,
	"PwrUp state":  2,
	"Ready state":  3,
	"PwrDn state":  4,
	"Fault state":  5,
}

var dataPathStateValues = map[string]float64{
	"N/A":           0,
	"DPDeactivated": 1,
	"DPInit":        2,
	"DPDeinit":      3,
	"DPActivated":   4,
	"DPTxTurnOn":    5,
	"DPTxTurnOff":   6,
	"DPInitialized": 7,
}

type Slots []SlotInfo

func (s *Slots) getSlot(pciAddress string) string {
	slotAddress := pciAddress[:len(pciAddress)-1] + "0"
	for _, slot := range *s {
		if slot.BusAddress == slotAddress {
			if utf8.ValidString(slot.SlotNumber) {
				return slot.SlotNumber
			} else {
				return "unknown"
			}
		}
	}
	return ""
}

func NewNicModuleCollector(namespace string) *NicModuleCollector {
	laneLabel := []string{"lane"}
	binLabel := []string{"bin"}
	speedLabel := []string{"speed"}
	stdLabels := []string{"mode", "caname", "netdev", "serial", "hostname", "product_serial", "vendor", "part_number", "slot", "port"}
	collector := &NicModuleCollector{
		cachedMetricsReads:  make(chan readCachedMetricsRequest),
		cachedMetricsWrites: make(chan []PortMetrics),

		netInfoDesc: prometheus.NewDesc(
			namespace+"_network_info",
			"Non-numeric data from /sys/class/net/<iface>, value is always 1.",
			[]string{"netdev", "pkey"},
			nil,
		),

		stateDesc: prometheus.NewDesc(
			namespace+"_state",
			"State (0: Disable, 1: Port PLL Down, 2: Polling, 3: Active, 4: Close port, 5: Physical Linkup, 6: Sleep, 7: Rx disable, ...)",
			stdLabels,
			nil,
		),

		physicalStateDesc: prometheus.NewDesc(
			namespace+"_infiniband_physical_state",
			"Infiniband physical state (0: Disabled, 1: Initializing, 2: Recover Config, 3: Config Test, 4: Wait Remote Test, 5: Wait Config Enhanced, 6: Config Idle, 7: LinkUp, ...)",
			stdLabels,
			nil,
		),

		moduleStateDesc: prometheus.NewDesc(
			namespace+"_module_state",
			"Module state (0: N/A, 1: LowPwr state, 2: PwrUp state, 3: Ready state, 4: PwrDn state, 5: Fault state)",
			stdLabels,
			nil,
		),

		dataPathStateDesc: prometheus.NewDesc(
			namespace+"_datapath_state",
			"DataPath state (0: N/A, 1: DPDeactivated, 2: DPInit, 3: DPDeinit, 4: DPActivated, 5: DPTxTurnOn, 6: DPTxTurnOff, 7: DPInitialized)",
			append(laneLabel, stdLabels...),
			nil,
		),

		speedDesc: prometheus.NewDesc(
			namespace+"_link_speed_bps",
			"Link speed in bps",
			stdLabels,
			nil,
		),

		widthDesc: prometheus.NewDesc(
			namespace+"_width",
			"Width",
			stdLabels,
			nil,
		),

		biasCurrentDesc: prometheus.NewDesc(
			namespace+"_optical_bias_current_mA",
			"Bias current in mA per lane for optical cables",
			append(laneLabel, stdLabels...),
			nil,
		),

		temperatureDesc: prometheus.NewDesc(
			namespace+"_temperature_celsius",
			"Temperature in degrees celsius",
			stdLabels,
			nil,
		),

		voltageDesc: prometheus.NewDesc(
			namespace+"_optical_voltage_mV",
			"Voltage in mV",
			stdLabels,
			nil,
		),

		wavelengthDesc: prometheus.NewDesc(
			namespace+"_optical_wavelength_nm",
			"Wavelength in nm",
			stdLabels,
			nil,
		),

		transferDistanceDesc: prometheus.NewDesc(
			namespace+"_optical_transfer_distance_m",
			"Transfer distance in m",
			stdLabels,
			nil,
		),

		rxPowerDesc: prometheus.NewDesc(
			namespace+"_optical_rx_power_dBm",
			"RX power in dBm per lane for optical cables",
			append(laneLabel, stdLabels...),
			nil,
		),

		txPowerDesc: prometheus.NewDesc(
			namespace+"_optical_tx_power_dBm",
			"TX power in dBm per lane for optical cables",
			append(laneLabel, stdLabels...),
			nil,
		),

		snrMediaDesc: prometheus.NewDesc(
			namespace+"_snr_media_dB",
			"SNR media in dB",
			append(laneLabel, stdLabels...),
			nil,
		),

		snrHostDesc: prometheus.NewDesc(
			namespace+"_snr_host_dB",
			"SNR host in dB",
			append(laneLabel, stdLabels...),
			nil,
		),

		attenuationDesc: prometheus.NewDesc(
			namespace+"_copper_attenuation_dB",
			"Attenuation in dB per signal speed for copper cables",
			append(speedLabel, stdLabels...),
			nil,
		),

		effectiveBerDesc: prometheus.NewDesc(
			namespace+"_effective_bit_error_rate",
			"Effective bit error rate",
			stdLabels,
			nil,
		),

		effectiveErrorsDesc: prometheus.NewDesc(
			namespace+"_effective_errors_total",
			"Effective errors total",
			stdLabels,
			nil,
		),

		rawBerDesc: prometheus.NewDesc(
			namespace+"_raw_bit_error_rate",
			"Raw bit error rate",
			stdLabels,
			nil,
		),

		rawErrorsDesc: prometheus.NewDesc(
			namespace+"_raw_errors_total",
			"Raw errors total",
			append(laneLabel, stdLabels...),
			nil,
		),

		fecErrorsDesc: prometheus.NewDesc(
			namespace+"_fec_errors",
			"FEC error bins",
			append(binLabel, stdLabels...),
			nil,
		),

		symbolBerDesc: prometheus.NewDesc(
			namespace+"_symbol_bit_error_rate",
			"Symbol bit error rate",
			stdLabels,
			nil,
		),

		symbolErrorsDesc: prometheus.NewDesc(
			namespace+"_symbol_errors_total",
			"Symbol errors total",
			stdLabels,
			nil,
		),

		linkDownDesc: prometheus.NewDesc(
			namespace+"_link_down_total",
			"Link down total",
			stdLabels,
			nil,
		),

		linkRecoveryDesc: prometheus.NewDesc(
			namespace+"_link_recovery_total",
			"Link recovery total",
			stdLabels,
			nil,
		),

		lastClearTimeDesc: prometheus.NewDesc(
			namespace+"_last_clear_time_seconds",
			"Time since totals and bers were cleared in seconds",
			stdLabels,
			nil,
		),
	}
	go collector.manageCachedMetricsAccess()
	return collector
}

func (n *NicModuleCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- n.netInfoDesc
	ch <- n.stateDesc
	ch <- n.physicalStateDesc
	ch <- n.moduleStateDesc
	ch <- n.dataPathStateDesc
	ch <- n.speedDesc
	ch <- n.widthDesc
	ch <- n.biasCurrentDesc
	ch <- n.temperatureDesc
	ch <- n.voltageDesc
	ch <- n.wavelengthDesc
	ch <- n.transferDistanceDesc
	ch <- n.rxPowerDesc
	ch <- n.txPowerDesc
	ch <- n.snrMediaDesc
	ch <- n.snrHostDesc
	ch <- n.attenuationDesc
	ch <- n.effectiveBerDesc
	ch <- n.effectiveErrorsDesc
	ch <- n.rawBerDesc
	ch <- n.rawErrorsDesc
	ch <- n.fecErrorsDesc
	ch <- n.symbolBerDesc
	ch <- n.symbolErrorsDesc
	ch <- n.linkDownDesc
	ch <- n.linkRecoveryDesc
	ch <- n.lastClearTimeDesc
}

func (n *NicModuleCollector) Collect(ch chan<- prometheus.Metric) {
	cachedMetrics := n.getCachedMetrics()
	for _, port := range cachedMetrics {
		if port.netdev != "" {
			ch <- prometheus.MustNewConstMetric(n.netInfoDesc, prometheus.GaugeValue, 1, port.netdev, port.pkey)
		}
		stdLabelValues := []string{port.mode, port.caname, port.netdev, port.serial, port.hostname, port.systemserial, port.vendor, port.partNumber, port.slot, port.port}
		ch <- prometheus.MustNewConstMetric(n.stateDesc, prometheus.GaugeValue, port.state, stdLabelValues...)
		ch <- prometheus.MustNewConstMetric(n.physicalStateDesc, prometheus.GaugeValue, port.physicalState, stdLabelValues...)
		ch <- prometheus.MustNewConstMetric(n.moduleStateDesc, prometheus.GaugeValue, port.moduleState, stdLabelValues...)
		for laneIdx, dataPathState := range port.dataPathState {
			laneValue := []string{strconv.Itoa(laneIdx + 1)}
			ch <- prometheus.MustNewConstMetric(n.dataPathStateDesc, prometheus.GaugeValue, dataPathState, append(laneValue, stdLabelValues...)...)
		}
		ch <- prometheus.MustNewConstMetric(n.speedDesc, prometheus.GaugeValue, port.speed, stdLabelValues...)
		ch <- prometheus.MustNewConstMetric(n.widthDesc, prometheus.GaugeValue, port.width, stdLabelValues...)
		for laneIdx, biasCurrentValue := range port.biasCurrent {
			laneValue := []string{strconv.Itoa(laneIdx + 1)}
			ch <- prometheus.MustNewConstMetric(n.biasCurrentDesc, prometheus.GaugeValue, biasCurrentValue, append(laneValue, stdLabelValues...)...)
		}
		ch <- prometheus.MustNewConstMetric(n.temperatureDesc, prometheus.GaugeValue, port.temperature, stdLabelValues...)
		ch <- prometheus.MustNewConstMetric(n.voltageDesc, prometheus.GaugeValue, port.voltage, stdLabelValues...)
		ch <- prometheus.MustNewConstMetric(n.wavelengthDesc, prometheus.GaugeValue, port.wavelength, stdLabelValues...)
		ch <- prometheus.MustNewConstMetric(n.transferDistanceDesc, prometheus.GaugeValue, port.transferDistance, stdLabelValues...)
		for laneIdx, rxPowerValue := range port.rxPower {
			laneValue := []string{strconv.Itoa(laneIdx + 1)}
			ch <- prometheus.MustNewConstMetric(n.rxPowerDesc, prometheus.GaugeValue, rxPowerValue, append(laneValue, stdLabelValues...)...)
		}
		for laneIdx, txPowerValue := range port.txPower {
			laneLabelValues := []string{strconv.Itoa(laneIdx + 1)}
			ch <- prometheus.MustNewConstMetric(n.txPowerDesc, prometheus.GaugeValue, txPowerValue, append(laneLabelValues, stdLabelValues...)...)
		}
		for laneIdx, snrMediaValue := range port.snrMedia {
			laneLabelValues := []string{strconv.Itoa(laneIdx + 1)}
			ch <- prometheus.MustNewConstMetric(n.snrMediaDesc, prometheus.GaugeValue, snrMediaValue, append(laneLabelValues, stdLabelValues...)...)
		}
		for laneIdx, snrHostValue := range port.snrHost {
			laneLabelValues := []string{strconv.Itoa(laneIdx + 1)}
			ch <- prometheus.MustNewConstMetric(n.snrHostDesc, prometheus.GaugeValue, snrHostValue, append(laneLabelValues, stdLabelValues...)...)
		}
		for speedValue, attenuationValue := range port.attenuation {
			speedLabelValues := []string{speedValue}
			ch <- prometheus.MustNewConstMetric(n.attenuationDesc, prometheus.GaugeValue, attenuationValue, append(speedLabelValues, stdLabelValues...)...)
		}
		ch <- prometheus.MustNewConstMetric(n.effectiveBerDesc, prometheus.GaugeValue, port.effectiveBer, stdLabelValues...)
		ch <- prometheus.MustNewConstMetric(n.effectiveErrorsDesc, prometheus.CounterValue, port.effectiveErrors, stdLabelValues...)
		ch <- prometheus.MustNewConstMetric(n.rawBerDesc, prometheus.GaugeValue, port.rawBer, stdLabelValues...)
		for laneIdx, rawErrors := range port.rawErrors {
			laneValue := []string{strconv.Itoa(laneIdx + 1)}
			ch <- prometheus.MustNewConstMetric(n.rawErrorsDesc, prometheus.CounterValue, rawErrors, append(laneValue, stdLabelValues...)...)
		}

		for binIdx, fecErrors := range port.fecErrors {
			binValue := []string{strconv.Itoa(binIdx + 1)}
			ch <- prometheus.MustNewConstMetric(n.fecErrorsDesc, prometheus.CounterValue, fecErrors, append(binValue, stdLabelValues...)...)
		}
		if port.mode == "infiniband" {
			ch <- prometheus.MustNewConstMetric(n.symbolBerDesc, prometheus.GaugeValue, port.symbolBer, stdLabelValues...)
			ch <- prometheus.MustNewConstMetric(n.symbolErrorsDesc, prometheus.CounterValue, port.symbolErrors, stdLabelValues...)
			ch <- prometheus.MustNewConstMetric(n.linkDownDesc, prometheus.CounterValue, port.linkDown, stdLabelValues...)
			ch <- prometheus.MustNewConstMetric(n.linkRecoveryDesc, prometheus.CounterValue, port.linkRecovery, stdLabelValues...)
		}
		ch <- prometheus.MustNewConstMetric(n.lastClearTimeDesc, prometheus.GaugeValue, port.lastClearTime, stdLabelValues...)
	}
}

func (n *NicModuleCollector) getCachedMetrics() []PortMetrics {
	request := readCachedMetricsRequest{
		resp: make(chan []PortMetrics),
	}
	n.cachedMetricsReads <- request
	cachedMetrics := <-request.resp
	return cachedMetrics
}

func (n *NicModuleCollector) cacheMetrics(metrics []PortMetrics) {
	n.cachedMetricsWrites <- metrics
}

func (n *NicModuleCollector) manageCachedMetricsAccess() {
	var cachedMetrics []PortMetrics
	for {
		select {
		case request := <-n.cachedMetricsReads:
			request.resp <- cachedMetrics
		case newValue := <-n.cachedMetricsWrites:
			cachedMetrics = newValue
		}
	}
}

func (n *NicModuleCollector) UpdateMetrics() {
	devices, _ := discoverMellanoxDevices()
	pciAddress2PhysicalDeviceInfo := getPciAddress2DeviceInfo()
	hostname := getHostName()
	systemserial := getSystemSerial()
	slots := getSlots()

	responses := make(chan runMlxlinkResponse)
	for _, device := range devices {
		physicalDeviceInfo := pciAddress2PhysicalDeviceInfo[device.pciAddress]
		device.caName = physicalDeviceInfo.caName
		device.netDev = physicalDeviceInfo.netDev
		device.pkey = physicalDeviceInfo.pkey
		slot := slots.getSlot(device.pciAddress)
		var port string
		if function, ok := getFunction(device.pciAddress); ok {
			port = strconv.Itoa(function + 1)
		}
		go n.runMlxlink(hostname, systemserial, slot, port, device, responses)
	}
	metrics := make([]PortMetrics, 0, len(devices))
	for i := 0; i < len(devices); i++ {
		response := <-responses
		if !response.error {
			metrics = append(metrics, response.result)
		}
	}
	n.cacheMetrics(metrics)
}

func getFunction(s string) (int, bool) {
	parts := strings.Split(s, ".")
	if len(parts) > 0 {
		functionStr := parts[len(parts)-1]
		if function, err := strconv.Atoi(functionStr); err == nil {
			return function, true
		}
	}
	return 0, false
}

func getSystemSerial() string {
	cmd := exec.Command("dmidecode", "-s", "system-serial-number")
	out, _ := cmd.Output()
	systemserial := strings.TrimSpace(string(out))
	return systemserial
}

func getHostName() string {
	hostname, err := os.Hostname()
	if err != nil {
		log.Errorf("Error getting hostname: %v\n", err)
		hostname = "unknown"
	}
	return hostname
}

func getBondedIbDevice2Slaves() map[string][]string {
	result := make(map[string][]string)
	if _, err := os.Stat("/proc/net/bonding"); os.IsNotExist(err) {
		return result
	}
	bonds, err := os.ReadDir("/proc/net/bonding")
	if err != nil {
		log.Println("Error reading bonds dir:", err)
		return result
	}
	netDevice2IbDevice := getNetDevice2IbDevice()
	log.Debugf("Network device to infiniband device: %v", netDevice2IbDevice)
	for _, bond := range bonds {
		log.Debugf("Checking bond: %v", bond.Name())
		if ibDevice, exists := netDevice2IbDevice[bond.Name()]; exists {
			log.Debugf("Reading slaves for %s", bond.Name())
			// is a bond, get slaves
			slavesFile := "/sys/class/net/" + bond.Name() + "/bonding/slaves"
			slaves, err := os.ReadFile(slavesFile)
			if err != nil {
				log.Errorf("Error getting slaves for %s: %s", bond, err)
				continue
			}
			foundSlaves := strings.Split(string(slaves), " ")
			for _, d := range foundSlaves {
				slave := strings.TrimSpace(d)
				log.Debugf("Found slave %s", slave)
				result[ibDevice] = append(result[ibDevice], slave)
			}
		}
	}
	return result
}

// Discover Mellanox NICs using lspci
func discoverMellanoxDevices() ([]DeviceInfo, error) {
	var mellanoxDevices []DeviceInfo
	cmd := exec.Command("lspci", "-D")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(output), "\n")

	mellanoxDevicePattern := regexp.MustCompile(`(Infiniband|Ethernet).*Mellanox`)
	for _, line := range lines {
		if mellanoxDevicePattern.MatchString(line) {
			deviceInfo := DeviceInfo{}
			deviceInfo.pciAddress = strings.Fields(line)[0]
			deviceInfo.mode = strings.ToLower((strings.Fields(line)[1]))
			mellanoxDevices = append(mellanoxDevices, deviceInfo)
		}
	}
	return mellanoxDevices, nil
}

func (n *NicModuleCollector) runMlxlink(hostname, systemserial, slot, port string, device DeviceInfo, resp chan runMlxlinkResponse) {
	cmd := exec.Command("mlxlink", "-d", device.pciAddress, "-json", "-m", "-c", "--rx_fec_histogram", "--show_histogram") // #nosec G204
	output, err := cmd.CombinedOutput()
	mlxout := gjson.Parse(string(output))
	valid_output := false
	if err == nil {
		valid_output = true
	} else {
		// Check and see if there was an error relating to the FEC histogram param - if there is, we can still provide all
		//  of the other metrics
		status_message := mlxout.Get("status.message").String()
		if strings.Contains(status_message, "FEC Histogram is valid with active link operation only") ||
			strings.Contains(status_message, "FEC Histogram is not supported for the current device") {
			valid_output = true
		} else {
			log.Errorf("Error running mlxlink -d %s: %s\n", device.pciAddress, status_message)
		}
	}

	if valid_output {
		metrics := parseOutput(mlxout, hostname, systemserial, slot, port, device)
		resp <- runMlxlinkResponse{metrics, false}
	} else {
		resp <- runMlxlinkResponse{PortMetrics{}, true}
	}
}

func getNetDevice2IbDevice() map[string]string {
	result := make(map[string]string)
	cmd := exec.Command("ibdev2netdev")
	output, err := cmd.Output()
	if err != nil {
		log.Errorf("Error getting netdevs: %s", err)
		return result
	}
	lines := strings.Split(string(output), "\n")

	for _, line := range lines {
		parts := strings.Split(line, " ==> ")
		if len(parts) > 1 {
			ibDevice := strings.Fields(parts[0])[0]
			netDevice := strings.TrimSpace(strings.Fields(parts[1])[0])
			result[netDevice] = ibDevice
		}
	}
	return result

}

func getSlots() Slots {
	cmd := exec.Command("dmidecode", "-t", "slot")
	output, err := cmd.Output()
	if err != nil {
		log.Errorf("Error executing dmidecode: %s", err)
		return Slots{}
	}
	return parseSlots(string(output))

}

func parseSlots(output string) Slots {
	result := Slots{}
	scanner := bufio.NewScanner(strings.NewReader(output))
	var slot SlotInfo
	designationPattern := regexp.MustCompile(`Designation:\s+(.+)`)
	busAddressPattern := regexp.MustCompile(`Bus Address:\s+(.+)`)
	slotNumberPattern := regexp.MustCompile(`\d+`) // Pattern to capture the slot number

	for scanner.Scan() {
		line := scanner.Text()

		if strings.Contains(line, "System Slot Information") {
			// When new slot info starts, append the previous slot if valid
			if slot.Designation != "" {
				result = append(result, slot)
			}
			slot = SlotInfo{} // Reset for new slot info
		}

		// Match and extract Designation
		if match := designationPattern.FindStringSubmatch(line); match != nil {
			slot.Designation = match[1]
			// Extract the slot number from the designation
			if slotNumMatch := slotNumberPattern.FindStringSubmatch(slot.Designation); slotNumMatch != nil {
				slot.SlotNumber = slotNumMatch[0]
			}
		}

		// Match and extract Bus Address
		if match := busAddressPattern.FindStringSubmatch(line); match != nil {
			slot.BusAddress = match[1]
		}
	}

	// Append the last slot if valid
	if slot.Designation != "" {
		result = append(result, slot)
	}

	return result
}

// Get a map of device to pci address from /sys/class/{className}/{device}/device links
func getDevice2PciAddress(className string) map[string]string {
	result := make(map[string]string)
	basePath := filepath.Join("/sys/class", className)
	files, err := os.ReadDir(basePath)
	if err != nil {
		log.Errorf("Error listing %s devices: %s\n", className, err)
	}

	for _, f := range files {
		device := f.Name()
		classDevicePath := filepath.Join(basePath, device, "device")
		// Read the device symlink to get the device path
		devicePath, err := os.Readlink(classDevicePath)
		if err != nil {
			log.Debugf("Error reading device link for '%s': %s\n", device, err)
			continue
		}

		// The device path is a symlink to the device directory in /sys/devices/
		// Extract the PCI address from the device path
		// devicePath format is typically something like: /sys/devices/pci0000:00/0000:00:00.0
		// Split the path to find the PCI address
		parts := strings.Split(devicePath, "/")
		if len(parts) < 4 {
			log.Errorf("Unexpected device path format for %s: %s\n", device, devicePath)
			continue
		}

		pciAddress := parts[len(parts)-1]
		result[device] = pciAddress
	}
	return result
}

// Get a map of pci address to physical device
func getPciAddress2DeviceInfo() map[string]DeviceInfo {
	result := make(map[string]DeviceInfo)
	path := "/sys/class/infiniband"
	if info, err := os.Stat(path); err == nil {
		if info.IsDir() {
			// get map of pci address to infiniband device from /sys/class/infinband/device links
			ibDevice2PciAddress := getDevice2PciAddress("infiniband")
			// get map of pci address to network device from /sys/class/net/device links
			netDevice2PciAddress := getDevice2PciAddress("net")
			// get map of bonded ib device to slaves
			bondedIbDevice2Slaves := getBondedIbDevice2Slaves()

			for ibDevice, ibAddress := range ibDevice2PciAddress {
				if slaves, exists := bondedIbDevice2Slaves[ibDevice]; exists {
					// Add slave devices
					for _, slave := range slaves {
						if slaveAddress, exists := netDevice2PciAddress[slave]; exists {
							deviceInfo := DeviceInfo{}
							deviceInfo.pciAddress = slaveAddress
							deviceInfo.caName = ibDevice
							deviceInfo.netDev, _ = lookupKey(netDevice2PciAddress, slaveAddress)
							result[slaveAddress] = deviceInfo
						} else {
							log.Errorf("No PCI Address found for: %s\n", slave)
						}
					}
				} else {
					// Add ib device
					deviceInfo := DeviceInfo{}
					deviceInfo.pciAddress = ibAddress
					deviceInfo.caName = ibDevice
					deviceInfo.netDev, _ = lookupKey(netDevice2PciAddress, ibAddress)
					deviceInfo.pkey, _ = getPkey(deviceInfo.netDev)
					result[ibAddress] = deviceInfo
				}
			}
		}
	}

	return result
}

func getPkey(ibDevice string) (string, bool) {
	pkeyFilePath := filepath.Join("/sys/class/net", ibDevice, "pkey")
	pkeyFileContent, err := os.ReadFile(pkeyFilePath)
	if err != nil {
		log.Debugf("Error reading pkey for %s: %s", ibDevice, err)
		return "", false
	}
	return strings.TrimSpace(string(pkeyFileContent)), true
}

func lookupKey(lookupMap map[string]string, lookupValue string) (string, bool) {
	for key, value := range lookupMap {
		if value == lookupValue {
			return key, true
		}
	}
	return "", false
}

// Parse mlxlink data and set metrics
func parseOutput(mlxout gjson.Result, hostname, systemserial, slot, port string, device DeviceInfo) PortMetrics {
	var metrics PortMetrics

	metrics.mode = device.mode
	metrics.caname = device.caName
	metrics.netdev = device.netDev
	metrics.pkey = device.pkey
	metrics.hostname = hostname
	metrics.systemserial = systemserial
	metrics.slot = slot
	metrics.port = port

	valuesRegex := regexp.MustCompile(`([\d\.,\-]+)`)
	speedsRegex := regexp.MustCompile(`Attenuation \((.*)\) \[dB\]`)

	state := mlxout.Get("result.output.Operational Info.State").String()
	if stateValue, stateOK := stateValues[state]; stateOK {
		metrics.state = stateValue
	}

	physicalState := mlxout.Get("result.output.Operational Info.Physical state").String()
	if physicalStateValue, physicalStateOK := physicalStateValues[physicalState]; physicalStateOK {
		metrics.physicalState = physicalStateValue
	}

	speed := mlxout.Get("result.output.Operational Info.Speed").String()
	if speedValue, speedOK := speed2bps[speed]; speedOK {
		metrics.speed = speedValue
	}

	formattedWidth := mlxout.Get("result.output.Operational Info.Width").String()
	width := strings.TrimSuffix(formattedWidth, "x")
	if widthValue, err := strconv.ParseFloat(width, 64); err == nil {
		metrics.width = widthValue
	}

	metrics.serial = mlxout.Get("result.output.Module Info.Vendor Serial Number").String()
	if !utf8.ValidString(metrics.serial) {
		metrics.serial = "unknown"
	}

	metrics.vendor = mlxout.Get("result.output.Module Info.Vendor Name").String()
	metrics.partNumber = mlxout.Get("result.output.Module Info.Vendor Part Number").String()

	moduleState := mlxout.Get("result.output.Module Info.Module State").String()
	if moduleStateValue, moduleStateOK := moduleStateValues[moduleState]; moduleStateOK {
		metrics.moduleState = moduleStateValue
	}

	var cableType string
	cableTypeValue := mlxout.Get("result.output.Module Info.Cable Type").String()
	if strings.Contains(cableTypeValue, "Optic") {
		cableType = "optical"
	} else if strings.Contains(cableTypeValue, "Copper") || strings.Contains(cableTypeValue, "copper") {
		cableType = "copper"
	}

	metrics.attenuation = map[string]float64{}
	metrics.rxPower = []float64{}
	metrics.txPower = []float64{}
	metrics.snrMedia = []float64{}
	metrics.snrHost = []float64{}
	metrics.fecErrors = []float64{}
	metrics.biasCurrent = []float64{}

	temperature := mlxout.Get("result.output.Module Info.Temperature [C]").String()
	if matches := valuesRegex.FindStringSubmatch(temperature); matches != nil {
		metrics.temperature, _ = strconv.ParseFloat(matches[1], 64)
	}

	snrMediaPerLane := mlxout.Get("result.output.Module Info.SNR Media Lanes [dB].values").Array()
	if len(snrMediaPerLane) != 1 || snrMediaPerLane[0].String() != "N/A" {
		metrics.snrMedia = make([]float64, len(snrMediaPerLane))
		for i, snrMedia := range snrMediaPerLane {
			metrics.snrMedia[i] = snrMedia.Float()
		}
	}

	snrHostPerLane := mlxout.Get("result.output.Module Info.SNR Host Lanes [dB].values").Array()
	if len(snrHostPerLane) != 1 || snrHostPerLane[0].String() != "N/A" {
		metrics.snrHost = make([]float64, len(snrHostPerLane))
		for i, snrHost := range snrHostPerLane {
			metrics.snrHost[i] = snrHost.Float()
		}
	}

	if cableType == "optical" {
		// Parse DataPath state
		dataPathStatePerLane := mlxout.Get("result.output.Module Info.DataPath state [per lane].values").Array()
		metrics.dataPathState = make([]float64, len(dataPathStatePerLane))
		for i, dataPathState := range dataPathStatePerLane {
			metrics.dataPathState[i] = dataPathStateValues[dataPathState.String()]
		}
		// Parse RX power
		rxPowerCurrent := mlxout.Get("result.output.Module Info.Rx Power Current [dBm]").String()
		if matches := valuesRegex.FindStringSubmatch(rxPowerCurrent); matches != nil {
			metrics.rxPower, _ = parseFloats(matches[1])
		}
		// Parse TX power
		txPowerCurrent := mlxout.Get("result.output.Module Info.Tx Power Current [dBm]").String()
		if matches := valuesRegex.FindStringSubmatch(txPowerCurrent); matches != nil {
			metrics.txPower, _ = parseFloats(matches[1])
		}
		// Parse bias current
		biasCurrent := mlxout.Get("result.output.Module Info.Bias Current [mA]").String()
		if matches := valuesRegex.FindStringSubmatch(biasCurrent); matches != nil {
			metrics.biasCurrent, _ = parseFloats(matches[1])
		}
		// Parse voltage
		voltage := mlxout.Get("result.output.Module Info.Voltage [mV]").String()
		if matches := valuesRegex.FindStringSubmatch(voltage); matches != nil {
			metrics.voltage, _ = strconv.ParseFloat(matches[1], 64)
		}
		// Parse wavelength
		wavelength := mlxout.Get("result.output.Module Info.Wavelength [nm]").String()
		if matches := valuesRegex.FindStringSubmatch(wavelength); matches != nil {
			metrics.wavelength, _ = strconv.ParseFloat(matches[1], 64)
		}
	} else if cableType == "copper" {
		// Parse attenuation for copper
		module_keys := mlxout.Get("result.output.Module Info.@keys").Array()
		for _, key := range module_keys {
			if !strings.HasPrefix(key.Str, "Attenuation") {
				continue
			}
			attenuation_key := key.Str
			attenuation := mlxout.Get("result.output.Module Info." + attenuation_key)
			attenuationValues, _ := parseFloats(attenuation.Str)
			attenuationSpeeds := []string{}
			if matches := speedsRegex.FindStringSubmatch(attenuation_key); matches != nil {
				attenuationSpeeds = parseSpeeds(matches[1])
			}
			for i, attenuationValue := range attenuationValues {
				if i < len(attenuationSpeeds) {
					metrics.attenuation[attenuationSpeeds[i]] = attenuationValue
				}
			}
		}
	}

	metrics.effectiveBer = mlxout.Get("result.output.Physical Counters and BER Info.Effective Physical BER").Float()
	metrics.effectiveErrors = mlxout.Get("result.output.Physical Counters and BER Info.Effective Physical Errors").Float()
	metrics.rawBer = mlxout.Get("result.output.Physical Counters and BER Info.Raw Physical BER").Float()

	rawErrorsPerLane := mlxout.Get("result.output.Physical Counters and BER Info.Raw Physical Errors Per Lane.values").Array()
	metrics.rawErrors = make([]float64, len(rawErrorsPerLane))
	for i, rawErrors := range rawErrorsPerLane {
		metrics.rawErrors[i] = rawErrors.Float()
	}

	if mlxout.Get("result.output.Histogram of FEC Errors").Exists() {
		fecErrorBins := mlxout.Get("result.output.Histogram of FEC Errors").Map()
		numBins := len(fecErrorBins) - 1
		metrics.fecErrors = make([]float64, numBins)
		if numBins > 0 {
			for i := range numBins {
				binJson := fecErrorBins["Bin "+strconv.Itoa(i)]
				binValue, err := strconv.ParseFloat(binJson.Get("values").Array()[1].String(), 64)
				if err != nil {
					metrics.fecErrors[i] = -1
				} else {
					metrics.fecErrors[i] = binValue
				}
			}
		}
	}

	if metrics.mode == "infiniband" {
		metrics.symbolBer = mlxout.Get("result.output.Physical Counters and BER Info.Symbol BER").Float()
		metrics.symbolErrors = mlxout.Get("result.output.Physical Counters and BER Info.Symbol Errors").Float()
		metrics.linkDown = mlxout.Get("result.output.Physical Counters and BER Info.Link Down Counter").Float()
		metrics.linkRecovery = mlxout.Get("result.output.Physical Counters and BER Info.Link Error Recovery Counter").Float()
	}

	metrics.lastClearTime = mlxout.Get("result.output.Physical Counters and BER Info.Time Since Last Clear [Min]").Float() * 60

	return metrics
}

func parseFloats(s string) ([]float64, bool) {
	parts := strings.Split(s, ",")
	values := make([]float64, len(parts))
	for i, part := range parts {
		if value, err := strconv.ParseFloat(strings.TrimSpace(part), 64); err != nil {
			return []float64{}, false
		} else {
			values[i] = value
		}
	}
	return values, true
}

func parseSpeeds(s string) []string {
	return strings.Split(s, ",")
}
