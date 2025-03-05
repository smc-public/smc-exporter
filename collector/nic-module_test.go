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
		lastClearTime:    6863.7*60,
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
		snrMedia:         []float64{4,3,2,1},
		snrHost:          []float64{8,7,6,5},
		attenuation:      map[string]float64{},
		effectiveBer:     15e-255,
		effectiveErrors:  1,
		rawBer:           2e-10,
		symbolBer:        15e-255,
		symbolErrors:     4,
		linkDown:         2,
		linkRecovery:     3,
		lastClearTime:    6750.4*60,
		rawErrors:        []float64{6045465, 14059590, 18460013, 5651086},
	}
	assert.Equal(t, expected, result)
}
