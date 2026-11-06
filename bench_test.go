package dualbutton

import (
	"testing"
	"time"
)

// benchDevice builds a Device wired to fake hardware.
func benchDevice() (*Device, *fakePin, *fakePin, *fakeClock) {
	blue, red := &fakePin{level: true}, &fakePin{level: true}
	clk := &fakeClock{}
	d, err := New(blue, red)
	if err != nil {
		panic(err)
	}
	d.SetClock(clk)
	return d, blue, red, clk
}

func reportReads(b *testing.B, blue, red *fakePin, clk *fakeClock, n int) {
	b.Helper()
	if n == 0 {
		return
	}
	b.ReportMetric(float64(blue.reads)/float64(n), "pinblue/op")
	b.ReportMetric(float64(red.reads)/float64(n), "pinred/op")
	b.ReportMetric(float64(clk.reads)/float64(n), "clock/op")
}

// BenchmarkUpdateIdle measures the base cost: two pin reads and one clock read
// with nothing happening.
func BenchmarkUpdateIdle(b *testing.B) {
	d, blue, red, clk := benchDevice()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Update()
	}
	b.StopTimer()
	reportReads(b, blue, red, clk, b.N)
}

// BenchmarkUpdatePoll measures a realistic 100 Hz polling loop.
func BenchmarkUpdatePoll(b *testing.B) {
	d, blue, red, clk := benchDevice()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		clk.ns += int64(10 * time.Millisecond)
		d.Update()
	}
	b.StopTimer()
	reportReads(b, blue, red, clk, b.N)
}

// BenchmarkUpdateBounce is the worst case for the state machine: the level
// changes on every poll.
func BenchmarkUpdateBounce(b *testing.B) {
	d, blue, red, clk := benchDevice()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		clk.ns += int64(2 * time.Millisecond)
		blue.level = i%2 == 0
		red.level = i%2 != 0
		d.Update()
	}
	b.StopTimer()
	reportReads(b, blue, red, clk, b.N)
}

// BenchmarkUpdateClick runs whole click cycles (1 press, 1 release, window
// close) at a 10 ms poll interval.
func BenchmarkUpdateClick(b *testing.B) {
	d, blue, red, clk := benchDevice()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		clk.ns += int64(10 * time.Millisecond)
		switch i % 80 {
		case 0:
			blue.level = false
		case 6:
			blue.level = true
		}
		d.Update()
	}
	b.StopTimer()
	reportReads(b, blue, red, clk, b.N)
}

// BenchmarkUpdateHoldTick keeps one button held with a 10 ms repeat, the
// heaviest path of the state machine.
func BenchmarkUpdateHoldTick(b *testing.B) {
	d, blue, red, clk := benchDevice()
	d.SetHoldTime(time.Millisecond)
	d.SetHoldRepeat(10 * time.Millisecond)
	blue.level = false
	d.Update() // baseline: held

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		clk.ns += int64(10 * time.Millisecond)
		d.Update()
	}
	b.StopTimer()
	reportReads(b, blue, red, clk, b.N)
}

// BenchmarkUpdateBoth moves both buttons on every poll.
func BenchmarkUpdateBoth(b *testing.B) {
	d, blue, red, clk := benchDevice()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		clk.ns += int64(10 * time.Millisecond)
		blue.level = i%40 < 20
		red.level = i%40 >= 20
		d.Update()
	}
	b.StopTimer()
	reportReads(b, blue, red, clk, b.N)
}

// BenchmarkUpdateRealClock measures the cost of the default time.Now-backed
// clock, so the TinyGo overhead of the monotonic source is visible.
func BenchmarkUpdateRealClock(b *testing.B) {
	d, _, _, _ := benchDevice()
	d.SetClock(realClock{})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Update()
	}
}

type realClock struct{}

func (realClock) Now() time.Duration { return time.Since(time.Unix(0, 0)) }
