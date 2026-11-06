package dualbutton

// btnState is the per-button state machine. It is kept tiny (40 bytes on
// 64-bit hosts) so two of them fit in the Device without allocation.
//
// Two independent clocks are implicit: rawSince marks when the electrical
// level last changed (used for debounce), and downSince marks when the press
// became stable (used for hold timing).
type btnState struct {
	rawSince  int64 // ns: last electrical edge
	downSince int64 // ns: start of the stable press
	windowAt  int64 // ns: release that closed the last click
	ticked    int64 // ns: last HoldTick
	clicks    uint8 // clicks inside the current window
	raw       bool  // last level, normalised (true = pressed)
	stable    bool  // debounced level
	holding   bool  // HoldStart already emitted for this press
	seeded    bool  // baseline taken on the first Update
}

// update advances the state machine for one button and returns the events
// produced by this call.
func (s *btnState) update(level bool, now int64, cfg *config) Events {
	if level != s.raw {
		s.raw = level
		s.rawSince = now
	}
	since := now - s.rawSince

	if !s.seeded {
		// Baseline: a button already pressed at boot is treated as pressed
		// without emitting Pressed (no phantom event in the first frame).
		s.seeded = true
		s.stable = level
		s.downSince = now
		s.windowAt = now
		return 0
	}

	if s.stable {
		if level {
			return s.held(now, cfg)
		}
		if since < int64(cfg.debounce) {
			return 0 // still bouncing
		}
		return s.release(now, cfg)
	}

	if !level {
		return s.closeWindow(now, cfg)
	}
	if since < int64(cfg.debounce) {
		return 0 // still bouncing
	}
	s.stable = true
	s.downSince = s.rawSince // hold time starts at the physical edge
	s.holding = false
	return Pressed
}

// held handles a button that is still pressed: HoldStart and HoldTick.
func (s *btnState) held(now int64, cfg *config) Events {
	var ev Events
	if !s.holding && cfg.holdTime > 0 && now-s.downSince >= int64(cfg.holdTime) {
		s.holding = true
		s.ticked = now
		s.clicks = 0 // a hold cancels a pending click
		ev |= HoldStart
	}
	if s.holding && cfg.holdRepeat > 0 && now-s.ticked >= int64(cfg.holdRepeat) {
		s.ticked = now
		ev |= HoldTick
	}
	return ev
}

// release handles the debounced release: Released plus either HoldEnd or one
// more click in the current window.
func (s *btnState) release(now int64, cfg *config) Events {
	s.stable = false
	ev := Released
	if s.holding {
		s.holding = false
		s.clicks = 0
		return ev | HoldEnd
	}
	s.clicks++
	s.windowAt = s.rawSince // window opens at the physical release
	if s.clicks >= cfg.maxClicks {
		ev |= clickEvent(s.clicks)
		s.clicks = 0
	}
	return ev
}

// closeWindow emits a pending click once the click window has elapsed.
func (s *btnState) closeWindow(now int64, cfg *config) Events {
	if s.clicks == 0 || now-s.windowAt < int64(cfg.clickWindow) {
		return 0
	}
	ev := clickEvent(s.clicks)
	s.clicks = 0
	return ev
}

func clickEvent(clicks uint8) Events {
	switch {
	case clicks == 1:
		return Click
	case clicks == 2:
		return DoubleClick
	default:
		return MultiClick
	}
}
