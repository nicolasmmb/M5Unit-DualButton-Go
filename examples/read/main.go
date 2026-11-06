//go:build esp32

// Example: print the events of the M5Stack Unit Dual Button on PORT B.
//
// Wiring (M5StickC Plus2, PORT B / Grove):
//
//	yellow → GPIO26 → Red button
//	white  → GPIO36 → Blue button
package main

import (
	"time"

	dualbutton "github.com/nicolasmmb/M5Unit-DualButton-Go"
)

func main() {
	time.Sleep(time.Second)

	dualbutton.ConfigureInput(dualbutton.StickCPlus2Blue)
	dualbutton.ConfigureInput(dualbutton.StickCPlus2Red)

	d, err := dualbutton.New(dualbutton.StickCPlus2Blue, dualbutton.StickCPlus2Red)
	if err != nil {
		println("dualbutton:", err.Error())
		return
	}
	d.SetHoldRepeat(200 * time.Millisecond)

	println("dualbutton: ready (yellow=Red G26, white=Blue G36)")

	last := time.Now()
	for {
		blue, red := d.Update()
		if blue.Any() {
			report(dualbutton.Blue, blue, d)
		}
		if red.Any() {
			report(dualbutton.Red, red, d)
		}
		if time.Since(last) >= time.Second {
			last = time.Now()
			println("state: blue", d.Down(dualbutton.Blue), "red", d.Down(dualbutton.Red))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func report(b dualbutton.Button, ev dualbutton.Events, d *dualbutton.Device) {
	println(b.String(), ev.String())
	if ev.Has(dualbutton.HoldStart) || ev.Has(dualbutton.HoldEnd) {
		println("hold:", int64(d.Held(b)/time.Millisecond), "ms")
	}
}
