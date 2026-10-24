package main

import (
	"testing"
	"time"
)

func TestPresence(t *testing.T) {
	t0 := time.Now()
	fresh := func() *presenceState { return &presenceState{windows: map[string]time.Time{}, lastHeard: t0} }
	at := func(d time.Duration) time.Time { return t0.Add(d) }
	set := func(p *presenceState, id string, d time.Duration) {
		p.lastHeard = at(d)
		if id != "" {
			p.windows[id], p.hadWindow = at(d), true
		}
	}

	// started but no window ever opened: wait out the long limit, then leave
	p := fresh()
	if p.shouldLeave(at(30*time.Second), false) || !p.shouldLeave(at(leaveWhenSilent+time.Second), false) {
		t.Fatal("no window ever: should leave only after the long limit")
	}
	// one window, closed: leave a few seconds later
	p = fresh()
	set(p, "a", 0)
	if p.shouldLeave(at(50*time.Second), false) {
		t.Fatal("an open window that is merely quiet must keep the program running")
	}
	p.windows = map[string]time.Time{} // it said bye at 60 s
	p.lastHeard = at(60 * time.Second)
	if p.shouldLeave(at(61*time.Second), false) {
		t.Fatal("left too early: a reloading page needs a moment to come back")
	}
	if !p.shouldLeave(at(60*time.Second+leaveAfterLastWindow+time.Second), false) {
		t.Fatal("should leave shortly after the last window closed")
	}
	// reload: bye, then the new page arrives within the grace time
	p = fresh()
	set(p, "a", 0)
	delete(p.windows, "a")
	set(p, "b", 500*time.Millisecond)
	if p.shouldLeave(at(10*time.Second), false) {
		t.Fatal("a reloaded page must keep the program running")
	}
	// two windows, one closes: stay
	p = fresh()
	set(p, "a", 0)
	set(p, "b", 0)
	delete(p.windows, "a")
	if p.shouldLeave(at(20*time.Second), false) {
		t.Fatal("the second window is still open")
	}
	// a window that vanished without saying bye is forgotten after the long limit
	if !p.shouldLeave(at(leaveWhenSilent+2*time.Second), false) {
		t.Fatal("a window that vanished silently should not keep the program alive for ever")
	}
	// the PC slept: nobody is dropped, everyone gets a fresh start
	p = fresh()
	set(p, "a", 0)
	if p.shouldLeave(at(3*time.Hour), true) {
		t.Fatal("waking from sleep must not end the program")
	}
	if p.shouldLeave(at(3*time.Hour+30*time.Second), false) {
		t.Fatal("after waking, the window gets time to be heard from again")
	}
	// requests without a window name (pictures) count as a sign of life but not as a window
	p = fresh()
	p.heard("")
	if p.hadWindow || len(p.windows) != 0 {
		t.Fatal("a nameless request is not a window")
	}
	p.heard("w1")
	p.bye("w1")
	if !p.hadWindow || len(p.windows) != 0 {
		t.Fatal("bye should remove the window")
	}
}
