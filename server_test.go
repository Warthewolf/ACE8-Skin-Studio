package main

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// serverTest gives the tests a token, a fresh set of jobs, stand-ins for opening windows and
// ending the program, and a usual port nothing listens on, and puts everything back afterwards.
func serverTest(t *testing.T) (h http.Handler, exited, shown chan bool) {
	t.Helper()
	oldTok, oldJobs, oldExit, oldShow, oldData, oldPort := token, jobs, exitProgram, showWindow, dataDir, preferredPort
	t.Cleanup(func() {
		token, jobs, exitProgram, showWindow, dataDir, preferredPort = oldTok, oldJobs, oldExit, oldShow, oldData, oldPort
	})
	if ln, err := net.Listen("tcp", "127.0.0.1:0"); err == nil {
		preferredPort = ln.Addr().(*net.TCPAddr).Port
		ln.Close()
	}
	token = "test-token"
	jobs = &jobManager{jobs: map[string]*Job{}}
	exited, shown = make(chan bool, 8), make(chan bool, 8)
	exitProgram = func(int) { exited <- true }
	showWindow = func() error { shown <- true; return nil }
	dataDir = t.TempDir()
	return routes(), exited, shown
}

func call(h http.Handler, method, path, body string, header ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Host = "127.0.0.1:48189"
	req.Header.Set("X-AC8-Token", token)
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func waitFor(t *testing.T, ch chan bool, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatalf("%s did not happen", what)
	}
}

func TestQuitWhileTaskRuns(t *testing.T) {
	h, exited, _ := serverTest(t)
	j, err := jobs.Start("Extract", nil, func(j *Job) (any, error) {
		<-j.ctx.Done()
		return nil, errors.New("cancelled")
	})
	if err != nil {
		t.Fatal(err)
	}
	// not without being told to: the task is never stopped part-way
	for _, body := range []string{"", "{}", `{"force":false}`} {
		rec := call(h, "POST", "/api/quit", body)
		var v struct {
			Error string
			Busy  bool
		}
		json.Unmarshal(rec.Body.Bytes(), &v)
		if rec.Code != http.StatusConflict || !v.Busy || !strings.Contains(v.Error, "task is running") {
			t.Fatalf("quit with %q while busy: %d %s", body, rec.Code, rec.Body)
		}
	}
	select {
	case <-exited:
		t.Fatal("the program quit while a task was running")
	case <-time.After(500 * time.Millisecond):
	}
	if j.statusNow() != "running" {
		t.Fatalf("task %s", j.statusNow())
	}
	// told to: the task is cancelled first, then the program quits
	if rec := call(h, "POST", "/api/quit", `{"force":true}`); rec.Code != 200 {
		t.Fatalf("forced quit: %d %s", rec.Code, rec.Body)
	}
	if j.statusNow() != "cancelled" {
		t.Fatalf("task %s after a forced quit", j.statusNow())
	}
	waitFor(t, exited, "quitting")
	// nothing running: quits as before
	if rec := call(h, "POST", "/api/quit", ""); rec.Code != 200 {
		t.Fatalf("quit: %d %s", rec.Code, rec.Body)
	}
	waitFor(t, exited, "quitting")
}

func TestWindowOpensWithLaunchCode(t *testing.T) {
	h, _, _ := serverTest(t)
	u := windowURL()
	if strings.Contains(u, token) {
		t.Fatalf("the access token is in the window's address: %s", u)
	}
	code := strings.TrimPrefix(u[strings.Index(u, "/launch"):], "/launch?code=")
	rec := call(h, "GET", "/launch?code="+code, "")
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/?t="+token {
		t.Fatalf("launch: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	// once only
	if rec := call(h, "GET", "/launch?code="+code, ""); rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "start "+appName+" again") {
		t.Fatalf("a used code worked: %d %s", rec.Code, rec.Body)
	}
	for _, c := range []string{"", "0123", token} {
		if rec := call(h, "GET", "/launch?code="+c, ""); rec.Code != http.StatusForbidden {
			t.Fatalf("code %q worked", c)
		}
	}
	// for a short time only
	code = newLaunchCode()
	launchMu.Lock()
	launchCodes[code] = time.Now().Add(-time.Second)
	launchMu.Unlock()
	if rec := call(h, "GET", "/launch?code="+code, ""); rec.Code != http.StatusForbidden {
		t.Fatal("an old code worked")
	}
	// not through another site's name; the code still works for the window afterwards
	code = newLaunchCode()
	req := httptest.NewRequest("GET", "/launch?code="+code, nil)
	req.Host = "evil.example:48189"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatal("launch through another name")
	}
	if rec := call(h, "GET", "/launch?code="+code, ""); rec.Code != http.StatusSeeOther {
		t.Fatalf("the window's code was used up: %d", rec.Code)
	}
	// over a real connection from this user's own program (on Linux the connection's owner is checked)
	srv := httptest.NewServer(h)
	defer srv.Close()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Get(srv.URL + "/launch?code=" + newLaunchCode())
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("launch from this user: %d", resp.StatusCode)
	}
	// the token still opens everything as before
	if rec := call(h, "GET", "/api/ping", ""); rec.Code != 200 {
		t.Fatalf("token: %d", rec.Code)
	}
	if rec := call(h, "GET", "/api/ping", "", "X-AC8-Token", "wrong"); rec.Code != http.StatusForbidden {
		t.Fatalf("wrong token: %d", rec.Code)
	}
}

func TestShowOnlyForSameDataFolder(t *testing.T) {
	h, _, shown := serverTest(t)
	srv := httptest.NewServer(h)
	defer srv.Close()
	post := func(body string) string {
		resp, err := http.Post(srv.URL+"/api/show", "text/plain", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return string(b)
	}
	if got := post(t.TempDir()); got != "other" {
		t.Fatalf("another data folder: %q", got)
	}
	if got := post(""); got != "other" {
		t.Fatalf("no data folder: %q", got)
	}
	select {
	case <-shown:
		t.Fatal("window shown for another data folder")
	case <-time.After(200 * time.Millisecond):
	}
	if got := post(dataDir + string(filepath.Separator)); got != "ok" {
		t.Fatalf("same data folder: %q", got)
	}
	waitFor(t, shown, "showing the window")
	// a second launch finds the running copy on the port it wrote down, when it is not the usual one
	port := srv.Listener.Addr().(*net.TCPAddr).Port
	os.WriteFile(filepath.Join(dataDir, portFileName), []byte(strconv.Itoa(port)), 0o644)
	if !showRunningCopy() {
		t.Fatal("running copy not found by its port file")
	}
	waitFor(t, shown, "showing the window")
}

func TestSecondLaunchKeepsTheLog(t *testing.T) {
	h, _, shown := serverTest(t)
	srv := httptest.NewServer(h) // the copy already running, for dataDir
	defer srv.Close()
	os.WriteFile(filepath.Join(dataDir, portFileName), []byte(strconv.Itoa(srv.Listener.Addr().(*net.TCPAddr).Port)), 0o644)
	logPath := filepath.Join(dataDir, "ACE8SkinStudio.log")
	const before = "10:00:00 ACE8 Skin Studio starting\n10:00:01 what the running copy logged\n"
	os.WriteFile(logPath, []byte(before), 0o644)
	done := make(chan int, 1)
	go func() { done <- runApp([]string{"-data", dataDir, "-no-open", "-stay"}) }()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("second launch ended with %d", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("a second copy started instead of showing the running one")
	}
	waitFor(t, shown, "showing the running copy's window")
	if b, _ := os.ReadFile(logPath); string(b) != before {
		t.Fatalf("the running copy's log was changed: %q", b)
	}
}

func TestThumbAsksAgainAfterChange(t *testing.T) {
	h, _, _ := serverTest(t)
	p := filepath.Join(t.TempDir(), "icon.png")
	if err := SaveImage(p, NewImg(64, 64), SaveRGB); err != nil {
		t.Fatal(err)
	}
	get := func(etag string) *httptest.ResponseRecorder {
		return call(h, "GET", "/api/thumb?w=32&path="+url.QueryEscape(p), "", "If-None-Match", etag)
	}
	first := get("")
	tag := first.Header().Get("ETag")
	if first.Code != 200 || tag == "" || !strings.Contains(first.Header().Get("Cache-Control"), "no-cache") {
		t.Fatalf("first: %d %q %q", first.Code, tag, first.Header().Get("Cache-Control"))
	}
	if rec := get(tag); rec.Code != http.StatusNotModified || rec.Body.Len() != 0 {
		t.Fatalf("unchanged file: %d", rec.Code)
	}
	// the icon rebuilt under the same name
	big := NewImg(128, 64)
	for i := range big.Pix {
		big.Pix[i] = 200
	}
	if err := SaveImage(p, big, SaveRGB); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(2 * time.Second)
	os.Chtimes(p, later, later)
	rec := get(tag)
	if rec.Code != 200 || rec.Header().Get("ETag") == tag {
		t.Fatalf("changed file: %d, tag %q", rec.Code, rec.Header().Get("ETag"))
	}
	// another size of the same picture is another answer
	if rec := call(h, "GET", "/api/thumb?w=64&path="+url.QueryEscape(p), "", "If-None-Match", rec.Header().Get("ETag")); rec.Code != 200 {
		t.Fatalf("other width: %d", rec.Code)
	}
	if rec := call(h, "GET", "/api/thumb?path="+url.QueryEscape(p+".gone"), ""); rec.Code != 404 || rec.Header().Get("ETag") != "" {
		t.Fatalf("missing file: %d", rec.Code)
	}
}
