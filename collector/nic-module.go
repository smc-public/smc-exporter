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

type NicModuleCollector struct {
	CachedSlots []SlotInfo

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
	stdLabels := []string{"device", "serial", "hostname", "systemserial", "slot"}
	return &NicModuleCollector{
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
	n.biasCurrent.Describe(ch)
	n.voltage.Describe(ch)
	n.wavelength.Describe(ch)
	n.transferDistance.Describe(ch)
	n.rxPower.Describe(ch)
	n.txPower.Describe(ch)
	n.attenuation.Describe(ch)
}

func (n *NicModuleCollector) Collect(ch chan<- prometheus.Metric) {
	n.biasCurrent.Collect(ch)
	n.voltage.Collect(ch)
	n.wavelength.Collect(ch)
	n.transferDistance.Collect(ch)
	n.rxPower.Collect(ch)
	n.txPower.Collect(ch)
	n.attenuation.Collect(ch)
}

func (n *NicModuleCollector) UpdateMetrics() {
	pciAddresses, _ := discoverMellanoxDevices()
	pciAddress2Device := getPciAddress2PhysicalDevice()
	for _, pciAddress := range pciAddresses {
		device := pciAddress2Device[pciAddress]
		go n.runMlxlink(pciAddress, device)
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
func discoverMellanoxDevices() ([]string, error) {
	var mellanoxDevices []string
	cmd := exec.Command("lspci", "-D")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(output), "\n")

	mellanoxDevicePattern := regexp.MustCompile(`(Infiniband|Ethernet).*Mellanox`)
	for _, line := range lines {
		if mellanoxDevicePattern.MatchString(line) {
			pciAddress := strings.Fields(line)[0]
			mellanoxDevices = append(mellanoxDevices, pciAddress)
		}
	}
	return mellanoxDevices, nil
}

func (n *NicModuleCollector) runMlxlink(pciAddress string, device string) {
	cmd := exec.Command("mlxlink", "-d", pciAddress, "-m")
	output, err := cmd.CombinedOutput()
	if err == nil {
		n.parseOutput(string(output), pciAddress, device)
	} else {
		log.Errorf("Error running mlxlink -d %s: %s\n", pciAddress, err)
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
func getPciAddress2PhysicalDevice() map[string]string {
	result := make(map[string]string)
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
					result[slaveAddress] = slave
				} else {
					log.Errorf("No PCI Address found for: %s\n", slave)
				}
			}
		} else {
			// Add ib device
			result[ibAddress] = ibDevice
		}

	}

	return result
}

// Parse mlxlink data and set metrics
func (n *NicModuleCollector) parseOutput(output, pciAddress string, device string) {
	hostname, err := os.Hostname()
	cmd := exec.Command("dmidecode", "-s", "system-serial-number")
	out, _ := cmd.Output()
	systemserial := strings.TrimSpace(string(out))
	slot := n.matchMellanoxSlot(pciAddress)
	if !utf8.ValidString(slot) {
		slot = "unknown"
	}
	if err != nil {
		log.Errorf("Error getting hostname: %v\n", err)
		hostname = "unknown"
	}

	scanner := bufio.NewScanner(strings.NewReader(output))
	var cableType string
	var serial string
	var rxPowerValues, txPowerValues, biasCurrentValues, attenuationValues []float64
	var voltageValue, wavelengthValue float64
	var rxPowerRegex, txPowerRegex, biasCurrentRegex, voltageRegex, attenuationRegex, wavelengthRegex, serialRegex *regexp.Regexp

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
						n.attenuation.WithLabelValues(attenuationSpeeds[i], device, serial, hostname, systemserial, slot).Set(attenuationValue)
					}
				}
			}
		}
	}

	// Export optical metrics
	for i, bias := range biasCurrentValues {
		n.biasCurrent.WithLabelValues(fmt.Sprintf("%d", i+1), device, serial, hostname, systemserial, slot).Set(bias)
	}
	for i, rx := range rxPowerValues {
		n.rxPower.WithLabelValues(fmt.Sprintf("%d", i+1), device, serial, hostname, systemserial, slot).Set(rx)
	}
	for i, tx := range txPowerValues {
		n.txPower.WithLabelValues(fmt.Sprintf("%d", i+1), device, serial, hostname, systemserial, slot).Set(tx)
	}
	n.voltage.WithLabelValues(device, serial, hostname, systemserial, slot).Set(voltageValue)
	n.wavelength.WithLabelValues(device, serial, hostname, systemserial, slot).Set(wavelengthValue)
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
