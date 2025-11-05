package collector

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
)

const (
	pcideviceSubsystem = "pcidevice"
)

var (
	pciIdsPaths = []string{
		"/usr/share/misc/pci.ids",
		"/usr/share/hwdata/pci.ids",
	}

	pcideviceLabelNames = []string{"segment", "bus", "device", "function"}
)

// PCIDeviceCollector implements prometheus.Collector
type PCIDeviceCollector struct {
	pciDeviceInfoDesc *prometheus.Desc
	pciVendors        map[string]string
	pciDevices        map[string]map[string]string
	pciSubsystems     map[string]map[string]string
	pciClasses        map[string]string
	pciSubclasses     map[string]string
	pciProgIfs        map[string]string
}

// NewPCIDeviceExporter creates a new exporter with metric descriptions
func NewPCIDeviceCollector(namespace string) (*PCIDeviceCollector, error) {
	c := &PCIDeviceCollector{}

	// Build label names based on whether name resolution is enabled
	labelNames := append(pcideviceLabelNames,
		[]string{"parent_segment", "parent_bus", "parent_device", "parent_function",
			"class_id", "vendor_id", "device_id", "subsystem_vendor_id", "subsystem_device_id", "revision", "vendor_name",
			"device_name", "subsystem_vendor_name", "subsystem_device_name", "class_name"}...)

	c.pciDeviceInfoDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, pcideviceSubsystem, "info"),
		"Non-numeric data from /sys/bus/pci/devices/<location>, value is always 1.",
		labelNames,
		nil,
	)

	c.loadPCIIds()

	return c, nil
}

// Describe sends the metric descriptions to Prometheus
func (e *PCIDeviceCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- e.pciDeviceInfoDesc
}

// Collect runs on every /metrics scrape
func (e *PCIDeviceCollector) Collect(ch chan<- prometheus.Metric) {
	start := time.Now()

	// Collect device info

	devices, err := GetPciDevices("/sys")
	if err != nil {
		log.Errorf("Error reading device information: %s", err)
	}

	duration := time.Since(start).Seconds()

	for _, device := range devices {
		// The device location is represented in separated format.
		values := device.Location.Strings()
		if device.ParentLocation != nil {
			values = append(values, device.ParentLocation.Strings()...)
		} else {
			values = append(values, []string{"*", "*", "*", "*"}...)
		}

		// Add basic device information
		classID := fmt.Sprintf("0x%06x", device.Class)
		vendorID := fmt.Sprintf("0x%04x", device.Vendor)
		deviceID := fmt.Sprintf("0x%04x", device.Device)
		subsysVendorID := fmt.Sprintf("0x%04x", device.SubsystemVendor)
		subsysDeviceID := fmt.Sprintf("0x%04x", device.SubsystemDevice)

		values = append(values, classID, vendorID, deviceID, subsysVendorID, subsysDeviceID, fmt.Sprintf("0x%02x", device.Revision))

		// Add name values if name resolution is enabled
		vendorName := e.getPCIVendorName(vendorID)
		deviceName := e.getPCIDeviceName(vendorID, deviceID)
		subsysVendorName := e.getPCIVendorName(subsysVendorID)
		subsysDeviceName := e.getPCISubsystemName(vendorID, deviceID, subsysVendorID, subsysDeviceID)
		className := e.getPCIClassName(classID)

		values = append(values, vendorName, deviceName, subsysVendorName, subsysDeviceName, className)

		// Send the metrics
		ch <- prometheus.MustNewConstMetric(e.pciDeviceInfoDesc, prometheus.GaugeValue, 1.0, values...)
	}

	log.Printf("Scraped metrics: latency=%.3fs", duration)
}

const pciDevicesPath = "bus/pci/devices"

// PciDeviceLocation represents the location of the device attached.
// "0000:00:00.0" represents Segment:Bus:Device.Function .
type PciDeviceLocation struct {
	Segment  int
	Bus      int
	Device   int
	Function int
}

func (pdl PciDeviceLocation) String() string {
	return fmt.Sprintf("%04x:%02x:%02x:%x", pdl.Segment, pdl.Bus, pdl.Device, pdl.Function)
}

func (pdl PciDeviceLocation) Strings() []string {
	return []string{
		fmt.Sprintf("%04x", pdl.Segment),
		fmt.Sprintf("%02x", pdl.Bus),
		fmt.Sprintf("%02x", pdl.Device),
		fmt.Sprintf("%x", pdl.Function),
	}
}

// PciDevice contains info from files in /sys/bus/pci/devices for a
// single PCI device.
type PciDevice struct {
	Location       PciDeviceLocation
	ParentLocation *PciDeviceLocation

	Class           uint32 // /sys/bus/pci/devices/<Location>/class
	Vendor          uint32 // /sys/bus/pci/devices/<Location>/vendor
	Device          uint32 // /sys/bus/pci/devices/<Location>/device
	SubsystemVendor uint32 // /sys/bus/pci/devices/<Location>/subsystem_vendor
	SubsystemDevice uint32 // /sys/bus/pci/devices/<Location>/subsystem_device
	Revision        uint32 // /sys/bus/pci/devices/<Location>/revision
}

func (pd PciDevice) Name() string {
	return pd.Location.String()
}

// PciDevices is a collection of every PCI device in
// /sys/bus/pci/devices .
//
// The map keys are the location of PCI devices.
type PciDevices map[string]PciDevice

// PciDevices returns info for all PCI devices read from
// /sys/bus/pci/devices .
func GetPciDevices(basePath string) (PciDevices, error) {
	path := path.Join(basePath, pciDevicesPath)

	dirs, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	pciDevs := make(PciDevices, len(dirs))
	for _, d := range dirs {
		device, err := parsePciDevice(basePath, d.Name())
		if err != nil {
			return nil, err
		}

		pciDevs[device.Name()] = *device
	}

	return pciDevs, nil
}

// Parse one PCI device
// Refer to https://docs.kernel.org/PCI/sysfs-pci.html
func parsePciDevice(basePath, name string) (*PciDevice, error) {
	devicePath := path.Join(basePath, pciDevicesPath, name)
	// the file must be symbolic link.
	realPath, err := os.Readlink(devicePath)
	if err != nil {
		return nil, fmt.Errorf("failed to readlink: %w", err)
	}

	// parse device location from realpath
	// like "../../../devices/pci0000:00/0000:00:02.5/0000:04:00.0"
	deviceLocStr := path.Base(realPath)
	parentDeviceLocStr := path.Base(path.Dir(realPath))

	deviceLoc, err := parsePciDeviceLocation(deviceLocStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse device location:%q %w", deviceLoc, err)
	}

	// the parent device may have "pci" prefix.
	// this is not pci device like bridges.
	// we ignore such location to avoid confusion.
	// TODO: is it really ok?
	var parentDeviceLoc *PciDeviceLocation
	if !strings.HasPrefix(parentDeviceLocStr, "pci") {
		parentDeviceLoc, err = parsePciDeviceLocation(parentDeviceLocStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse parent device location %q: %w", parentDeviceLocStr, err)
		}
	}

	device := &PciDevice{
		Location:       *deviceLoc,
		ParentLocation: parentDeviceLoc,
	}

	// These files must exist in a device directory.
	for _, f := range [...]string{"class", "vendor", "device", "subsystem_vendor", "subsystem_device", "revision"} {
		name := path.Join(devicePath, f)
		valueStr, err := SysReadFile(name)
		if err != nil {
			return nil, fmt.Errorf("failed to read file %q: %w", name, err)
		}
		value, err := strconv.ParseInt(valueStr, 0, 32)
		if err != nil {
			return nil, fmt.Errorf("failed to parse %q: %w", valueStr, err)
		}

		switch f {
		case "class":
			device.Class = uint32(value)
		case "vendor":
			device.Vendor = uint32(value)
		case "device":
			device.Device = uint32(value)
		case "subsystem_vendor":
			device.SubsystemVendor = uint32(value)
		case "subsystem_device":
			device.SubsystemDevice = uint32(value)
		case "revision":
			device.Revision = uint32(value)
		default:
			return nil, fmt.Errorf("unknown file %q", f)
		}
	}

	return device, nil
}

func parsePciDeviceLocation(loc string) (*PciDeviceLocation, error) {
	locs := strings.Split(loc, ":")
	if len(locs) != 3 {
		return nil, fmt.Errorf("invalid location '%s'", loc)
	}
	locs = append(locs[0:2], strings.Split(locs[2], ".")...)
	if len(locs) != 4 {
		return nil, fmt.Errorf("invalid location '%s'", loc)
	}

	seg, err := strconv.ParseInt(locs[0], 16, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid segment: %w", err)
	}
	bus, err := strconv.ParseInt(locs[1], 16, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid bus: %w", err)
	}
	device, err := strconv.ParseInt(locs[2], 16, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid device: %w", err)
	}
	function, err := strconv.ParseInt(locs[3], 16, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid function: %w", err)
	}

	return &PciDeviceLocation{
		Segment:  int(seg),
		Bus:      int(bus),
		Device:   int(device),
		Function: int(function),
	}, nil
}

// SysReadFile is a simplified os.ReadFile that invokes syscall.Read directly.
// https://github.com/prometheus/node_exporter/pull/728/files
//
// Note that this function will not read files larger than 128 bytes.
func SysReadFile(file string) (string, error) {
	f, err := os.Open(file)
	if err != nil {
		return "", err
	}
	defer f.Close()

	// On some machines, hwmon drivers are broken and return EAGAIN.  This causes
	// Go's os.ReadFile implementation to poll forever.
	//
	// Since we either want to read data or bail immediately, do the simplest
	// possible read using syscall directly.
	const sysFileBufferSize = 128
	b := make([]byte, sysFileBufferSize)
	n, err := syscall.Read(int(f.Fd()), b)
	if err != nil {
		return "", err
	}

	return string(bytes.TrimSpace(b[:n])), nil
}

// loadPCIIds loads PCI device information from pci.ids file
func (c *PCIDeviceCollector) loadPCIIds() {
	var file *os.File
	var err error

	c.pciVendors = make(map[string]string)
	c.pciDevices = make(map[string]map[string]string)
	c.pciSubsystems = make(map[string]map[string]string)
	c.pciClasses = make(map[string]string)
	c.pciSubclasses = make(map[string]string)
	c.pciProgIfs = make(map[string]string)

	// Try each possible default path
	for _, path := range pciIdsPaths {
		file, err = os.Open(path)
		if err == nil {
			log.Debug("Loading PCI IDs from default path", "path", path)
			break
		}
	}
	if err != nil {
		log.Debug("Failed to open any default PCI IDs file", "error", err)
		return
	}

	defer file.Close()

	scanner := bufio.NewScanner(file)
	var currentVendor, currentDevice, currentBaseClass, currentSubclass string
	var inClassContext bool

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Handle class lines (starts with 'C')
		if strings.HasPrefix(line, "C ") {
			parts := strings.SplitN(line, "  ", 2)
			if len(parts) >= 2 {
				classID := strings.TrimSpace(parts[0][1:]) // Remove 'C' prefix
				className := strings.TrimSpace(parts[1])
				c.pciClasses[classID] = className
				currentBaseClass = classID
				inClassContext = true
			}
			continue
		}

		// Handle subclass lines (single tab after class)
		if strings.HasPrefix(line, "\t") && !strings.HasPrefix(line, "\t\t") && inClassContext {
			line = strings.TrimPrefix(line, "\t")
			parts := strings.SplitN(line, "  ", 2)
			if len(parts) >= 2 && currentBaseClass != "" {
				subclassID := strings.TrimSpace(parts[0])
				subclassName := strings.TrimSpace(parts[1])
				// Store as base class + subclass (e.g., "0100" for SCSI storage controller)
				fullClassID := currentBaseClass + subclassID
				c.pciSubclasses[fullClassID] = subclassName
				currentSubclass = fullClassID
			}
			continue
		}

		// Handle programming interface lines (double tab after subclass)
		if strings.HasPrefix(line, "\t\t") && !strings.HasPrefix(line, "\t\t\t") && inClassContext {
			line = strings.TrimPrefix(line, "\t\t")
			parts := strings.SplitN(line, "  ", 2)
			if len(parts) >= 2 && currentSubclass != "" {
				progIfID := strings.TrimSpace(parts[0])
				progIfName := strings.TrimSpace(parts[1])
				// Store as base class + subclass + programming interface (e.g., "010802" for NVM Express)
				fullClassID := currentSubclass + progIfID
				c.pciProgIfs[fullClassID] = progIfName
			}
			continue
		}

		// Handle vendor lines (no leading whitespace, not starting with 'C')
		if !strings.HasPrefix(line, "\t") && !strings.HasPrefix(line, "C ") {
			parts := strings.SplitN(line, "  ", 2)
			if len(parts) >= 2 {
				currentVendor = strings.TrimSpace(parts[0])
				c.pciVendors[currentVendor] = strings.TrimSpace(parts[1])
				currentDevice = ""
				inClassContext = false
			}
			continue
		}

		// Handle device lines (single tab)
		if strings.HasPrefix(line, "\t") && !strings.HasPrefix(line, "\t\t") {
			line = strings.TrimPrefix(line, "\t")
			parts := strings.SplitN(line, "  ", 2)
			if len(parts) >= 2 && currentVendor != "" {
				currentDevice = strings.TrimSpace(parts[0])
				if c.pciDevices[currentVendor] == nil {
					c.pciDevices[currentVendor] = make(map[string]string)
				}
				c.pciDevices[currentVendor][currentDevice] = strings.TrimSpace(parts[1])
			}
			continue
		}

		// Handle subsystem lines (double tab)
		if strings.HasPrefix(line, "\t\t") {
			line = strings.TrimPrefix(line, "\t\t")
			parts := strings.SplitN(line, "  ", 2)
			if len(parts) >= 2 && currentVendor != "" && currentDevice != "" {
				subsysID := strings.TrimSpace(parts[0])
				subsysName := strings.TrimSpace(parts[1])
				key := fmt.Sprintf("%s:%s", currentVendor, currentDevice)
				if c.pciSubsystems[key] == nil {
					c.pciSubsystems[key] = make(map[string]string)
				}
				// Convert subsystem ID from "vendor device" format to "vendor:device" format
				subsysParts := strings.Fields(subsysID)
				if len(subsysParts) == 2 {
					subsysKey := fmt.Sprintf("%s:%s", subsysParts[0], subsysParts[1])
					c.pciSubsystems[key][subsysKey] = subsysName
				}
			}
		}
	}

	// Debug summary
	totalDevices := 0
	for _, devices := range c.pciDevices {
		totalDevices += len(devices)
	}
	totalSubsystems := 0
	for _, subsystems := range c.pciSubsystems {
		totalSubsystems += len(subsystems)
	}

	log.Debug("Loaded PCI device data",
		"vendors", len(c.pciVendors),
		"devices", totalDevices,
		"subsystems", totalSubsystems,
		"classes", len(c.pciClasses),
		"subclasses", len(c.pciSubclasses),
		"progIfs", len(c.pciProgIfs),
	)
}

// getPCIVendorName converts PCI vendor ID to human-readable string using pci.ids
func (c *PCIDeviceCollector) getPCIVendorName(vendorID string) string {
	// Remove "0x" prefix if present
	vendorID = strings.TrimPrefix(vendorID, "0x")
	vendorID = strings.ToLower(vendorID)

	if name, ok := c.pciVendors[vendorID]; ok {
		return name
	}
	return vendorID // Return ID if name not found
}

// getPCIDeviceName converts PCI device ID to human-readable string using pci.ids
func (c *PCIDeviceCollector) getPCIDeviceName(vendorID, deviceID string) string {
	// Remove "0x" prefix if present
	vendorID = strings.TrimPrefix(vendorID, "0x")
	deviceID = strings.TrimPrefix(deviceID, "0x")
	vendorID = strings.ToLower(vendorID)
	deviceID = strings.ToLower(deviceID)

	if devices, ok := c.pciDevices[vendorID]; ok {
		if name, ok := devices[deviceID]; ok {
			return name
		}
	}
	return deviceID // Return ID if name not found
}

// getPCISubsystemName converts PCI subsystem ID to human-readable string using pci.ids
func (c *PCIDeviceCollector) getPCISubsystemName(vendorID, deviceID, subsysVendorID, subsysDeviceID string) string {
	// Normalize all IDs
	vendorID = strings.TrimPrefix(vendorID, "0x")
	deviceID = strings.TrimPrefix(deviceID, "0x")
	subsysVendorID = strings.TrimPrefix(subsysVendorID, "0x")
	subsysDeviceID = strings.TrimPrefix(subsysDeviceID, "0x")

	key := fmt.Sprintf("%s:%s", vendorID, deviceID)
	subsysKey := fmt.Sprintf("%s:%s", subsysVendorID, subsysDeviceID)

	if subsystems, ok := c.pciSubsystems[key]; ok {
		if name, ok := subsystems[subsysKey]; ok {
			return name
		}
	}
	return subsysDeviceID
}

// getPCIClassName converts PCI class ID to human-readable string using pci.ids
func (c *PCIDeviceCollector) getPCIClassName(classID string) string {
	// Remove "0x" prefix if present and normalize
	classID = strings.TrimPrefix(classID, "0x")
	classID = strings.ToLower(classID)

	// Try to find the programming interface first (6 digits: base class + subclass + programming interface)
	if len(classID) >= 6 {
		progIf := classID[:6]
		if className, exists := c.pciProgIfs[progIf]; exists {
			return className
		}
	}

	// Try to find the subclass (4 digits: base class + subclass)
	if len(classID) >= 4 {
		subclass := classID[:4]
		if className, exists := c.pciSubclasses[subclass]; exists {
			return className
		}
	}

	// If not found, try with just the base class (first 2 digits)
	if len(classID) >= 2 {
		baseClass := classID[:2]
		if className, exists := c.pciClasses[baseClass]; exists {
			return className
		}
	}

	// Return the original class ID if not found
	return "Unknown class (" + classID + ")"
}
