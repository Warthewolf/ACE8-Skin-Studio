package main

import (
	"strconv"
	"strings"
	"testing"
)

// A long log is cut short, and the window, asking for the lines after the last ones it got (as
// watchJob does), still gets each line once and in order.
func TestJobLogCutShort(t *testing.T) {
	j := testJob()
	var got []string
	since := 0
	poll := func() {
		v := j.view(since)
		since = v.Next
		got = append(got, v.Lines...)
	}
	const total = 12000
	for i := 0; i < total; i++ {
		j.Log("line %d", i)
		quiet := i > 6000 && i < 11000 // more lines between two looks than the log keeps
		if i%50 == 0 && !quiet {
			poll()
		}
	}
	poll()
	if len(j.Lines) > logMax {
		t.Fatalf("log kept %d lines", len(j.Lines))
	}
	last, shown, missed, notes := -1, 0, 0, 0
	for _, l := range got {
		n, err := strconv.Atoi(strings.TrimPrefix(l, "line "))
		if err != nil {
			if !strings.Contains(l, "left out") {
				t.Fatalf("unexpected line %q", l)
			}
			notes++
			continue
		}
		if n <= last {
			t.Fatalf("line %d shown again after line %d", n, last)
		}
		shown++
		missed += n - last - 1
		last = n
	}
	if last != total-1 || shown+missed != total {
		t.Fatalf("last line shown %d; %d shown and %d missed of %d", last, shown, missed, total)
	}
	// only lines taken out before the window looked are missing, and the window is told
	if missed == 0 || notes == 0 || missed > j.dropped {
		t.Fatalf("%d lines missed (%d notes), %d taken out", missed, notes, j.dropped)
	}
	// from the start (the support report): the first lines, a note, the newest lines
	v := j.view(0)
	if len(v.Lines) != len(j.Lines)+1 || v.Lines[0] != "line 0" || !strings.Contains(v.Lines[logHead], "left out") ||
		v.Lines[len(v.Lines)-1] != "line 11999" || v.Next != total {
		t.Fatalf("whole view: %d lines, next %d", len(v.Lines), v.Next)
	}
	// nothing new: nothing sent again
	if v := j.view(total); len(v.Lines) != 0 {
		t.Fatalf("%d lines sent again", len(v.Lines))
	}
}
