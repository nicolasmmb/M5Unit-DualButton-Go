package dualbutton

import (
	"context"
	"testing"
	"time"
	"unsafe"
)

// TestNoAllocUpdate is the allocation contract: polling both buttons must not
// touch the heap.
func TestNoAllocUpdate(t *testing.T) {
	h := newHarness(t)
	h.pressB()
	h.step(1)
	h.step(50)

	allocs := testing.AllocsPerRun(200, func() {
		h.d.Update()
	})
	if allocs != 0 {
		t.Fatalf("Update allocates %v per call", allocs)
	}
}

// TestNoAllocUpdateContext covers the context wrapper as well.
func TestNoAllocUpdateContext(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	allocs := testing.AllocsPerRun(200, func() {
		if _, _, err := h.d.UpdateContext(ctx); err != nil {
			t.Fatalf("UpdateContext: %v", err)
		}
	})
	if allocs != 0 {
		t.Fatalf("UpdateContext allocates %v per call", allocs)
	}
}

// TestNoAllocAccessors covers Read/Down/Held.
func TestNoAllocAccessors(t *testing.T) {
	h := newHarness(t)
	h.step(1)
	h.pressB()
	h.step(50)

	allocs := testing.AllocsPerRun(200, func() {
		_ = h.d.Read(Blue)
		_ = h.d.Down(Blue)
		_ = h.d.Held(Red)
	})
	if allocs != 0 {
		t.Fatalf("accessors allocate %v per call", allocs)
	}
}

// TestFootprint keeps the RAM cost small enough for a firmware that also runs
// BLE and a display.
func TestFootprint(t *testing.T) {
	device := unsafe.Sizeof(Device{})
	state := unsafe.Sizeof(btnState{})
	cfg := unsafe.Sizeof(config{})
	t.Logf("Device=%d B btnState=%d B config=%d B", device, state, cfg)

	if device > 256 {
		t.Fatalf("Device is %d B, budget is 256 B", device)
	}
	if state > 48 {
		t.Fatalf("btnState is %d B, budget is 48 B", state)
	}
	if cfg > 64 {
		t.Fatalf("config is %d B, budget is 64 B", cfg)
	}
}

// TestNewDoesNotPollHardware: construction must not read the pins, so the
// baseline really happens on the first Update.
func TestNewDoesNotPollHardware(t *testing.T) {
	blue, red := &fakePin{level: true}, &fakePin{level: true}
	if _, err := New(blue, red); err != nil {
		t.Fatal(err)
	}
	if blue.reads != 0 || red.reads != 0 {
		t.Fatalf("New read the pins: blue=%d red=%d", blue.reads, red.reads)
	}
}

// TestDefaultTimings documents the defaults shared with UIFlow2.
func TestDefaultTimings(t *testing.T) {
	h := newHarness(t)
	if h.d.cfg.debounce != 50*time.Millisecond ||
		h.d.cfg.clickWindow != 400*time.Millisecond ||
		h.d.cfg.holdTime != 800*time.Millisecond ||
		h.d.cfg.holdRepeat != 0 ||
		h.d.cfg.maxClicks != 2 ||
		!h.d.cfg.activeLow {
		t.Fatalf("unexpected defaults: %+v", h.d.cfg)
	}
}
