package dualbutton

// Layout describes where the unit is plugged: a name for the connector and the
// two pins in logical order (Blue is the white wire, Red is the yellow one).
//
// The pins are the Pin interface, so a Layout can be built from machine.Pin on
// any board and from a fake pin in host tests.
type Layout struct {
	Name      string
	Blue, Red Pin
}

// NewLayout returns a Device for the layout. The pins must already be
// configured as inputs; on ESP32 and RP2040 use Layout.Configure, or
// ConfigureInput per pin.
func NewLayout(l Layout) (*Device, error) {
	return New(l.Blue, l.Red)
}

// Choose returns the first candidate whose probe found both pins high.
//
// It holds the decision of Detect, so it can be tested on the host. Detect
// fills high by pulling each pair down: the unit divider (10K to 5 V, 20K to
// GND) keeps the connected pair high, an empty connector follows the pull-down.
func Choose(cands []Layout, high []bool) (Layout, bool) {
	for i, l := range cands {
		if i < len(high) && high[i] {
			return l, true
		}
	}
	return Layout{}, false
}
