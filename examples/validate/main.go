//go:build esp32

// Guided validation firmware for the M5Stack Unit Dual Button.
//
// It first detects which Grove port the unit is plugged into, then walks a
// checklist of gestures. The firmware never guesses: every step only passes
// when the expected event pattern shows up in the driver.
//
// Feedback on the M5StickC Plus2 LCD backlight (GPIO27):
//
//	1 blink        new step started, waiting for your gesture
//	2 blinks       PASS
//	5 quick blinks FAIL/blocked
//
// Everything is also printed on the serial port with millisecond timestamps.
//
// Run it, then leave the serial port open: the checklist waits forever.
package main

import (
	"time"

	"machine"

	dualbutton "github.com/nicolasmmb/M5Unit-DualButton-Go"
)

const (
	blinkPin     = machine.GPIO27
	poll         = 10 * time.Millisecond
	heartbeat    = 5 * time.Second
	holdTime     = 800 * time.Millisecond
	holdRepeat   = 200 * time.Millisecond
	rapidPresses = 10
	simulWindow  = 400 * time.Millisecond
)

type pair struct {
	name string
	a, b machine.Pin
}

var candidates = []pair{
	{name: "PORT B", a: machine.GPIO26, b: machine.GPIO36},
	// PORT A: yellow (red button) is G32, white (blue button) is G33, same
	// Grove pin order as PORT B, so the pair is passed as (blue, red).
	{name: "PORT A", a: machine.GPIO33, b: machine.GPIO32},
}

type validator struct {
	port  string
	d     *dualbutton.Device
	start time.Time
	step  int
	fails int
}

func main() {
	time.Sleep(800 * time.Millisecond)
	blinkPin.Configure(machine.PinConfig{Mode: machine.PinOutput})
	blinkPin.Set(true)

	println("dualbutton: guided validation")
	println("backlight: 1 blink = new step, 2 = PASS, 5 = FAIL")

	p := detect()
	d, err := dualbutton.New(p.a, p.b)
	if err != nil {
		println("dualbutton: new:", err.Error())
		failFlash()
		return
	}
	d.SetHoldTime(holdTime)

	v := &validator{port: p.name, d: d, start: time.Now()}
	println("unit detected on", p.name)
	v.run()
	v.summary()
}

func (v *validator) now() int64 { return int64(time.Since(v.start) / time.Millisecond) }

func level(p machine.Pin) string {
	if p.Get() {
		return "H"
	}
	return "L"
}

// probe prints the electrical signature of both Grove ports. A port with the
// unit attached keeps both lines high even with the internal pull-down on
// (the unit has 10K to 5 V against the MCU's ~45K pull-down); an empty port
// follows the pull-down and reads low.
func probe() (string, machine.Pin, machine.Pin) {
	sig := ""
	var found machine.Pin
	var found2 machine.Pin
	for _, c := range candidates {
		c.a.Configure(machine.PinConfig{Mode: machine.PinInputPulldown})
		c.b.Configure(machine.PinConfig{Mode: machine.PinInputPulldown})
		time.Sleep(5 * time.Millisecond)
		a, b := level(c.a), level(c.b)
		c.a.Configure(machine.PinConfig{Mode: machine.PinInput})
		c.b.Configure(machine.PinConfig{Mode: machine.PinInput})
		sig += " " + c.name + "=" + a + b
		if a == "H" && b == "H" {
			found, found2 = c.a, c.b
		}
	}
	return sig, found, found2
}

func detect() pair {
	for i := 0; i < 60; i++ {
		sig, a, b := probe()
		if a != 0 {
			for _, c := range candidates {
				if c.a == a && c.b == b {
					println("probe:"+sig+" ->", c.name)
					blinkMany(2, 80)
					return c
				}
			}
		}
		println("detect: waiting for the unit |pulldown probe:" + sig)
		time.Sleep(2 * time.Second)
	}
	println("detect: unit not found, continuing anyway on PORT B")
	return candidates[0]
}

func (v *validator) prompt(text string) {
	v.step++
	println("")
	println("### STEP", v.step, "-", text)
	blinkPin.Set(false)
	time.Sleep(250 * time.Millisecond)
	blinkPin.Set(true)
}

func (v *validator) pass(detail string) {
	println(">>> PASS STEP", v.step, detail)
	blinkMany(2, 80)
}

func (v *validator) fail(detail string) {
	v.fails++
	println("!!! FAIL STEP", v.step, detail)
	failFlash()
}

// wait polls until done returns true, logging every event and a heartbeat.
func (v *validator) wait(done func(blue, red dualbutton.Events) bool) {
	last := time.Now()
	for {
		blue, red := v.d.Update()
		if blue.Any() {
			println(v.now(), "ms blue:", blue.String())
		}
		if red.Any() {
			println(v.now(), "ms red:", red.String())
		}
		if done(blue, red) {
			return
		}
		if time.Since(last) >= heartbeat {
			last = time.Now()
			println(v.now(), "ms step", v.step, "waiting on", v.port, "| blue down:",
				v.d.Down(dualbutton.Blue), "red down:", v.d.Down(dualbutton.Red))
		}
		time.Sleep(poll)
	}
}

func pick(b dualbutton.Button, blue, red dualbutton.Events) dualbutton.Events {
	if b == dualbutton.Blue {
		return blue
	}
	return red
}

func anyClick(ev dualbutton.Events) bool {
	return ev.Has(dualbutton.Click) || ev.Has(dualbutton.DoubleClick) || ev.Has(dualbutton.MultiClick)
}

func (v *validator) run() {
	v.click(dualbutton.Blue, "click the BLUE button once")
	v.click(dualbutton.Red, "click the RED button once")

	v.d.SetMaxClicks(2)
	v.double(dualbutton.Blue, "double click the BLUE button")
	v.double(dualbutton.Red, "double click the RED button")

	v.d.SetMaxClicks(4)
	v.multi("triple click the BLUE button")
	v.d.SetMaxClicks(2)

	v.hold("press and HOLD the BLUE button ~1.5 s, then release")
	v.d.SetHoldRepeat(holdRepeat)
	v.holdRepeat("press and HOLD the RED button ~2 s, then release")
	v.d.SetHoldRepeat(0)

	v.simultaneous("press BOTH buttons together, hold ~1 s, release")
	v.rapid("press the BLUE button 10 times, as fast as you can")
}

func (v *validator) click(b dualbutton.Button, text string) {
	v.prompt(text)
	done := false
	v.wait(func(blue, red dualbutton.Events) bool {
		if anyClick(pick(b, blue, red)) {
			done = true
		}
		return done
	})
	v.pass("click detected")
}

func (v *validator) double(b dualbutton.Button, text string) {
	v.prompt(text)
	done := false
	v.wait(func(blue, red dualbutton.Events) bool {
		if pick(b, blue, red).Has(dualbutton.DoubleClick) {
			done = true
		}
		return done
	})
	v.pass("double click detected")
}

func (v *validator) multi(text string) {
	v.prompt(text)
	done := false
	v.wait(func(blue, red dualbutton.Events) bool {
		if blue.Has(dualbutton.MultiClick) {
			done = true
		}
		return done
	})
	v.pass("multi click detected")
}

func (v *validator) hold(text string) {
	v.prompt(text)
	started, ended, clickAfter := false, false, false
	v.wait(func(blue, red dualbutton.Events) bool {
		if blue.Has(dualbutton.HoldStart) {
			started = true
		}
		if blue.Has(dualbutton.HoldEnd) {
			ended = true
		}
		if ended && anyClick(blue) {
			clickAfter = true
		}
		return ended
	})
	if !started {
		v.fail("HoldStart never emitted")
		return
	}
	if clickAfter {
		v.fail("Click emitted after the hold")
		return
	}
	v.pass("HoldStart + HoldEnd")
}

func (v *validator) holdRepeat(text string) {
	v.prompt(text)
	started, ended := false, false
	ticks := 0
	v.wait(func(blue, red dualbutton.Events) bool {
		ev := pick(dualbutton.Red, blue, red)
		if ev.Has(dualbutton.HoldStart) {
			started = true
		}
		if ev.Has(dualbutton.HoldTick) {
			ticks++
		}
		if ev.Has(dualbutton.HoldEnd) {
			ended = true
		}
		return ended
	})
	if !started {
		v.fail("HoldStart never emitted")
		return
	}
	if ticks < 3 {
		v.fail("fewer than 3 HoldTick events")
		return
	}
	v.pass("HoldStart + ticks + HoldEnd")
}

func (v *validator) simultaneous(text string) {
	v.prompt(text)
	var blueAt, redAt int64
	v.wait(func(blue, red dualbutton.Events) bool {
		if blue.Has(dualbutton.Pressed) {
			blueAt = v.now()
		}
		if red.Has(dualbutton.Pressed) {
			redAt = v.now()
		}
		if blueAt == 0 || redAt == 0 {
			return false
		}
		return !v.d.Down(dualbutton.Blue) && !v.d.Down(dualbutton.Red)
	})
	delta := blueAt - redAt
	if delta < 0 {
		delta = -delta
	}
	if time.Duration(delta)*time.Millisecond > simulWindow {
		v.fail("presses were not simultaneous")
		return
	}
	v.pass("both buttons pressed together")
}

func (v *validator) rapid(text string) {
	v.prompt(text)
	presses := 0
	v.wait(func(blue, red dualbutton.Events) bool {
		if blue.Has(dualbutton.Released) {
			presses++
		}
		return presses >= rapidPresses
	})
	v.pass("rapid presses counted")
}

func (v *validator) summary() {
	println("")
	println("### DONE | fails:", v.fails)
	if v.fails == 0 {
		println("RESULT: PASS")
		blinkMany(3, 400)
	} else {
		println("RESULT: FAIL")
		failFlash()
	}
	for {
		v.d.Update()
		time.Sleep(poll)
	}
}

func blinkMany(n, ms int) {
	for i := 0; i < n; i++ {
		blinkPin.Set(false)
		time.Sleep(time.Duration(ms) * time.Millisecond)
		blinkPin.Set(true)
		time.Sleep(time.Duration(ms) * time.Millisecond)
	}
}

func failFlash() { blinkMany(5, 60) }
