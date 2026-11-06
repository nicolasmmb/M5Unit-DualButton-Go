// Package dualbutton drives the M5Stack Unit Dual Button (SKU U025).
//
// The unit has no controller chip: it is two tactile switches wired to a Grove
// PORT B connector, each with an on-board 10K/20K divider (5 V → 3.3 V) and
// active-low output. This package therefore needs nothing but two GPIO inputs:
//
//	blue := machine.GPIO36 // white wire
//	red := machine.GPIO26  // yellow wire
//	dualbutton.ConfigureInput(blue) // //go:build esp32 helper
//	dualbutton.ConfigureInput(red)
//
//	d, err := dualbutton.New(blue, red)
//	for {
//	    b, r := d.Update()
//	    if b.Has(dualbutton.Click) {
//	        println("blue click")
//	    }
//	    _ = r
//	    time.Sleep(10 * time.Millisecond)
//	}
//
// Pin is the only hardware dependency, so the whole state machine also runs on
// a desktop host with fake pins and a fake clock. Update performs no heap
// allocation.
package dualbutton

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Pin is the subset of machine.Pin used by the driver. machine.Pin satisfies
// it directly (Get is a value method), so no adapter is needed.
type Pin interface {
	// Get reports the logic level currently present on the pin:
	// true for high, false for low.
	Get() bool
}

// Clock is a monotonic time source. The default implementation is backed by
// time.Now; inject a fake one to test transitions without sleeping.
type Clock interface {
	Now() time.Duration
}

// Button identifies one of the two switches of the unit.
type Button uint8

const (
	// Blue is the white wire of the Grove cable (GPIO36 on an M5StickC Plus2).
	Blue Button = iota
	// Red is the yellow wire of the Grove cable (GPIO26 on an M5StickC Plus2).
	Red
)

const numButtons = 2

// Timing defaults match the M5Stack UIFlow2 hardware.Button driver.
const (
	// DefaultDebounce is the time a press or release must stay stable.
	DefaultDebounce = 50 * time.Millisecond
	// DefaultClickWindow is how long a click waits for another click to turn
	// into a double click.
	DefaultClickWindow = 400 * time.Millisecond
	// DefaultHoldTime is the press duration that produces HoldStart.
	DefaultHoldTime = 800 * time.Millisecond
	// DefaultMaxClicks is how many clicks close the window immediately instead
	// of waiting for DefaultClickWindow. 2 means "single waits, double fires at
	// once". Use 1 to fire Click immediately and disable double click.
	DefaultMaxClicks uint8 = 2
	// MinDebounce is the lowest accepted debounce value.
	MinDebounce = time.Millisecond
	// MaxClicks is the highest accepted value for SetMaxClicks.
	MaxClicks uint8 = 4
)

// Errors returned by the constructor. They are sentinels: compare with errors.Is.
var (
	ErrNilPin = errors.New("dualbutton: nil pin")
)

type config struct {
	debounce    time.Duration
	clickWindow time.Duration
	holdTime    time.Duration
	holdRepeat  time.Duration
	maxClicks   uint8
	activeLow   bool
}

func defaultConfig() config {
	return config{
		debounce:    DefaultDebounce,
		clickWindow: DefaultClickWindow,
		holdTime:    DefaultHoldTime,
		holdRepeat:  0, // disabled by default
		maxClicks:   DefaultMaxClicks,
		activeLow:   true,
	}
}

// Device is the dual button state machine. The zero value is not usable: call
// New. A Device is safe for concurrent use; Update is serialized by a mutex.
type Device struct {
	mu   sync.Mutex
	pin  [numButtons]Pin
	clk  Clock
	cfg  config
	b    [numButtons]btnState
	boot time.Time
}

// New returns a Device for the two unit pins, blue (white wire) and red
// (yellow wire). Pins must already be configured as inputs; the unit has its
// own 10K/20K divider, so no internal pull-up is required (and GPIO36 on the
// ESP32 has none).
//
// New does not touch the hardware: the first Update records the baseline
// level, so a button held during boot does not produce a phantom Pressed.
func New(blue, red Pin) (*Device, error) {
	if blue == nil || red == nil {
		return nil, ErrNilPin
	}
	d := &Device{
		pin:  [numButtons]Pin{blue, red},
		cfg:  defaultConfig(),
		boot: time.Now(),
	}
	d.clk = bootClock{d.boot}
	return d, nil
}

// bootClock is the default Clock: monotonic time since New.
type bootClock struct{ start time.Time }

func (c bootClock) Now() time.Duration { return time.Since(c.start) }

// SetActiveLow selects the polarity. The unit is active low (default true):
// pressed reads 0. Use false for a button wired to VCC with a pull-down.
func (d *Device) SetActiveLow(activeLow bool) {
	d.mu.Lock()
	d.cfg.activeLow = activeLow
	d.mu.Unlock()
}

// SetDebounce sets how long a level must stay stable to be accepted.
// Values below MinDebounce are clamped.
func (d *Device) SetDebounce(v time.Duration) {
	if v < MinDebounce {
		v = MinDebounce
	}
	d.mu.Lock()
	d.cfg.debounce = v
	d.mu.Unlock()
}

// SetClickWindow sets how long the driver waits for another click before
// reporting Click, DoubleClick or MultiClick. Values below MinDebounce are
// clamped.
func (d *Device) SetClickWindow(v time.Duration) {
	if v < MinDebounce {
		v = MinDebounce
	}
	d.mu.Lock()
	d.cfg.clickWindow = v
	d.mu.Unlock()
}

// SetHoldTime sets the press duration that produces HoldStart. Zero disables
// the hold detection entirely (Pressed/Released only).
func (d *Device) SetHoldTime(v time.Duration) {
	if v < 0 {
		v = 0
	}
	d.mu.Lock()
	d.cfg.holdTime = v
	d.mu.Unlock()
}

// SetHoldRepeat sets the interval of HoldTick events while a button stays
// pressed after HoldStart. Zero (default) disables repetition.
func (d *Device) SetHoldRepeat(v time.Duration) {
	if v < 0 {
		v = 0
	}
	d.mu.Lock()
	d.cfg.holdRepeat = v
	d.mu.Unlock()
}

// SetMaxClicks sets how many clicks close the click window immediately.
// It is clamped to 1..MaxClicks. 1 fires Click at once (no double click),
// 2 (default) fires DoubleClick at once, 3 or 4 wait for the window and then
// report MultiClick for 3+ clicks.
func (d *Device) SetMaxClicks(n uint8) {
	if n < 1 {
		n = 1
	}
	if n > MaxClicks {
		n = MaxClicks
	}
	d.mu.Lock()
	d.cfg.maxClicks = n
	d.mu.Unlock()
}

// SetClock replaces the time source. A nil clock is ignored.
func (d *Device) SetClock(c Clock) {
	if c == nil {
		return
	}
	d.mu.Lock()
	d.clk = c
	d.mu.Unlock()
}

// Update polls both pins once, using a single clock read, and returns the
// event flags produced since the previous call.
//
// Call it at 10-20 ms intervals for accurate DoubleClick and HoldTick
// timing. It never blocks and never allocates.
func (d *Device) Update() (blue, red Events) {
	d.mu.Lock()
	defer d.mu.Unlock()
	now := int64(d.clk.Now())
	return d.step(Blue, now), d.step(Red, now)
}

// UpdateContext is Update with a cancellation check between the two buttons.
// The pin reads are non-blocking, so the context only matters as an early
// exit; a canceled context returns ctx.Err().
func (d *Device) UpdateContext(ctx context.Context) (blue, red Events, err error) {
	if err := ctx.Err(); err != nil {
		return 0, 0, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	now := int64(d.clk.Now())
	blue = d.step(Blue, now)
	if err := ctx.Err(); err != nil {
		return blue, 0, err
	}
	red = d.step(Red, now)
	return blue, red, nil
}

func (d *Device) step(b Button, now int64) Events {
	s := &d.b[b]
	pressed := d.pin[b].Get()
	if d.cfg.activeLow {
		pressed = !pressed
	}
	return s.update(pressed, now, &d.cfg)
}

// Read returns the instantaneous level of b, already normalised for
// SetActiveLow: true means pressed. It is not debounced and performs one pin
// read.
func (d *Device) Read(b Button) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	level := d.pin[b].Get()
	return level != d.cfg.activeLow
}

// Down reports the debounced state: true while the button is held.
func (d *Device) Down(b Button) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.b[b].stable
}

// Held reports how long the button has been held in the debounced state, or 0
// when released.
func (d *Device) Held(b Button) time.Duration {
	d.mu.Lock()
	defer d.mu.Unlock()
	s := &d.b[b]
	if !s.stable {
		return 0
	}
	return time.Duration(int64(d.clk.Now()) - s.downSince)
}

// Reset clears the state machine and pending clicks and re-baselines the
// levels on the next Update, so no phantom event is emitted afterwards.
func (d *Device) Reset() {
	d.mu.Lock()
	d.b = [numButtons]btnState{}
	d.mu.Unlock()
}
