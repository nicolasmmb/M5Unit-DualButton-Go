# M5Unit-DualButton-Go

[![test](https://github.com/nicolasmmb/M5Unit-DualButton-Go/actions/workflows/test.yml/badge.svg)](https://github.com/nicolasmmb/M5Unit-DualButton-Go/actions/workflows/test.yml)

Pure-Go driver for the **M5Stack Unit Dual Button** (SKU U025) for TinyGo, and
a general-purpose two-button event state machine. No C, C++, CGo or Arduino.

The unit has **no controller chip**: it is two tactile switches wired to a Grove
PORT B connector, each with an on-board 10K/20K divider. This package therefore
depends on nothing but two GPIO inputs — `machine.Pin` satisfies the `Pin`
interface directly, and a fake pin makes the whole state machine testable on
the host.

## Features

- Debounce, click, double click, multi click, hold start/tick/end — one
  bitmask of events per button per `Update`.
- **Zero heap allocations** per `Update` (two pin reads + one clock read).
- **Injectable clock**: deterministic jump-free tests of every transition,
  no `sleep`.
- Timings configurable (debounce, click window, hold, hold repeat, max clicks)
  with the same defaults as the M5Stack UIFlow2 `hardware.Button` driver.
- Active-low by default (the unit), active-high supported.
- Works on any board: it is just GPIO. `ConfigureInput` helper for ESP32.
- Host tests with a scripted pin and a manual clock, plus randomized
  invariant tests, allocation and footprint contracts.
- `Device` is **200 bytes**, `btnState` 40 bytes, `config` 40 bytes.

## Hardware

| Parameter | Value |
|---|---|
| Unit | M5Stack Unit Dual Button (U025) |
| Interface | **GPIO**, no chip, no protocol |
| Connector | Grove HY2.0-4P on **PORT B** |
| Buttons | Red (yellow wire), Blue (white wire) |
| Idle / pressed | ≈ **3.3 V** / ≈ 0 V (active low) |
| Internal divider | 10K to 5 V + 20K to GND + 100 nF, per button |
| Pull-up needed | **No** — the unit has its own divider |
| Wiring (M5StickC Plus2) | Red = **G26**, Blue = **G36** |

Notes:

- **GPIO36 is input-only** and has no internal pull resistor; the unit divider
  is what makes it work. Never enable a pull on it.
- The 10K/20K divider brings the 5 V Grove rail down to ≈3.3 V, so the pin sees
  a safe high level.
- For a bare button with no divider, configure the pin with
  `machine.PinInputPullup` (or `PinInputPulldown` plus `SetActiveLow(false)`).

```
5V ──[10K]──┬── signal (Grove) ──┬── switch ── GND
            │                    │
         [20K]                  [100n]
            │
           GND
```

## Install

```bash
go get github.com/nicolasmmb/M5Unit-DualButton-Go@v0.1.0
```

## Quick start

```go
dualbutton.ConfigureInput(dualbutton.StickCPlus2Blue) // GPIO36
dualbutton.ConfigureInput(dualbutton.StickCPlus2Red)  // GPIO26

d, err := dualbutton.New(dualbutton.StickCPlus2Blue, dualbutton.StickCPlus2Red)
if err != nil {
    println("dualbutton:", err.Error())
    return
}

for {
    blue, red := d.Update()
    switch {
    case blue.Has(dualbutton.Click):
        println("blue click")
    case blue.Has(dualbutton.DoubleClick):
        println("blue double click")
    case blue.Has(dualbutton.HoldStart):
        println("blue hold")
    }
    if red.Has(dualbutton.Click) {
        println("red click")
    }
    time.Sleep(10 * time.Millisecond) // poll at 50-100 Hz
}
```

Poll between 5 and 20 ms. `Update` never blocks and never allocates; `Down`,
`Held` and `Read` are cheap accessors for UI rendering.

## API

| Function | Description |
|---|---|
| `New(blue, red Pin) (*Device, error)` | Two pins already configured as inputs; `ErrNilPin` |
| `Update() (blue, red Events)` | Poll both pins once, single clock read |
| `UpdateContext(ctx) (Events, Events, error)` | Same, with a cancellation check |
| `Read(b) bool` | Instantaneous level, normalised (true = pressed) |
| `Down(b) bool` | Debounced state |
| `Held(b) time.Duration` | Time held, 0 when released |
| `Reset()` | Clear state and re-baseline on the next `Update` |
| `SetActiveLow(bool)` | Polarity, default `true` |
| `SetDebounce(d)` | Default 50 ms, clamped to ≥ 1 ms |
| `SetClickWindow(d)` | Default 400 ms |
| `SetHoldTime(d)` | Default 800 ms, 0 disables hold |
| `SetHoldRepeat(d)` | Hold tick interval, 0 (default) disables |
| `SetMaxClicks(n)` | 1–4, default 2 |
| `SetClock(Clock)` | Replace `time.Now`-backed clock |
| `ConfigureInput(p machine.Pin)` | Input without pull (ESP32/C3/S3/RP2040) |
| `Layout` + `NewLayout(l)` | Named connector with the two pins in logical order |
| `Layout.Configure()` | Input mode for both pins (ESP32/C3/S3/RP2040) |
| `Detect(cands ...Layout) (Layout, bool)` | Finds the connector that has the unit |
| `Choose(cands, high)` | The pure decision behind Detect, host-testable |

Events:

| Flag | When |
|---|---|
| `Pressed` | Press accepted (after the debounce) |
| `Released` | Release accepted (after the debounce) |
| `Click` | Window closed with exactly one click |
| `DoubleClick` | Two clicks inside the window |
| `MultiClick` | Three or more clicks inside the window |
| `HoldStart` | Press lasted `SetHoldTime` |
| `HoldTick` | Every `SetHoldRepeat` while held |
| `HoldEnd` | Hold released (comes with `Released`) |

`SetMaxClicks` controls latency versus ambiguity:

| Value | Behaviour |
|---|---|
| `1` | `Click` fires at the release debounce; double click disabled |
| `2` (default) | single click waits for the window, double click fires immediately |
| `3` / `4` | `MultiClick` after the window |

## Port detection

The unit is a passive divider, so a pull-down probe tells where it is: the
connected pair stays high, an empty connector falls.

```go
layout, ok := dualbutton.Detect(dualbutton.StickCPlus2PortB, dualbutton.StickCPlus2PortA)
if !ok {
    println("unit? check the Grove cable")
    return
}
println("unit on", layout.Name)   // "StickC Plus2 PORT A"

d, err := dualbutton.NewLayout(layout)
```

`Detect` is the same probe the validation firmware used to find the unit on
PORT A when the cable was on the wrong connector.

## Semantics and caveats

- **A press is only reported after the debounce**, so a mechanical glitch
  shorter than `SetDebounce` produces no `Pressed` at all (the UIFlow2 driver
  reports `PRESSED` before debouncing and then has to roll it back).
- **The click window starts at the physical release edge**, not at the
  debounced `Released`, so `Click` arrives ≈ `clickWindow` after the finger
  leaves the button.
- **A hold cancels a pending click**: click → hold reports `HoldStart`/`HoldEnd`
  and no `Click`.
- **A button held during boot emits no `Pressed`** (baseline on the first
  `Update`), but the release still counts as one click, exactly like UIFlow2.
  Call `Reset()` if you need a clean slate.
- `Down` is already true for a button held at boot; hold timing starts then.
- The pin must be an input before the first `Update`; `Update` only reads it.
- One `Update` is one clock read shared by both buttons, so both see the same
  instant.

## Performance

Host measurements against fake pins (see [BENCHMARKS.md](BENCHMARKS.md)):

| Scenario | ns/op | pin reads | clock reads | allocs/op |
|---|---:|---:|---:|---:|
| `Update` idle | ~15 | 2 | 1 | 0 |
| `Update` at 100 Hz | ~15 | 2 | 1 | 0 |
| `Update` bounce storm | ~17 | 2 | 1 | 0 |
| `Update` hold + tick | ~15 | 2 | 1 | 0 |
| `Update` default clock | ~51 | 2 | 1 | 0 |

RAM: `Device` 200 B, `btnState` 40 B, `config` 40 B — two devices still fit in
one BLE connection's worth of buffers.

TinyGo example for `m5stick-c`: 39.5 KB flash, 4.9 KB RAM (framework included).

```bash
make bench    # benchmarks with pin/clock reads per op and allocs
make alloc    # 0-allocation and footprint contracts
make check    # vet + race tests + contracts + TinyGo example build
```

## Parity with the official drivers

| UIFlow2 | This package |
|---|---|
| `isPressed()` | `Pressed` / `Down(b)` |
| `isReleased()` | `Released` |
| `wasClicked()` | `Click` |
| `wasDoubleClicked()` | `DoubleClick` |
| `wasHold()` / `isHolding()` | `HoldStart` / `Down(b)` + `Held(b)` |
| multi-click callback | `MultiClick` (via `SetMaxClicks`) |
| long-press start/stop | `HoldStart` / `HoldEnd` |
| during long press | `HoldTick` (`SetHoldRepeat`) |
| `SimpleButton.value()` | `Read(b)` |
| `setCallback` | poll the returned bits (no callback, no reentrancy) |

## Limitations / roadmap

- No interrupt-driven mode yet (`machine.Pin.SetInterrupt` callback + wake);
  polling at 10–20 ms is enough for UI use and costs ~15 ns per update.
- No callback API: the event bitmask keeps `Update` allocation-free and free of
  reentrancy; a `SetHandler` can be added without breaking the API.
- Debounce is level-based, not a filter with hysteresis; tact switches with
  very long bounce (>50 ms) need a higher `SetDebounce`.
- The driver cannot tell whether the unit is connected — there is no chip to
  identify. With **nothing on PORT B**, GPIO26/GPIO36 float and read **low**,
  so the device reports *both buttons held* from the first `Update`. That is a
  wiring symptom, not a driver bug: check the Grove cable/port (the example
  prints a warning when both buttons read pressed at boot).
- The unit needs the 5 V rail of PORT B: without it the on-board 10K pull-up
  is dead and the 20K pulldown holds both signals low (same symptom).

## Hardware status

Validated on an M5StickC Plus2 with the real unit, driven by
`examples/validate` (guided checklist, one gesture per step, PASS only when the
expected event pattern shows up):

| Check | Result |
|---|---|
| `tinygo build` / `tinygo flash` (`m5stick-c`) | ✅ 45 KB flash / 4.9 KB RAM |
| Port auto-detection (pull-down probe) | ✅ found the unit on **PORT A** (G32/G33) |
| 1 click (blue, red) | ✅ `Pressed` → `Released` → `Click` |
| Double click (blue, red) | ✅ `Released|DoubleClick`, no trailing `Click` |
| Triple click | ✅ `MultiClick` exactly `window` after the release edge |
| Hold 1.5 s / 2 s | ✅ `HoldStart` at ~800 ms, `HoldEnd`, no `Click` |
| Hold repeat 200 ms | ✅ 8 ticks, **0–1 ms jitter** |
| Both buttons together | ✅ 10 ms skew, independent state machines |
| 10 rapid presses | ✅ 5 × `DoubleClick`, 0 stray `Click` |
| **PORT B (G26/G36)** | ⚠️ not validated: the unit was plugged into PORT A on this board |

The driver is pin-agnostic, so the PORT A run validates the whole state
machine; only the `StickCPlus2Red`/`StickCPlus2Blue` constants and the
GPIO36-without-pull-up path remain unexercised on hardware.

`examples/validate` also detects the port itself: it pulls both Grove pairs
down and keeps the pair that stays high (the unit has 10K to 5 V against the
MCU's ~45K pull-down). That is the cheap, deterministic way to answer "is the
unit plugged in, and where" with no I2C and no extra pin.

## References

- [M5Stack Unit Dual Button (U025)](https://docs.m5stack.com/en/unit/dual_button)
- `Sch_UNIT_DualButton_v1.1.pdf` (schematic, 10K/20K divider)
- [UIFlow2 Dual Button Unit](https://uiflow-micropython.readthedocs.io/en/latest/unit/dualbutton.html)
- `uiflow-micropython/m5stack/libs/unit/dual_button.py`
- `uiflow-micropython/m5stack/libs/driver/button.py`
- `M5-ProductExampleCodes/Unit/DUAL_BUTTON/Arduino/dual_button/dual_button.ino`

## License

MIT — see [LICENSE](LICENSE).
