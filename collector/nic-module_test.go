package collector

import (
	"os"
	"testing"
	"github.com/stretchr/testify/assert"
)

func TestActiveEthernet(t *testing.T) {
	bytes, _ := os.ReadFile("testdata/mlxlink_active_ethernet.txt")
	testMlxlinkOutput := string(bytes)
	deviceInfo := DeviceInfo{
		pciAddress: "1b:00.0",
		mode: "ethernet",
		caName: "mlx5_0",
		netDev: "ib0",
	}
	result := parseOutput(testMlxlinkOutput, "hostname", "systemserial", "slot", deviceInfo)
	expected := PortMetrics{
		mode: "ethernet",
		caname: "mlx5_0",
		netdev: "ib0",
		serial: "5C2410312895",
		hostname: "hostname",
		product_serial: "systemserial",
		slot: "slot",
		state: 3,
		physicalState: 10,
		speed: 200000000000,
		biasCurrent: []float64{7.800,8.220,7.840,8.100},
		voltage: 3248900,
		wavelength: 850,
		transferDistance: 0.0,
		rxPower: []float64{-3,-2,-1,0},
		txPower: []float64{1,2,3,4},
		attenuation: nil,
	}
	assert.Equal(t, expected, result)
}

func TestActiveInfiniband(t *testing.T) {
	bytes, _ := os.ReadFile("testdata/mlxlink_active_infiniband.txt")
	testMlxlinkOutput := string(bytes)
	deviceInfo := DeviceInfo{
		pciAddress: "1b:00.0",
		mode: "ethernet",
		caName: "mlx5_0",
		netDev: "ib0",
	}
	result := parseOutput(testMlxlinkOutput, "hostname", "systemserial", "slot", deviceInfo)
	expected := PortMetrics{
		mode: "ethernet",
		caname: "mlx5_0",
		netdev: "ib0",
		serial: "5C2410312316",
		hostname: "hostname",
		product_serial: "systemserial",
		slot: "slot",
		state: 3,
		physicalState: 7,
		speed: 400000000000,
		biasCurrent: []float64{8.440,8.480,8.340,8.400},
		voltage: 3258200,
		wavelength: 858,
		transferDistance: 0.0,
		rxPower: []float64{-3,-2,-1,0},
		txPower: []float64{1,2,3,4},
		attenuation: nil,
	}
	assert.Equal(t, expected, result)
}
