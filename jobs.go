package main

// Background jobs (one at a time) with a progress value and a log the window can poll.

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

type Job struct {
	mu       sync.Mutex
	ID       string
	Title    string
	Status   string // running, done, failed, cancelled
	Step     string
	Progress float64 // 0..1, or -1 when unknown
	Lines    []string
	Result   any
	Err      string
	Started  time.Time
	Ended    time.Time
	ctx      context.Context
	cancel   context.CancelFunc
	secrets  []string
	toolTail []string // the last lines the most recent external program printed
}

func (j *Job) Log(format string, a ...any) {
	s := fmt.Sprintf(format, a...)
	j.mu.Lock()
	for _, sec := range j.secrets {
		if sec != "" {
			s = strings.ReplaceAll(s, sec, "<hidden>")
		}
	}
	j.Lines = append(j.Lines, s)
	if len(j.Lines) > 5000 {
		j.Lines = append(j.Lines[:100], j.Lines[len(j.Lines)-4000:]...)
	}
	j.mu.Unlock()
	logf("[job %s] %s", j.ID, s)
}

func (j *Job) SetStep(step string, progress float64) {
	j.mu.Lock()
	j.Step, j.Progress = step, progress
	j.mu.Unlock()
}

func (j *Job) SetProgress(done, total int) {
	if total <= 0 {
		return
	}
	j.mu.Lock()
	j.Progress = float64(done) / float64(total)
	j.mu.Unlock()
}

func (j *Job) Cancelled() bool { return j.ctx.Err() != nil }

type jobView struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Status   string   `json:"status"`
	Step     string   `json:"step"`
	Progress float64  `json:"progress"`
	Lines    []string `json:"lines"`
	Next     int      `json:"next"`
	Result   any      `json:"result,omitempty"`
	Error    string   `json:"error,omitempty"`
	Seconds  float64  `json:"seconds"`
}

func (j *Job) view(since int) jobView {
	j.mu.Lock()
	defer j.mu.Unlock()
	if since < 0 || since > len(j.Lines) {
		since = 0
	}
	end := j.Ended
	if end.IsZero() {
		end = time.Now()
	}
	return jobView{ID: j.ID, Title: j.Title, Status: j.Status, Step: j.Step, Progress: j.Progress,
		Lines: append([]string{}, j.Lines[since:]...), Next: len(j.Lines), Result: j.Result, Error: j.Err,
		Seconds: end.Sub(j.Started).Seconds()}
}

type jobManager struct {
	mu      sync.Mutex
	jobs    map[string]*Job
	current *Job
	seq     int
}

var jobs = &jobManager{jobs: map[string]*Job{}}

var errBusy = errors.New("another task is still running; wait for it to finish or cancel it")

// Start runs fn in the background. Only one job runs at a time.
func (m *jobManager) Start(title string, secrets []string, fn func(j *Job) (any, error)) (*Job, error) {
	m.mu.Lock()
	if m.current != nil && m.current.statusNow() == "running" {
		m.mu.Unlock()
		return nil, errBusy
	}
	m.seq++
	ctx, cancel := context.WithCancel(context.Background())
	j := &Job{ID: fmt.Sprintf("j%d", m.seq), Title: title, Status: "running", Progress: -1, Started: time.Now(), ctx: ctx, cancel: cancel, secrets: secrets}
	m.jobs[j.ID] = j
	m.current = j
	m.mu.Unlock()
	go func() {
		var res any
		var err error
		func() {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("unexpected problem: %v", r)
				}
			}()
			res, err = fn(j)
		}()
		j.mu.Lock()
		j.Ended = time.Now()
		j.Result = res
		switch {
		case err != nil && ctx.Err() != nil:
			j.Status, j.Err = "cancelled", "Cancelled."
		case err != nil:
			j.Status, j.Err = "failed", err.Error()
		default:
			j.Status, j.Progress = "done", 1
		}
		j.mu.Unlock()
		if err != nil {
			logf("[job %s] ended: %v", j.ID, err)
		}
		cancel()
		invalidateLibrary()
		debug.FreeOSMemory() // large pictures were held in memory; give it back
	}()
	return j, nil
}

func init() {
	debug.SetGCPercent(40)
}

func (j *Job) statusNow() string {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.Status
}

func (m *jobManager) Get(id string) *Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.jobs[id]
}

// Last returns the most recently started job, or nil.
func (m *jobManager) Last() *Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.current
}

func (m *jobManager) Running() bool {
	m.mu.Lock()
	c := m.current
	m.mu.Unlock()
	return c != nil && c.statusNow() == "running"
}

var reANSI = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

// runTool runs an external program and feeds its output to the job log line by line.
func runTool(j *Job, exe string, args []string, onLine func(string)) error {
	if isRetoc(exe) && needsOodle(args) {
		if err := ensureOodle(j, exe, getSettings().PaksDir); err != nil {
			return err
		}
	}
	cmd := exec.CommandContext(j.ctx, exe, args...)
	hideWindow(cmd)
	pr, pw := io.Pipe()
	cmd.Stdout, cmd.Stderr = pw, pw
	if err := cmd.Start(); err != nil {
		pw.Close()
		return fmt.Errorf("%s could not be started (%v). Check that the file is still there; antivirus programs sometimes remove or block it", exeBase(exe), err)
	}
	done := make(chan struct{})
	var tail []string
	go func() {
		defer close(done)
		scanToolLines(pr, func(line string) {
			tail = append(tail, line)
			if len(tail) > 60 {
				tail = tail[len(tail)-60:]
			}
			if onLine != nil {
				onLine(line)
			} else {
				j.Log("%s", line)
			}
		})
	}()
	err := cmd.Wait()
	pw.Close()
	<-done
	j.mu.Lock()
	j.toolTail = tail
	j.mu.Unlock()
	if j.ctx.Err() != nil {
		return errors.New("cancelled")
	}
	if err != nil {
		packing := false
		for _, a := range args {
			if a == "to-zen" {
				packing = true
			}
		}
		why := explainToolFailure(tail, packing)
		logf("[job %s] %s ended with %v", j.ID, exeBase(exe), err)
		return fmt.Errorf("%s What %s printed is under Show details.", why, strings.TrimSuffix(exeBase(exe), ".exe"))
	}
	return nil
}

// runToolTTY runs a tool whose error output goes to a pipe it takes for a terminal (ttyPipe), so
// it keeps the messages it would otherwise drop, and passes each line of everything it prints to
// onLine (with the progress display's own lines among them).
func runToolTTY(j *Job, exe string, args []string, onLine func(string)) error {
	if isRetoc(exe) && needsOodle(args) {
		if err := ensureOodle(j, exe, getSettings().PaksDir); err != nil {
			return err
		}
	}
	tr, tw, err := ttyPipe()
	if err != nil {
		return runTool(j, exe, args, onLine) // without the messages it keeps for terminals
	}
	cmd := exec.CommandContext(j.ctx, exe, args...)
	hideWindow(cmd)
	pr, pw := io.Pipe()
	cmd.Stdout, cmd.Stderr = pw, tw
	if err := cmd.Start(); err != nil {
		tw.Close()
		tr.Close()
		pw.Close()
		return err
	}
	tw.Close() // the program has its own copy; the pipe ends when it does
	var wg sync.WaitGroup
	var mu sync.Mutex
	read := func(r io.Reader) {
		defer wg.Done()
		scanToolLines(r, func(line string) {
			mu.Lock()
			defer mu.Unlock()
			onLine(line)
		})
	}
	wg.Add(2)
	go read(pr)
	go read(tr)
	err = cmd.Wait()
	pw.Close()
	wg.Wait()
	tr.Close()
	if j.ctx.Err() != nil {
		return errors.New("cancelled")
	}
	return err
}

// scanToolLines reads a tool's output line by line (lines end at \n or \r), without its colour
// codes, and passes each line that is not empty to fn.
func scanToolLines(r io.Reader, fn func(string)) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	sc.Split(func(data []byte, atEOF bool) (int, []byte, error) {
		for i, b := range data {
			if b == '\n' || b == '\r' {
				return i + 1, data[:i], nil
			}
		}
		if atEOF && len(data) > 0 {
			return len(data), data, nil
		}
		return 0, nil, nil
	})
	for sc.Scan() {
		if line := strings.TrimRight(reANSI.ReplaceAllString(sc.Text(), ""), " \t"); line != "" {
			fn(line)
		}
	}
	io.Copy(io.Discard, r)
}

// explainToolFailure turns what retoc printed before it stopped into advice in plain words.
// packing is true when a mod was being packed, false when the game's files were being read.
func explainToolFailure(tail []string, packing bool) string {
	text := strings.ToLower(strings.Join(tail, "\n"))
	has := func(parts ...string) bool {
		for _, p := range parts {
			if strings.Contains(text, p) {
				return true
			}
		}
		return false
	}
	switch {
	case has("os error 112", "no space left", "not enough space", "disk full", "disk is full"):
		return "The disk is full. One aircraft's textures take roughly 2 to 4 GB; free some space, or move the workspace folder in Settings."
	case has("os error 32", "being used by another process"):
		return "A file is in use by another program. Close the game (and FModel, if it is open) and try again."
	case has("os error 5", "access is denied", "permission denied"):
		return "Windows refused access to a file. Use a workspace folder you can write to (not inside Program Files), and check that your antivirus is not blocking retoc."
	case has("invalid aes key"):
		return "retoc does not accept the AES key. Check it in Settings: it must be 64 hexadecimal characters, optionally starting with 0x."
	case has("missing encryption key", "no aes key"):
		return "The AES key in Settings does not open the game's files. The key is wrong for this version of the game, or the game has updated and uses a new key."
	case has("invalid eiostoretocversion", "unrecognized toc magic", "invalid eiocontainerflags", "unknown compression method"):
		return "This version of retoc does not understand the game's files. The game has probably updated; get the newest retoc from github.com/trumank/retoc/releases."
	case has("oodle") && !has("oodle decompression failed"):
		return "retoc could not load the Oodle library it needs. It downloads that file the first time it runs, so check that you are online and that your antivirus has not blocked it."
	case packing && has("pkg_cooked", "pkg_filtereditoronly", "uncooked"):
		return "retoc refused one of the files because it is not in the game's cooked format."
	case packing:
		return "retoc could not pack the mod."
	case has("oodle decompression failed", "hash mismatch", "failed to fill whole buffer", "unexpected end of file", "invalid utf-8", "panicked", "failed to read chunk"):
		return "retoc could not read the game's files. The AES key is probably wrong, or the game has updated (which can change the key and can need a newer retoc). Check the key in Settings first."
	case has("os error 2", "os error 3", "cannot find the path", "cannot find the file", "no such file"):
		return "A folder or file retoc needs was not found. Check the Paks folder in Settings."
	}
	return "retoc could not read the game's files. If this happens for every aircraft, the AES key is probably wrong or the game has updated; check the key in Settings."
}

func exeBase(p string) string {
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}
