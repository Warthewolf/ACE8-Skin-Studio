package main

// Knowing when to stop: the program has no window of its own, only pages open in a browser.
// Each open page gives itself a name and says "bye" when it closes, so the program can leave
// a few seconds after its last page is closed instead of lingering in the background.

import (
	"os"
	"sync"
	"time"
)

const (
	leaveAfterLastWindow = 3 * time.Second   // long enough for a reloading page to come back
	leaveWhenSilent      = 100 * time.Second // no page said goodbye, but none has been heard from either
	sleepGap             = 20 * time.Second  // a longer pause than this between checks means the PC was asleep
)

type presenceState struct {
	mu        sync.Mutex
	windows   map[string]time.Time // open pages -> when each was last heard from
	hadWindow bool                 // a page has been open at some point
	lastHeard time.Time            // the last request of any kind
}

var presence = &presenceState{windows: map[string]time.Time{}, lastHeard: time.Now()}

// heard notes a request; id is the page's name, or "" for requests that carry none.
func (p *presenceState) heard(id string) {
	now := time.Now()
	p.mu.Lock()
	p.lastHeard = now
	if id != "" && len(id) <= 64 {
		p.windows[id] = now
		p.hadWindow = true
	}
	p.mu.Unlock()
}

// bye notes that a page has closed (or is reloading).
func (p *presenceState) bye(id string) {
	p.mu.Lock()
	delete(p.windows, id)
	p.lastHeard = time.Now()
	p.mu.Unlock()
}

// shouldLeave reports whether the program should stop now. woke is true when the computer has
// just come out of sleep, in which case every page gets a fresh chance to be heard from.
func (p *presenceState) shouldLeave(now time.Time, woke bool) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if woke {
		p.lastHeard = now
		for id := range p.windows {
			p.windows[id] = now
		}
		return false
	}
	for id, seen := range p.windows {
		if now.Sub(seen) > leaveWhenSilent {
			delete(p.windows, id) // closed without saying so, for example when the browser was ended
		}
	}
	silent := now.Sub(p.lastHeard)
	if p.hadWindow && len(p.windows) == 0 && silent > leaveAfterLastWindow {
		return true
	}
	return silent > leaveWhenSilent
}

// watchdog ends the program once its last window has closed and nothing is running.
func watchdog() {
	last := time.Now()
	for {
		time.Sleep(time.Second)
		now := time.Now()
		woke := now.Sub(last) > sleepGap
		last = now
		if jobs.Running() {
			continue // a task carries on to the end even with the window closed
		}
		if presence.shouldLeave(now, woke) {
			logf("window closed; exiting")
			os.Exit(0)
		}
	}
}
