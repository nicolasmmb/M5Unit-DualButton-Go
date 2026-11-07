//go:build esp32

package dualbutton

import "machine"

// Pins of an M5StickC Plus2 PORT B (Grove) wired to the Unit Dual Button.
const (
	// StickCPlus2Red is the yellow wire (Red button).
	StickCPlus2Red = machine.GPIO26
	// StickCPlus2Blue is the white wire (Blue button).
	StickCPlus2Blue = machine.GPIO36
)

// Layouts of the M5 boards whose PORT B carries the unit.
var (
	// StickCPlus2PortB is the side Grove connector of an M5StickC Plus2.
	StickCPlus2PortB = Layout{Name: "StickC Plus2 PORT B", Blue: machine.GPIO36, Red: machine.GPIO26}
	// StickCPlus2PortA is the I2C connector; the unit works there too.
	StickCPlus2PortA = Layout{Name: "StickC Plus2 PORT A", Blue: machine.GPIO33, Red: machine.GPIO32}
)
