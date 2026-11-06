package dualbutton

import (
	"context"
	"math/rand"
	"sync"
	"testing"
	"time"
)

// fakePin is a scripted input pin.
type fakePin struct {
	level bool // electrical level: true = high (released with active-low)
	reads int
}

func (p *fakePin) Get() bool {
	p.reads++
	return p.level
}

// fakeClock is a manual monotonic clock.
type fakeClock struct {
	ns    int64
	reads int
}

func (c *fakeClock) Now() time.Duration {
	c.reads++
	return time.Duration(c.ns)
}

type harness struct {
	t    *testing.T
	d    *Device
	blue *fakePin
	red  *fakePin
	clk  *fakeClock
}

// newHarness starts with both buttons released (pins high, active low).
func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{
		t:    t,
		blue: &fakePin{level: true},
		red:  &fakePin{level: true},
		clk:  &fakeClock{},
	}
	d, err := New(h.blue, h.red)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	d.SetClock(h.clk)
	h.d = d
	return h
}

// step advances the clock by ms and runs Update.
func (h *harness) step(ms int64) (Events, Events) {
	h.clk.ns += ms * int64(time.Millisecond)
	return h.d.Update()
}

// stepB advances the clock and asserts the red button stayed quiet.
func (h *harness) stepB(ms int64) Events {
	h.t.Helper()
	blue, red := h.step(ms)
	if red != 0 {
		h.t.Fatalf("red produced %s", red)
	}
	return blue
}

// edge registers an electrical change immediately, the way a real poll loop
// would see it, without advancing time. It also asserts the change produced no
// event by itself.
func (h *harness) edge() {
	h.t.Helper()
	blue, red := h.d.Update()
	if blue != 0 || red != 0 {
		h.t.Fatalf("edge produced %s/%s", blue, red)
	}
}

func (h *harness) pressB() {
	h.blue.level = false
	h.edge()
}

func (h *harness) releaseB() {
	h.blue.level = true
	h.edge()
}

func (h *harness) pressR() {
	h.red.level = false
	h.edge()
}

func (h *harness) releaseR() {
	h.red.level = true
	h.edge()
}

func (h *harness) expect(at string, got, want Events) {
	h.t.Helper()
	if got != want {
		h.t.Fatalf("%s: got [%s], want [%s]", at, got, want)
	}
}

func TestNewNilPin(t *testing.T) {
	if _, err := New(nil, &fakePin{}); err != ErrNilPin {
		t.Fatalf("blue nil: got %v, want ErrNilPin", err)
	}
	if _, err := New(&fakePin{}, nil); err != ErrNilPin {
		t.Fatalf("red nil: got %v, want ErrNilPin", err)
	}
}

func TestBaselineIsSilent(t *testing.T) {
	h := newHarness(t)
	blue, red := h.step(1)
	h.expect("baseline", blue|red, 0)
	if h.d.Down(Blue) || h.d.Down(Red) {
		t.Fatal("buttons must start released")
	}
	if got := h.d.Held(Blue); got != 0 {
		t.Fatalf("Held: got %v, want 0", got)
	}
}

func TestPressReleaseClick(t *testing.T) {
	h := newHarness(t)
	h.step(1)

	h.pressB()
	h.expect("bounce", h.stepB(20), 0)
	h.expect("pressed", h.stepB(30), Pressed)
	if !h.d.Down(Blue) {
		t.Fatal("Down must be true after Pressed")
	}

	h.releaseB()
	h.expect("release bounce", h.stepB(20), 0)
	h.expect("released", h.stepB(30), Released)
	if h.d.Down(Blue) {
		t.Fatal("Down must be false after Released")
	}

	// The window opens on the physical release edge, so 400 ms from it are
	// 350 ms after the debounced Released.
	h.expect("window open", h.stepB(300), 0)
	h.expect("click", h.stepB(50), Click)
}

func TestGlitchProducesNothing(t *testing.T) {
	h := newHarness(t)
	h.step(1)

	h.pressB()
	h.step(10)
	h.releaseB()
	for i := 0; i < 20; i++ {
		if ev := h.stepB(25); ev != 0 {
			t.Fatalf("glitch produced %s", ev)
		}
	}
}

func TestBounceStorm(t *testing.T) {
	h := newHarness(t)
	h.step(1)

	// 40 ms of 1 ms toggling, ending pressed.
	presses := 0
	for i := 0; i < 40; i++ {
		h.blue.level = i%2 != 0
		blue := h.stepB(1)
		if blue.Has(Pressed) {
			presses++
		}
		if blue != 0 && blue != Pressed {
			t.Fatalf("unexpected %s while bouncing", blue)
		}
	}
	h.blue.level = false
	h.step(10)
	if blue := h.stepB(50); blue != Pressed {
		t.Fatalf("after bounce: got %s, want Pressed", blue)
	}
	if presses != 0 {
		t.Fatalf("Pressed emitted %d times during bounce", presses)
	}
}

func TestMaxClicksOneFiresImmediately(t *testing.T) {
	h := newHarness(t)
	h.step(1)
	h.d.SetMaxClicks(1)

	h.pressB()
	h.expect("pressed", h.stepB(50), Pressed)
	h.releaseB()
	h.expect("click", h.stepB(50), Released|Click)
	h.expect("nothing pending", h.stepB(500), 0)
}

func TestDoubleClick(t *testing.T) {
	h := newHarness(t)
	h.step(1)

	h.pressB()
	h.expect("press 1", h.stepB(50), Pressed)
	h.releaseB()
	h.expect("release 1", h.stepB(50), Released)

	h.pressB()
	h.expect("press 2", h.stepB(50), Pressed)
	h.releaseB()
	h.expect("double click", h.stepB(50), Released|DoubleClick)
	h.expect("no trailing click", h.stepB(600), 0)
}

func TestSlowClicksAreSingles(t *testing.T) {
	h := newHarness(t)
	h.step(1)

	for i := 0; i < 2; i++ {
		h.pressB()
		h.expect("pressed", h.stepB(50), Pressed)
		h.releaseB()
		h.expect("released", h.stepB(50), Released)
		h.expect("click", h.stepB(450), Click)
	}
}

func TestMultiClick(t *testing.T) {
	h := newHarness(t)
	h.step(1)
	h.d.SetMaxClicks(4)

	for i := 0; i < 3; i++ {
		h.pressB()
		h.expect("pressed", h.stepB(50), Pressed)
		h.releaseB()
		want := Released
		if i == 2 {
			want = Released // still waiting for the window
		}
		h.expect("released", h.stepB(50), want)
	}
	h.expect("multiclick", h.stepB(400), MultiClick)
}

func TestHoldAndRelease(t *testing.T) {
	h := newHarness(t)
	h.step(1)

	h.pressB()
	h.expect("pressed", h.stepB(50), Pressed)
	h.expect("before hold", h.stepB(700), 0)
	h.expect("hold start", h.stepB(50), HoldStart)
	if got := h.d.Held(Blue); got != 800*time.Millisecond {
		t.Fatalf("Held: got %v, want 800ms", got)
	}

	h.releaseB()
	h.expect("hold end", h.stepB(50), Released|HoldEnd)
	if got := h.d.Held(Blue); got != 0 {
		t.Fatalf("Held after release: got %v", got)
	}
	h.expect("no click after hold", h.stepB(600), 0)
}

func TestHoldRepeat(t *testing.T) {
	h := newHarness(t)
	h.step(1)
	h.d.SetHoldRepeat(200 * time.Millisecond)

	h.pressB()
	h.expect("pressed", h.stepB(50), Pressed)
	h.expect("hold start", h.stepB(750), HoldStart)
	h.expect("tick 1", h.stepB(200), HoldTick)
	h.expect("tick 2", h.stepB(200), HoldTick)
	h.expect("tick idle", h.stepB(100), 0)
	h.releaseB()
	h.expect("end", h.stepB(50), Released|HoldEnd)
}

func TestHoldDisabled(t *testing.T) {
	h := newHarness(t)
	h.step(1)
	h.d.SetHoldTime(0)

	h.pressB()
	h.expect("pressed", h.stepB(50), Pressed)
	h.expect("no hold", h.stepB(3000), 0)
	h.releaseB()
	h.expect("released", h.stepB(50), Released)
	h.expect("click", h.stepB(400), Click)
}

func TestHoldCancelsPendingClick(t *testing.T) {
	h := newHarness(t)
	h.step(1)
	h.d.SetHoldRepeat(100 * time.Millisecond)

	// First a quick click, then a hold before the window closes.
	h.pressB()
	h.expect("pressed", h.stepB(50), Pressed)
	h.releaseB()
	h.expect("released", h.stepB(50), Released)

	h.pressB()
	h.expect("pressed again", h.stepB(50), Pressed)
	h.expect("hold start", h.stepB(750), HoldStart)

	h.releaseB()
	h.expect("hold end", h.stepB(50), Released|HoldEnd)
	h.expect("pending click dropped", h.stepB(1000), 0)
}

func TestBootPressedIsNotAPhantomPress(t *testing.T) {
	h := newHarness(t)
	h.pressB() // already held before the first Update

	blue, red := h.step(10)
	h.expect("baseline", blue|red, 0)
	if !h.d.Down(Blue) {
		t.Fatal("Down must be true for a button held at boot")
	}

	h.expect("hold from boot", h.stepB(790), HoldStart)
	h.releaseB()
	h.expect("released", h.stepB(50), Released|HoldEnd)
}

func TestBootPressCountsOneClick(t *testing.T) {
	h := newHarness(t)
	h.pressB()
	h.step(10) // baseline

	h.releaseB()
	h.expect("released without Pressed", h.stepB(50), Released)
	h.expect("click", h.stepB(400), Click)
}

func TestActiveHigh(t *testing.T) {
	h := newHarness(t)
	h.d.SetActiveLow(false)
	h.blue.level = false
	h.red.level = false
	h.step(1)

	h.blue.level = true // pressed when active high
	h.edge()
	h.expect("pressed", h.stepB(50), Pressed)
	if !h.d.Read(Blue) {
		t.Fatal("Read must report pressed")
	}
	h.blue.level = false
	h.edge()
	h.expect("released", h.stepB(50), Released)
}

func TestIndependentButtons(t *testing.T) {
	h := newHarness(t)
	h.step(1)

	h.pressB()
	blue, red := h.step(50)
	h.expect("blue", blue, Pressed)
	h.expect("red", red, 0)

	h.pressR()
	blue, red = h.step(50)
	h.expect("red", red, Pressed)
	h.expect("blue idle", blue, 0)

	if !h.d.Down(Blue) || !h.d.Down(Red) {
		t.Fatal("both buttons must be down")
	}
}

func TestReadsPerUpdate(t *testing.T) {
	h := newHarness(t)
	h.step(1) // baseline consumes a read pair
	h.blue.reads, h.red.reads, h.clk.reads = 0, 0, 0

	h.step(10)
	if h.blue.reads != 1 || h.red.reads != 1 {
		t.Fatalf("pin reads: blue=%d red=%d, want 1 each", h.blue.reads, h.red.reads)
	}
	if h.clk.reads != 1 {
		t.Fatalf("clock reads: %d, want 1", h.clk.reads)
	}
}

func TestReset(t *testing.T) {
	h := newHarness(t)
	h.step(1)

	h.pressB()
	h.expect("pressed", h.stepB(50), Pressed)
	h.d.Reset()

	h.expect("after reset", h.stepB(500), 0)
	if !h.d.Down(Blue) {
		t.Fatal("reset must re-baseline the current level")
	}
	h.releaseB()
	h.expect("released", h.stepB(50), Released)
}

func TestSettersClamp(t *testing.T) {
	h := newHarness(t)
	h.step(1)

	h.d.SetDebounce(0)
	h.d.SetMaxClicks(0) // clamped to 1
	h.pressB()
	h.expect("1ms debounce", h.stepB(1), Pressed)
	h.releaseB()
	h.expect("immediate click", h.stepB(1), Released|Click)

	h.d.SetClickWindow(-5) // clamped to MinDebounce
	h.d.SetHoldTime(-5)    // clamped to 0
	h.d.SetHoldRepeat(-5)  // clamped to 0
	h.pressB()
	h.expect("pressed", h.stepB(5), Pressed)
	h.expect("no hold", h.stepB(2000), 0)
	h.d.SetClock(nil) // ignored
	h.expect("clock still works", h.stepB(5), 0)
}

func TestUpdateContext(t *testing.T) {
	h := newHarness(t)
	ctx, cancel := context.WithCancel(context.Background())

	blue, red, err := h.d.UpdateContext(ctx)
	if err != nil || blue|red != 0 {
		t.Fatalf("live ctx: events=%s/%s err=%v", blue, red, err)
	}

	cancel()
	if _, _, err := h.d.UpdateContext(ctx); err != context.Canceled {
		t.Fatalf("canceled ctx: got %v, want context.Canceled", err)
	}
}

func TestEventsHelpers(t *testing.T) {
	var zero Events
	if zero.Any() || zero.String() != "none" {
		t.Fatalf("zero: Any=%v String=%q", zero.Any(), zero.String())
	}
	both := Pressed | Released
	if !both.Has(Pressed) || !both.Has(Released) || !both.Any() {
		t.Fatal("Has/Any")
	}
	if got := (DoubleClick | HoldEnd).String(); got != "DoubleClick|HoldEnd" {
		t.Fatalf("String: %q", got)
	}
	if Blue.String() != "Blue" || Red.String() != "Red" || Button(9).String() != "Button(9)" {
		t.Fatal("Button.String")
	}
}

func TestConcurrentAccess(t *testing.T) {
	h := newHarness(t)
	var wg sync.WaitGroup
	stop := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				h.d.Update()
			}
		}
	}()
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_ = h.d.Down(Blue)
				_ = h.d.Down(Red)
				_ = h.d.Held(Blue)
				_ = h.d.Read(Red)
			}
		}()
	}
	close(stop)
	wg.Wait()
}

// TestRandomSequences drives random level changes and checks the invariants of
// the state machine (event exclusivity and Pressed/Released/Down coherence).
func TestRandomSequences(t *testing.T) {
	for seed := int64(0); seed < 40; seed++ {
		rng := rand.New(rand.NewSource(seed))
		h := newHarness(t)
		h.d.SetDebounce(time.Duration(1+rng.Intn(20)) * time.Millisecond)
		h.d.SetClickWindow(time.Duration(10+rng.Intn(100)) * time.Millisecond)
		h.d.SetHoldTime(time.Duration(rng.Intn(200)) * time.Millisecond)
		if rng.Intn(2) == 0 {
			h.d.SetHoldRepeat(time.Duration(5+rng.Intn(50)) * time.Millisecond)
		}
		h.d.SetMaxClicks(uint8(1 + rng.Intn(int(MaxClicks))))

		down := [numButtons]bool{}
		h.step(1)
		down[Blue], down[Red] = h.d.Down(Blue), h.d.Down(Red)

		for i := 0; i < 3000; i++ {
			if rng.Intn(10) == 0 {
				h.blue.level = rng.Intn(2) == 0
			}
			if rng.Intn(10) == 0 {
				h.red.level = rng.Intn(2) == 0
			}
			blue, red := h.step(int64(1 + rng.Intn(12)))
			for _, c := range [numButtons]struct {
				b  Button
				ev Events
			}{{Blue, blue}, {Red, red}} {
				ev := c.ev
				if n := countClickEvents(ev); n > 1 {
					t.Fatalf("seed %d step %d: %s has %d click events", seed, i, ev, n)
				}
				if ev.Has(Pressed) && ev.Has(Released) {
					t.Fatalf("seed %d step %d: %s has Pressed and Released", seed, i, ev)
				}
				if ev.Has(Pressed) {
					if down[c.b] {
						t.Fatalf("seed %d step %d: %s double Pressed", seed, i, c.b)
					}
					down[c.b] = true
				}
				if ev.Has(Released) {
					if !down[c.b] {
						t.Fatalf("seed %d step %d: %s Released while up", seed, i, c.b)
					}
					down[c.b] = false
				}
				if got := h.d.Down(c.b); got != down[c.b] {
					t.Fatalf("seed %d step %d: %s Down=%v, want %v", seed, i, c.b, got, down[c.b])
				}
				if ev.Has(HoldStart) && !down[c.b] {
					t.Fatalf("seed %d step %d: %s HoldStart while up", seed, i, c.b)
				}
				if ev.Has(HoldTick) && !down[c.b] {
					t.Fatalf("seed %d step %d: %s HoldTick while up", seed, i, c.b)
				}
				if ev.Has(HoldEnd) && down[c.b] {
					t.Fatalf("seed %d step %d: %s HoldEnd while down", seed, i, c.b)
				}
			}
		}
	}
}

func countClickEvents(ev Events) int {
	n := 0
	for _, f := range [...]Events{Click, DoubleClick, MultiClick} {
		if ev.Has(f) {
			n++
		}
	}
	return n
}
