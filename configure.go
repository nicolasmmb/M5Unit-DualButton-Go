//go:build esp32 || esp32c3 || esp32s3 || rp2040

package dualbutton

import (
	"machine"
	"time"
)

// Configure puts both pins of the layout in plain input mode, without internal
// pull. The unit has its own 10K/20K divider, and GPIO36 on the classic ESP32
// accepts no pull at all.
func (l Layout) Configure() {
	for _, pin := range [...]Pin{l.Blue, l.Red} {
		if p, ok := pin.(machine.Pin); ok {
			p.Configure(machine.PinConfig{Mode: machine.PinInput})
		}
	}
}

// Detect returns the first layout that looks connected.
//
// Each candidate is pulled down: a connected unit keeps both pins high through
// its 10K/5 V divider, an empty connector falls. The pins are restored to plain
// input afterwards.
func Detect(cands ...Layout) (Layout, bool) {
	high := make([]bool, len(cands))
	for i, l := range cands {
		blue, okBlue := l.Blue.(machine.Pin)
		red, okRed := l.Red.(machine.Pin)
		if !okBlue || !okRed {
			continue
		}
		blue.Configure(machine.PinConfig{Mode: machine.PinInputPulldown})
		red.Configure(machine.PinConfig{Mode: machine.PinInputPulldown})
		time.Sleep(5 * time.Millisecond)
		high[i] = blue.Get() && red.Get()
		l.Configure()
	}
	return Choose(cands, high)
}

// ConfigureInput puts p in plain input mode, without internal pull. The unit
// has its own divider, and GPIO36 on the classic ESP32 accepts no pull at all.
func ConfigureInput(p machine.Pin) {
	p.Configure(machine.PinConfig{Mode: machine.PinInput})
}
