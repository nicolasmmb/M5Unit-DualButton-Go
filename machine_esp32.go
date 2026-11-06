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

// ConfigureInput puts p in plain input mode, without any internal pull.
//
// That is what the unit needs: it has its own 10K pull-up to 5 V plus a 20K
// pull-down divider, so the pin idles at ~3.3 V. It is also the only mode
// GPIO36 accepts (input-only, no internal pull resistor).
//
// For a bare button with no divider, configure the pin yourself with
// machine.PinInputPullup (or PinInputPulldown with SetActiveLow(false)).
func ConfigureInput(p machine.Pin) {
	p.Configure(machine.PinConfig{Mode: machine.PinInput})
}
