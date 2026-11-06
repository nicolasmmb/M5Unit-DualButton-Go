package dualbutton

import "strings"

// Events is a bitmask of everything that happened to one button during a
// single Update call.
type Events uint8

const (
	// Pressed is emitted once, after the press debounce completed.
	Pressed Events = 1 << iota
	// Released is emitted once, after the release debounce completed.
	Released
	// Click is emitted once the click window closed with exactly one click.
	Click
	// DoubleClick is emitted for two clicks inside the click window.
	DoubleClick
	// MultiClick is emitted for three or more clicks inside the click window.
	MultiClick
	// HoldStart is emitted when a press lasts at least SetHoldTime.
	HoldStart
	// HoldTick repeats every SetHoldRepeat while the button stays held.
	HoldTick
	// HoldEnd is emitted when a hold is released.
	HoldEnd
)

// Has reports whether all bits in flag are set.
func (e Events) Has(flag Events) bool { return e&flag == flag }

// Any reports whether at least one event happened.
func (e Events) Any() bool { return e != 0 }

// String returns a "|"-separated list such as "Pressed|HoldStart".
// It allocates; do not use it in a hot path.
func (e Events) String() string {
	if e == 0 {
		return "none"
	}
	names := [...]struct {
		flag Events
		name string
	}{
		{Pressed, "Pressed"},
		{Released, "Released"},
		{Click, "Click"},
		{DoubleClick, "DoubleClick"},
		{MultiClick, "MultiClick"},
		{HoldStart, "HoldStart"},
		{HoldTick, "HoldTick"},
		{HoldEnd, "HoldEnd"},
	}
	var sb strings.Builder
	for _, n := range names {
		if e&n.flag != 0 {
			if sb.Len() > 0 {
				sb.WriteByte('|')
			}
			sb.WriteString(n.name)
		}
	}
	return sb.String()
}
