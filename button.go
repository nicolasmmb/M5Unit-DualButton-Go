package dualbutton

import "strconv"

// String returns "Blue" or "Red". Unknown values return "Button(n)".
// It allocates for unknown values only.
func (b Button) String() string {
	switch b {
	case Blue:
		return "Blue"
	case Red:
		return "Red"
	default:
		return "Button(" + strconv.FormatUint(uint64(b), 10) + ")"
	}
}
