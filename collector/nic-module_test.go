package collector

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestActiveEthernet(t *testing.T) {
	bytes, _ := os.ReadFile("testdata/mlxlink_active_ethernet.json")
	testMlxlinkOutput := string(bytes)
	deviceInfo := DeviceInfo{
		pciAddress: "1b:00.0",
		mode:       "ethernet",
		caName:     "mlx5_0",
		netDev:     "ib0",
	}
	result := parseOutput(testMlxlinkOutput, "hostname", "systemserial", "slot", deviceInfo)
	expected := PortMetrics{
		hostname:         "hostname",
		systemserial:     "systemserial",
		slot:             "slot",
		mode:             "ethernet",
		caname:           "mlx5_0",
		netdev:           "ib0",
		state:            3,
		physicalState:    10,
		speed:            200000000000,
		width:            4,
		serial:           "5C2410312895",
		vendor:           "Firmus",
		partNumber:       "QSFP200I-SR4-5M",
		moduleState:      3,
		dataPathState:    []float64{4, 4, 4, 4},
		biasCurrent:      []float64{7.640, 8.100, 7.740, 8.180},
		temperature:      41,
		voltage:          3248.7,
		wavelength:       850,
		transferDistance: 0.0,
		rxPower:          []float64{-3, -2, -1, 0},
		txPower:          []float64{1, 2, 3, 4},
		snrMedia:         []float64{},
		snrHost:          []float64{},
		attenuation:      map[string]float64{},
		effectiveBer:     15e-255,
		effectiveErrors:  5,
		rawBer:           8e-13,
		symbolBer:        0,
		symbolErrors:     0,
		linkDown:         0,
		linkRecovery:     0,
		lastClearTime:    6863.7 * 60,
		rawErrors:        []float64{54126, 3782, 1578, 5277},
	}
	assert.Equal(t, expected, result)
}

func TestActiveInfiniband(t *testing.T) {
	bytes, _ := os.ReadFile("testdata/mlxlink_active_infiniband.json")
	testMlxlinkOutput := string(bytes)
	deviceInfo := DeviceInfo{
		pciAddress: "1b:00.0",
		mode:       "infiniband",
		caName:     "mlx5_0",
		netDev:     "ib0",
	}
	result := parseOutput(testMlxlinkOutput, "hostname", "systemserial", "slot", deviceInfo)
	expected := PortMetrics{
		hostname:         "hostname",
		systemserial:     "systemserial",
		slot:             "slot",
		mode:             "infiniband",
		caname:           "mlx5_0",
		netdev:           "ib0",
		state:            3,
		physicalState:    7,
		speed:            400000000000,
		width:            4,
		serial:           "5C2410312316",
		vendor:           "Firmus",
		partNumber:       "OSFP400I-SR4-5M",
		moduleState:      3,
		dataPathState:    []float64{4, 4, 4, 4},
		biasCurrent:      []float64{8.540, 8.600, 8.630, 8.660},
		temperature:      45,
		voltage:          3258.9,
		wavelength:       858,
		transferDistance: 0.0,
		rxPower:          []float64{-3, -2, -1, 0},
		txPower:          []float64{1, 2, 3, 4},
		snrMedia:         []float64{4, 3, 2, 1},
		snrHost:          []float64{8, 7, 6, 5},
		attenuation:      map[string]float64{},
		effectiveBer:     15e-255,
		effectiveErrors:  1,
		rawBer:           2e-10,
		symbolBer:        15e-255,
		symbolErrors:     4,
		linkDown:         2,
		linkRecovery:     3,
		lastClearTime:    6750.4 * 60,
		rawErrors:        []float64{6045465, 14059590, 18460013, 5651086},
	}
	assert.Equal(t, expected, result)
}

func TestParseSlots(t *testing.T) {
	bytes, _ := os.ReadFile("testdata/dmidecode_output.txt")
	testDmiDecodeOutput := string(bytes)
	result := parseSlots(testDmiDecodeOutput)
	expected := Slots{
		SlotInfo{
			"PCIe Slot 38",
			"0000:1a:00.0",
			"38",
		},
		SlotInfo{
			"PCIe Slot 40",
			"0000:1b:00.0",
			"40",
		},
		SlotInfo{
			"PCIe Slot 39",
			"0000:3c:00.0",
			"39",
		},
		SlotInfo{
			"PCIe Slot 37",
			"0000:4d:00.0",
			"37",
		},
		SlotInfo{
			"PCIe Slot 36",
			"0000:5e:00.0",
			"36",
		},
		SlotInfo{
			"PCIe Slot 32",
			"0000:9c:00.0",
			"32",
		},
		SlotInfo{
			"PCIe Slot 31",
			"",
			"31",
		},
		SlotInfo{
			"PCIe Slot 33",
			"0000:bc:00.0",
			"33",
		},
		SlotInfo{
			"PCIe Slot 34",
			"0000:cc:00.0",
			"34",
		},
		SlotInfo{
			"PCIe Slot 35",
			"0000:dc:00.0",
			"35",
		},
		SlotInfo{
			"PCIe Slot 28",
			"0000:19:00.0",
			"28",
		},
		SlotInfo{
			"PCIe Slot 24",
			"0000:3b:00.0",
			"24",
		},
		SlotInfo{
			"PCIe Slot 23",
			"0000:4c:00.0",
			"23",
		},
		SlotInfo{
			"PCIe Slot 27",
			"0000:5d:00.0",
			"27",
		},
		SlotInfo{
			"PCIe Slot 25",
			"0000:9b:00.0",
			"25",
		},
		SlotInfo{
			"PCIe Slot 21",
			"0000:bb:00.0",
			"21",
		},
		SlotInfo{
			"PCIe Slot 26",
			"0000:cb:00.0",
			"26",
		},
		SlotInfo{
			"PCIe Slot 22",
			"0000:db:00.0",
			"22",
		},
		SlotInfo{
			"PCIe SSD Slot 7 in Bay 1",
			"0000:18:00.0",
			"7",
		},
		SlotInfo{
			"PCIe SSD Slot 6 in Bay 1",
			"0000:3a:00.0",
			"6",
		},
		SlotInfo{
			"PCIe SSD Slot 5 in Bay 1",
			"0000:4b:00.0",
			"5",
		},
		SlotInfo{
			"PCIe SSD Slot 4 in Bay 1",
			"0000:5c:00.0",
			"4",
		},
		SlotInfo{
			"PCIe SSD Slot 0 in Bay 1",
			"0000:9a:00.0",
			"0",
		},
		SlotInfo{
			"PCIe SSD Slot 1 in Bay 1",
			"0000:ba:00.0",
			"1",
		},
		SlotInfo{
			"PCIe SSD Slot 3 in Bay 1",
			"0000:ca:00.0",
			"3",
		},
		SlotInfo{
			"PCIe SSD Slot 2 in Bay 1",
			"0000:da:00.0",
			"2",
		},
	}
	assert.Equal(t, expected, result)

}

var testSlots = Slots{
	SlotInfo{
		"PCIe Slot 38",
		"0000:1a:00.0",
		"38",
	},
	SlotInfo{
		"PCIe Slot 40",
		"0000:1b:00.0",
		"40",
	},
	SlotInfo{
		"PCIe Slot 39",
		"0000:3c:00.0",
		"39",
	},
}

func TestGetSlotInfoPort0(t *testing.T) {
	result := testSlots.getSlot("0000:1b:00.0")
	assert.Equal(t, "40", result)

}

func TestGetSlotInfoPort1(t *testing.T) {
	result := testSlots.getSlot("0000:1b:00.1")
	assert.Equal(t, "40", result)

}
