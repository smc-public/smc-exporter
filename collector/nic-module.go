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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
)

type SlotInfo struct {
	Designation string
	BusAddress  string
	SlotNumber  string
}

type DeviceInfo struct {
	pciAddress string
	mode        string
	caName     string
	netDev      string
}

type NicModuleCollector struct {
	CachedSlots []SlotInfo

	state            *prometheus.GaugeVec
	physicalState    *prometheus.GaugeVec
	speed            *prometheus.GaugeVec
	biasCurrent      *prometheus.GaugeVec
	voltage          *prometheus.GaugeVec
	wavelength       *prometheus.GaugeVec
	transferDistance *prometheus.GaugeVec
	rxPower          *prometheus.GaugeVec
	txPower          *prometheus.GaugeVec
	attenuation      *prometheus.GaugeVec
}

func NewNicModuleCollector(namespace string) *NicModuleCollector {
	laneLabel := []string{"lane"}
	speedLabel := []string{"speed"}
	stdLabels := []string{"mode", "caname", "netdev", "serial", "hostname", "product_serial", "slot"}
	return &NicModuleCollector{
		state: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "state",
			Help:      "State (0: Disable, 1: Port PLL Down, 2: Polling, 3: Active, 4: Close port, 5: Physical Linkup, 6: Sleep, 7: Rx disable, ...)",
		}, append(stdLabels)),

		physicalState: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "infiniband_physical_state",
			Help:      "Infiniband physical state (0: Disabled, 1: Initializing, 2: Recover Config, 3: Config Test, 4: Wait Remote Test, 5: Wait Config Enhanced, 6: Config Idle, 7: LinkUp, ...)",
		}, append(stdLabels)),

		speed: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "link_speed_bps",
			Help:      "Link speed in bps",
		}, append(stdLabels)),

		biasCurrent: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "optical_bias_current_mA",
			Help:      "Bias current in mA per lane for optical cables",
		}, append(laneLabel, stdLabels...)),

		voltage: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "optical_voltage_mV",
			Help:      "Voltage in mV",
		}, stdLabels),

		wavelength: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "optical_wavelength_nm",
			Help:      "Wavelength in nm",
		}, stdLabels),

		transferDistance: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "optical_transfer_distance_m",
			Help:      "Transfer distance in m",
		}, stdLabels),

		rxPower: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "optical_rx_power_dBm",
			Help:      "RX power in dBm per lane for optical cables",
		}, append(laneLabel, stdLabels...)),

		txPower: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "optical_tx_power_dBm",
			Help:      "TX power in dBm per lane for optical cables",
		}, append(laneLabel, stdLabels...)),

		attenuation: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "copper_attenuation_dB",
			Help:      "Attenuation in dB per signal speed for copper cables",
		}, append(speedLabel, stdLabels...)),
	}
}

func (n *NicModuleCollector) Describe(ch chan<- *prometheus.Desc) {
	n.state.Describe(ch)
	n.physicalState.Describe(ch)
	n.speed.Describe(ch)
	n.biasCurrent.Describe(ch)
	n.voltage.Describe(ch)
	n.wavelength.Describe(ch)
	n.transferDistance.Describe(ch)
	n.rxPower.Describe(ch)
	n.txPower.Describe(ch)
	n.attenuation.Describe(ch)
}

func (n *NicModuleCollector) Collect(ch chan<- prometheus.Metric) {
	n.state.Collect(ch)
	n.physicalState.Collect(ch)
	n.speed.Collect(ch)
	n.biasCurrent.Collect(ch)
	n.voltage.Collect(ch)
	n.wavelength.Collect(ch)
	n.transferDistance.Collect(ch)
	n.rxPower.Collect(ch)
	n.txPower.Collect(ch)
	n.attenuation.Collect(ch)
}

func (n *NicModuleCollector) UpdateMetrics() {
	devices, _ := discoverMellanoxDevices()
	pciAddress2PhysicalDeviceInfo := getPciAddress2PhysicalDevice()
	for _, device := range devices {
		physicalDeviceInfo := pciAddress2PhysicalDeviceInfo[device.pciAddress]
		device.caName = physicalDeviceInfo.caName
		device.netDev = physicalDeviceInfo.netDev
		go n.runMlxlink(device)
	}
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

func (n *NicModuleCollector) runMlxlink(device DeviceInfo) {
	cmd := exec.Command("mlxlink", "-d", device.pciAddress, "-m")
	output, err := cmd.CombinedOutput()
	if err == nil {
		n.parseOutput(string(output), device)
	} else {
		log.Errorf("Error running mlxlink -d %s: %s\n", device.pciAddress, err)
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
func (n *NicModuleCollector) UpdateSlotInfo() {
	cmd := exec.Command("dmidecode", "-t", "slot")
	output, err := cmd.Output()
	if err != nil {
		log.Errorf("Error executing dmidecode: %s", err)
		return
	}
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	var slot SlotInfo
	designationPattern := regexp.MustCompile(`Designation:\s+(.+)`)
	busAddressPattern := regexp.MustCompile(`Bus Address:\s+(.+)`)
	slotNumberPattern := regexp.MustCompile(`\d+`) // Pattern to capture the slot number

	for scanner.Scan() {
		line := scanner.Text()

		if strings.Contains(line, "System Slot Information") {
			// When new slot info starts, append the previous slot if valid
			if slot.Designation != "" {
				n.CachedSlots = append(n.CachedSlots, slot)
			}
			slot = SlotInfo{} // Reset for new slot info
		}

		// Match and extract Designation
		if match := designationPattern.FindStringSubmatch(line); match != nil {
			slot.Designation = match[1]
			// Extract the slot number from the designation
			if slotNumMatch := slotNumberPattern.FindStringSubmatch(slot.Designation); len(slotNumMatch) > 0 {
				fmt.Sscanf(slotNumMatch[0], "%s", &slot.SlotNumber) // Convert string to int
			}
		}

		// Match and extract Bus Address
		if match := busAddressPattern.FindStringSubmatch(line); match != nil {
			slot.BusAddress = match[1]
		}
	}

	// Append the last slot if valid
	if slot.Designation != "" {
		n.CachedSlots = append(n.CachedSlots, slot)
	}

}

func (n *NicModuleCollector) findSlotByBusAddress(busAddress string) (SlotInfo, bool) {
	for _, slot := range n.CachedSlots {
		if slot.BusAddress == busAddress {
			return slot, true
		}
	}
	return SlotInfo{}, false
}

func (n *NicModuleCollector) matchMellanoxSlot(mellanoxPciAddress string) string {
	pciAsArray := strings.Split(mellanoxPciAddress, ".")
	slotAddress := mellanoxPciAddress
	if pciAsArray[len(pciAsArray)-1] == "1" {
		log.Debugf("Found port for %s is 1, changing to 0\n", mellanoxPciAddress)
		pciAsArray[len(pciAsArray)-1] = "0"
		slotAddress = strings.Join(pciAsArray, ".")
	}

	// Find the matching slot
	if slot, found := n.findSlotByBusAddress(slotAddress); found {
		log.Debugf("Found slot for address %s: %s\n", mellanoxPciAddress, slot.SlotNumber)
		return slot.SlotNumber
	}
	return ""
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
func getPciAddress2PhysicalDevice() map[string]DeviceInfo {
	result := make(map[string]DeviceInfo)
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
			result[ibAddress] = deviceInfo
		}

	}

	return result
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
func (n *NicModuleCollector) parseOutput(output string, device DeviceInfo) {
	hostname, err := os.Hostname()
	cmd := exec.Command("dmidecode", "-s", "system-serial-number")
	out, _ := cmd.Output()
	systemserial := strings.TrimSpace(string(out))
	slot := n.matchMellanoxSlot(device.pciAddress)
	if !utf8.ValidString(slot) {
		slot = "unknown"
	}
	if err != nil {
		log.Errorf("Error getting hostname: %v\n", err)
		hostname = "unknown"
	}

	// mlxlink uses ansi escape codes to highlight values
	// remove them so we can concentrate on content
	output = removeAnsiEscapeCodes(output)

	scanner := bufio.NewScanner(strings.NewReader(output))
	var cableType string
	var (
		state   float64
		stateOK bool
	)
	var (
		physicalState   float64
		physicalStateOK bool
	)
	var (
		speed   float64
		speedOK bool
	)
	var serial string
	var rxPowerValues, txPowerValues, biasCurrentValues, attenuationValues []float64
	var voltageValue, wavelengthValue float64
	var (
		stateRegex,
		physicalStateRegex,
		speedRegex,
		rxPowerRegex,
		txPowerRegex,
		biasCurrentRegex,
		voltageRegex,
		attenuationRegex,
		wavelengthRegex,
		serialRegex *regexp.Regexp
	)

	stateRegex = regexp.MustCompile(`^State *: (.*)`)
	physicalStateRegex = regexp.MustCompile(`^Physical state *: (.*)`)
	speedRegex = regexp.MustCompile(`^Speed *: (.*)`)
	rxPowerRegex = regexp.MustCompile(`Rx Power Current \[dBm\] *: ([\d\.,\-]+)`)
	txPowerRegex = regexp.MustCompile(`Tx Power Current \[dBm\] *: ([\d\.,\-]+)`)
	biasCurrentRegex = regexp.MustCompile(`Bias Current \[mA\] *: ([\d\.,\-]+)`)
	voltageRegex = regexp.MustCompile(`Voltage \[mV\] *: ([\d\.,\-]+)`)
	attenuationRegex = regexp.MustCompile(`Attenuation \((.*)\) \[dB\] *: ([\d\.,\-]+)`)
	wavelengthRegex = regexp.MustCompile(`Wavelength \[nm\] *: ([\d\.,\-]+)`)
	serialRegex = regexp.MustCompile(`Vendor Serial Number *:\s+([a-zA-Z0-9]+)`)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "Vendor Serial Number") {
			serial = strings.TrimSpace(strings.Split(line, ":")[1])
		} else if matches := serialRegex.FindStringSubmatch(line); matches != nil {
			serial = matches[1]
		}
		if !utf8.ValidString(serial) {
			serial = "unknown"
		}

		if matches := stateRegex.FindStringSubmatch(line); matches != nil {
			state, stateOK = stateValue(matches[1])
		}

		if matches := physicalStateRegex.FindStringSubmatch(line); matches != nil {
			physicalState, physicalStateOK = physicalStateValue(matches[1])
		}

		if matches := speedRegex.FindStringSubmatch(line); matches != nil {
			speed, speedOK = gbps(matches[1])
		}

		if strings.Contains(line, "Cable Type") || strings.Contains(line, "Connector") {
			if strings.Contains(line, "Optic") {
				cableType = "optical"
			} else if strings.Contains(line, "Copper") || strings.Contains(line, "copper") {
				cableType = "copper"
			}
		}

		if cableType == "optical" {
			// Parse RX power
			if matches := rxPowerRegex.FindStringSubmatch(line); matches != nil {
				for _, val := range parseFloats(matches[1]) {
					rxPowerValues = append(rxPowerValues, val)
				}
			}
			// Parse TX power
			if matches := txPowerRegex.FindStringSubmatch(line); matches != nil {
				for _, val := range parseFloats(matches[1]) {
					txPowerValues = append(txPowerValues, val)
				}
			}
			// Parse bias current
			if matches := biasCurrentRegex.FindStringSubmatch(line); matches != nil {
				for _, val := range parseFloats(matches[1]) {
					biasCurrentValues = append(biasCurrentValues, val)
				}
			}
			// Parse voltage
			if matches := voltageRegex.FindStringSubmatch(line); matches != nil {
				voltageValue = parseFloats(matches[1])[0]
				voltageValue = voltageValue * 1000
			}
			// Parse wavelength
			if matches := wavelengthRegex.FindStringSubmatch(line); matches != nil {
				wavelengthValue = parseFloats(matches[1])[0]
			}
		} else if cableType == "copper" {
			// Parse attenuation for copper
			if matches := attenuationRegex.FindStringSubmatch(line); matches != nil {
				attenuationValues = parseFloats(matches[2])
				attenuationSpeeds := parseSpeeds(matches[1])
				for i, attenuationValue := range attenuationValues {
					if i < len(attenuationSpeeds) {
						n.attenuation.WithLabelValues(
							attenuationSpeeds[i], device.mode, device.caName, device.netDev, serial, hostname, systemserial, slot,
						).Set(attenuationValue)
					}
				}
			}
		}
	}

	// Export link metrics
	if stateOK {
		n.state.WithLabelValues(device.mode, device.caName, device.netDev, serial, hostname, systemserial, slot).Set(state)
	}
	if physicalStateOK {
		n.physicalState.WithLabelValues(device.mode, device.caName, device.netDev, serial, hostname, systemserial, slot).Set(physicalState)
	}
	if speedOK {
		n.speed.WithLabelValues(device.mode, device.caName, device.netDev, serial, hostname, systemserial, slot).Set(speed)
	}

	// Export optical metrics
	for i, bias := range biasCurrentValues {
		n.biasCurrent.WithLabelValues(fmt.Sprintf("%d", i+1), device.mode, device.caName, device.netDev, serial, hostname, systemserial, slot).Set(bias)
	}
	for i, rx := range rxPowerValues {
		n.rxPower.WithLabelValues(fmt.Sprintf("%d", i+1), device.mode, device.caName, device.netDev, serial, hostname, systemserial, slot).Set(rx)
	}
	for i, tx := range txPowerValues {
		n.txPower.WithLabelValues(fmt.Sprintf("%d", i+1), device.mode, device.caName, device.netDev, serial, hostname, systemserial, slot).Set(tx)
	}
	n.voltage.WithLabelValues(device.mode, device.caName, device.netDev, serial, hostname, systemserial, slot).Set(voltageValue)
	n.wavelength.WithLabelValues(device.mode, device.caName, device.netDev, serial, hostname, systemserial, slot).Set(wavelengthValue)
}

func removeAnsiEscapeCodes(output string) string {
	ansiEscapeCodeRegEx := regexp.MustCompile("[\u001B\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\u0007)|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PRZcf-ntqry=><~]))")
	return ansiEscapeCodeRegEx.ReplaceAllString(output, "")
}

func stateValue(stateName string) (float64, bool) {
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

	if value, exists := stateValues[stateName]; exists {
		return value, true
	} else {
		return -1, false
	}
}

func physicalStateValue(physicalStateName string) (float64, bool) {
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

	if value, exists := physicalStateValues[physicalStateName]; exists {
		return value, true
	} else {
		return -1, false
	}
}

func gbps(speed string) (float64, bool) {
	var speed2Gbps = map[string]float64{
		// IB
		"IB-SDR":   10000000,
		"IB-DDR":   20000000,
		"IB-QDR":   40000000,
		"IB-FDR10": 40000000,
		"IB-FDR":   56000000,
		"IB-EDR":   100000000,
		"IB-HDR":   200000000,
		"IB-NDR":   400000000,
		"IB-XDR":   800000000,
		// Eth
		"BaseTx100M": 100000,
		"BaseT1000M": 1000000,
		"BaseT10M":   10000,
		"CX":         1000000,
		"KX":         1000000,
		"CX4":        10000000,
		"KX4":        10000000,
		"BaseT10G":   10000000,
		"10GbE":      10000000,
		"20GbE":      20000000,
		"25GbE":      25000000,
		"40GbE":      40000000,
		"50GbE":      50000000,
		"56GbE":      56000000,
		"100GbE":     100000000,
		// Ext Eth
		"100M": 100000,
		"1G":   1000000,
		"2.5G": 2500000,
		"5G":   5000000,
		"10G":  10000000,
		"40G":  40000000,
		"25G":  25000000,
		"50G":  50000000,
		"100G": 100000000,
		"200G": 200000000,
		"400G": 400000000,
		"800G": 800000000,
		"10M":  10000,
	}

	if value, exists := speed2Gbps[speed]; exists {
		return value, true
	} else {
		return -1, false
	}
}

func parseFloats(s string) []float64 {
	parts := strings.Split(s, ",")
	values := make([]float64, len(parts))
	for i, part := range parts {
		values[i], _ = strconv.ParseFloat(strings.TrimSpace(part), 64)
	}
	return values
}

func parseSpeeds(s string) []string {
	return strings.Split(s, ",")
}
