package main

// The program's window is a local web page served on 127.0.0.1 and shown in an app window.

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed ui/index.html
var indexHTML []byte

var (
	token   string
	baseURL string
)

// preferredPort is the usual port (a variable so tests can keep clear of a copy running on it).
var preferredPort = 48189

func runApp(args []string) int {
	fs := flag.NewFlagSet("app", flag.ContinueOnError)
	port := fs.Int("port", 0, "port to listen on (default: automatic)")
	noOpen := fs.Bool("no-open", false, "do not open the window")
	data := fs.String("data", "", "folder for settings.json (default: next to the program)")
	tok := fs.String("token", "", "fixed access token (testing)")
	stay := fs.Bool("stay", false, "keep running when the window closes")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	dataDir = *data
	if dataDir == "" {
		exe, err := os.Executable()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		dataDir = filepath.Dir(exe)
	}
	os.MkdirAll(dataDir, 0o755)

	// If the program is already running with this data folder, bring its window up instead of
	// starting a second copy. This is asked before the log is opened: opening it empties it, and
	// the log is the running copy's.
	if *port == 0 && showRunningCopy() {
		return 0
	}
	token = *tok
	if token == "" {
		b := make([]byte, 16)
		rand.Read(b)
		token = hex.EncodeToString(b)
	}
	var ln net.Listener
	var err error
	tryPorts := []int{*port}
	if *port == 0 {
		tryPorts = []int{preferredPort, 0}
	}
	for _, p := range tryPorts {
		if ln, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p)); err == nil {
			break
		}
	}
	if err != nil {
		// added to the end of the log, which may be another copy's
		logFile, _ = os.OpenFile(filepath.Join(dataDir, "ACE8SkinStudio.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		logf("%s %s cannot listen: %v", appName, appVersion, err)
		fmt.Fprintln(os.Stderr, "cannot start:", err)
		return 1
	}
	// This copy serves the data folder from here on, so the log is its own.
	logFile, _ = os.Create(filepath.Join(dataDir, "ACE8SkinStudio.log"))
	logf("%s %s starting on %s", appName, appVersion, runtime.GOOS)
	loadSettings()
	listenPort := ln.Addr().(*net.TCPAddr).Port
	baseURL = fmt.Sprintf("http://127.0.0.1:%d", listenPort)
	logf("listening on %s", baseURL)
	// where a second launch finds this copy when it could not have the usual port
	os.WriteFile(filepath.Join(dataDir, portFileName), []byte(strconv.Itoa(listenPort)), 0o644)
	fmt.Println(baseURL + "/?t=" + token)
	purgeInbox(getSettings())

	presence.heard("")
	if !*noOpen {
		if err := showWindow(); err != nil {
			logf("could not open the window: %v", err)
		}
	}
	if !*stay {
		go watchdog()
	}
	srv := &http.Server{Handler: routes(), ReadHeaderTimeout: 10 * time.Second}
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logf("server stopped: %v", err)
		return 1
	}
	return 0
}

// portFileName is the file in the data folder that holds the port the running copy listens on.
const portFileName = "ACE8SkinStudio.port"

// showRunningCopy asks a copy of the program that is already running with the same data folder to
// show its window, and reports whether one did. It is looked for on the usual port and on the port
// the last copy for this data folder wrote down.
func showRunningCopy() bool {
	dir := dataDir
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	ports := []int{preferredPort}
	if b, err := os.ReadFile(filepath.Join(dataDir, portFileName)); err == nil {
		if p, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil && p > 0 && p < 65536 && p != preferredPort {
			ports = append(ports, p)
		}
	}
	client := &http.Client{Timeout: 2 * time.Second}
	for _, p := range ports {
		resp, err := client.Post(fmt.Sprintf("http://127.0.0.1:%d/api/show", p), "text/plain", strings.NewReader(dir))
		if err != nil {
			continue
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 64))
		resp.Body.Close()
		if resp.StatusCode == 200 && strings.TrimSpace(string(b)) == "ok" {
			return true
		}
	}
	return false
}

// sameFolder tells whether two paths name the same folder.
func sameFolder(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	sa, errA := os.Stat(a)
	sb, errB := os.Stat(b)
	if errA == nil && errB == nil {
		return os.SameFile(sa, sb)
	}
	a, b = filepath.Clean(a), filepath.Clean(b)
	return a == b || (runtime.GOOS == "windows" && strings.EqualFold(a, b))
}

// The window is opened with a launch code, not the access token: a browser's command line can be
// read by the other users of the computer (on Linux, in /proc), and a launch code works only once,
// for a short time, and only for this user. The window's first request (/launch) swaps it for the
// page with the token.
var (
	launchMu    sync.Mutex
	launchCodes = map[string]time.Time{} // code -> until when it works
)

const launchCodeLife = 2 * time.Minute

func newLaunchCode() string {
	b := make([]byte, 16)
	rand.Read(b)
	code := hex.EncodeToString(b)
	now := time.Now()
	launchMu.Lock()
	defer launchMu.Unlock()
	for c, until := range launchCodes {
		if now.After(until) {
			delete(launchCodes, c)
		}
	}
	launchCodes[code] = now.Add(launchCodeLife)
	return code
}

// useLaunchCode reports whether code is a launch code that still works, and makes it unusable.
func useLaunchCode(code string) bool {
	launchMu.Lock()
	defer launchMu.Unlock()
	until, ok := launchCodes[code]
	delete(launchCodes, code)
	return ok && time.Now().Before(until)
}

// windowURL is the address a new window is opened with.
func windowURL() string { return baseURL + "/launch?code=" + newLaunchCode() }

// showWindow opens a window on the program (a variable so tests do not open one).
var showWindow = func() error { return openURL(windowURL(), !getSettings().DefaultBrowser) }

// exitProgram ends the program (a variable so tests can stand in for it).
var exitProgram = os.Exit

// quitWait is how long Quit waits for a cancelled task to stop.
const quitWait = 5 * time.Second

// purgeInbox deletes uploaded copies left over from earlier runs.
func purgeInbox(s Settings) {
	es, err := os.ReadDir(s.inboxDir())
	if err != nil {
		return
	}
	for _, e := range es {
		if e.IsDir() && reBatch.MatchString(e.Name()) {
			os.RemoveAll(filepath.Join(s.inboxDir(), e.Name()))
		}
	}
}

// ------------------------------------------------------------------ http helpers

type apiError struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, apiError{err.Error()})
}

func readJSON(r *http.Request, v any) error {
	b, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(b)) == 0 {
		return nil
	}
	return json.Unmarshal(b, v)
}

// guard rejects requests that do not come from the program's own window.
func guard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !localHost(r.Host) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		t := r.Header.Get("X-AC8-Token")
		if t == "" {
			t = r.URL.Query().Get("t")
		}
		if t != token {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if r.URL.Path != "/api/bye" {
			presence.heard(clientID(r))
		}
		next(w, r)
	}
}

// localHost tells whether a request was sent to this computer by name (not through another site's
// name that leads here).
func localHost(host string) bool {
	return strings.HasPrefix(host, "127.0.0.1:") || strings.HasPrefix(host, "localhost:")
}

// clientID is the name the page that sent a request gave itself.
func clientID(r *http.Request) string {
	if c := r.Header.Get("X-AC8-Client"); c != "" {
		return c
	}
	return r.URL.Query().Get("c")
}

func startJob(w http.ResponseWriter, title string, secrets []string, fn func(j *Job) (any, error)) {
	if p := workspaceProblem(getSettings().Workspace); p != "" {
		fail(w, http.StatusConflict, errors.New(p+" Open Settings and choose another Workspace folder, or press \"Use the folder next to the program\" there."))
		return
	}
	j, err := jobs.Start(title, secrets, fn)
	if err != nil {
		fail(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, 200, map[string]string{"job": j.ID})
}

type stateView struct {
	App      string            `json:"app"`
	Version  string            `json:"version"`
	Settings Settings          `json:"settings"`
	Checks   map[string]check  `json:"checks"`
	Folders  map[string]string `json:"folders"`
	CanPick  bool              `json:"canPick"`
	Busy     string            `json:"busy"`
}

func currentState() stateView {
	s := getSettings()
	busy := ""
	jobs.mu.Lock()
	if jobs.current != nil && jobs.current.statusNow() == "running" {
		busy = jobs.current.ID
	}
	jobs.mu.Unlock()
	return stateView{App: appName, Version: appVersion, Settings: s, Checks: s.checks(), CanPick: canPick, Busy: busy,
		Folders: map[string]string{"data": dataDir, "defaultWorkspace": defaultWorkspace(), "raw": s.rawDir(), "paint": s.paintDir(), "split": s.splitDir(), "mods": s.modsOut(), "models": s.modelsDir(), "removedMods": s.removedModsDir(),
			"shared": s.exportsDir(), "slotMods": s.slotModsDir(), "ue4ss": s.ue4ssModsDir(), "pilots": s.pilotsDir()}}
}

// ------------------------------------------------------------------ library cache

var libMu sync.Mutex
var libCache []LibSkin
var libValid bool

// cachedLibrary scans the raw folders once and remembers the answer until something changes.
func cachedLibrary(fresh bool) []LibSkin {
	libMu.Lock()
	defer libMu.Unlock()
	if !libValid || fresh {
		libCache = scanLibrary(getSettings())
		if libCache == nil {
			libCache = []LibSkin{}
		}
		libValid = true
	}
	return libCache
}

func invalidateLibrary() {
	libMu.Lock()
	libValid = false
	libMu.Unlock()
}

// ------------------------------------------------------------------ pictures: cache and thumbnails

var thumbMu sync.Mutex
var thumbCache = map[string][]byte{}

// Decoding a 4096 x 4096 picture takes about 70 MB, so only two are decoded at a time.
var thumbSlots = make(chan struct{}, 2)

const thumbCacheMax = 400

var thumbTidy *time.Timer

// tidyAfterThumbs hands memory back to Windows shortly after the last preview was made.
func tidyAfterThumbs() {
	thumbMu.Lock()
	defer thumbMu.Unlock()
	if thumbTidy != nil {
		thumbTidy.Stop()
	}
	thumbTidy = time.AfterFunc(3*time.Second, func() {
		if !jobs.Running() {
			debug.FreeOSMemory()
		}
	})
}

func fileKey(p string) string {
	st, err := os.Stat(p)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%s|%d|%d", p, st.Size(), st.ModTime().UnixNano())
}

// makeThumb makes a small copy of a picture. Its see-through parts are kept only when alpha is set
// (emblems): in textures the alpha often holds other data.
func makeThumb(p string, maxW int, alpha bool) ([]byte, error) {
	key := fileKey(p)
	if key == "" {
		return nil, errors.New("file not found")
	}
	key += "|" + strconv.Itoa(maxW) + "|" + strconv.FormatBool(alpha)
	thumbMu.Lock()
	if b, ok := thumbCache[key]; ok {
		thumbMu.Unlock()
		return b, nil
	}
	thumbMu.Unlock()
	thumbSlots <- struct{}{}
	defer func() { <-thumbSlots }()
	thumbMu.Lock()
	if b, ok := thumbCache[key]; ok { // made by another request while this one waited
		thumbMu.Unlock()
		return b, nil
	}
	thumbMu.Unlock()
	defer tidyAfterThumbs()
	im, err := LoadImage(p)
	if err != nil {
		return nil, err
	}
	w := min(im.W, maxW)
	h := max(1, im.H*w/im.W)
	small := resize(im, w, h)
	var buf bytes.Buffer
	mode := SaveRGB
	if alpha {
		mode = SaveRGBA
	}
	if err := encodePNG(&buf, small, mode); err != nil {
		return nil, err
	}
	thumbMu.Lock()
	if len(thumbCache) > thumbCacheMax {
		thumbCache = map[string][]byte{}
	}
	thumbCache[key] = buf.Bytes()
	thumbMu.Unlock()
	return buf.Bytes(), nil
}

// textureThumb makes a small preview of a raw game texture from one of its stored smaller copies.
func textureThumb(uexpPath string, maxW int) ([]byte, error) {
	key := fileKey(uexpPath)
	if key == "" || !strings.EqualFold(filepath.Ext(uexpPath), ".uexp") {
		return nil, errors.New("file not found")
	}
	key = "tex|" + key
	thumbMu.Lock()
	if b, ok := thumbCache[key]; ok {
		thumbMu.Unlock()
		return b, nil
	}
	thumbMu.Unlock()
	b, _, err := texturePreviewPNG(uexpPath, maxW)
	if err != nil {
		return nil, err
	}
	thumbMu.Lock()
	if len(thumbCache) > thumbCacheMax {
		thumbCache = map[string][]byte{}
	}
	thumbCache[key] = b
	thumbMu.Unlock()
	return b, nil
}

// notChanged lets the browser keep a preview only while its file stays the same: a rebuilt icon is
// written over the old one under the same name, so a preview kept by its address alone would show
// the old picture. It marks the answer with a tag made from the file's size and time (key, from
// fileKey) and what was asked, and answers "not changed" when the browser already has that one.
func notChanged(w http.ResponseWriter, r *http.Request, key string, asked ...any) bool {
	w.Header().Set("Cache-Control", "private, no-cache")
	if key == "" {
		return false
	}
	h := sha256.Sum256([]byte(fmt.Sprint(key, asked)))
	tag := `"` + hex.EncodeToString(h[:12]) + `"`
	w.Header().Set("ETag", tag)
	if r.Header.Get("If-None-Match") == tag {
		w.WriteHeader(http.StatusNotModified)
		return true
	}
	return false
}

type inspectRow struct {
	Path   string `json:"path"`
	Name   string `json:"name"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Pieces int    `json:"pieces"`
	Kind   Kind   `json:"kind"`
	Error  string `json:"error,omitempty"`
}

func inspect(p string) inspectRow {
	row := inspectRow{Path: p, Name: filepath.Base(p)}
	w, h, err := ImageSize(p)
	if err != nil {
		row.Error = "This is not a PNG or TGA picture."
		return row
	}
	row.Width, row.Height = w, h
	row.Kind = GuessKind(strings.TrimSuffix(row.Name, filepath.Ext(row.Name)))
	if h > 0 && w%h == 0 {
		row.Pieces = w / h
	}
	return row
}

// ------------------------------------------------------------------ routes

func routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'; img-src 'self' blob: data:")
		w.Write(indexHTML)
	})
	// The window's first request: the launch code it was opened with is swapped for the page, with
	// the access token (see showWindow).
	mux.HandleFunc("/launch", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		refuse := func(why string) {
			logf("a window was refused: %s", why)
			http.Error(w, "This link to "+appName+" has already been used, or is too old. To open the program's window, start "+appName+" again.", http.StatusForbidden)
		}
		switch {
		case !localHost(r.Host):
			refuse("sent through another name")
		case !sameUserPeer(r):
			refuse("asked for by another user of this computer")
		case !useLaunchCode(r.URL.Query().Get("code")):
			refuse("the launch code was used before or is too old")
		default:
			http.Redirect(w, r, "/?t="+token, http.StatusSeeOther)
		}
	})
	// Another copy of the program asks this one to show its window (no token: it only opens our own
	// page). It is only for a copy started with the same data folder; one with another starts by itself.
	mux.HandleFunc("/api/show", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasPrefix(r.Host, "127.0.0.1:") || r.Header.Get("Origin") != "" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		b, _ := io.ReadAll(io.LimitReader(r.Body, 8<<10))
		if !sameFolder(strings.TrimSpace(string(b)), dataDir) {
			io.WriteString(w, "other")
			return
		}
		presence.heard("")
		io.WriteString(w, "ok")
		// after the answer, so the other copy is not kept waiting while the browser starts
		go func() {
			if err := showWindow(); err != nil {
				logf("could not open the window: %v", err)
			}
		}()
	})
	api := func(path string, h http.HandlerFunc) { mux.HandleFunc(path, guard(h)) }

	api("/api/state", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, currentState()) })
	api("/api/ping", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]bool{"ok": true}) })
	api("/api/bye", func(w http.ResponseWriter, r *http.Request) {
		// The window is closing (or reloading). The program leaves a few seconds after its last
		// window has gone, unless one comes back.
		presence.bye(clientID(r))
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
	api("/api/quit", func(w http.ResponseWriter, r *http.Request) {
		// {"force": true} quits even while a task runs; without it (or with no body) a running task
		// is never stopped part-way
		var q struct{ Force bool }
		readJSON(r, &q)
		if j := jobs.Last(); j != nil && j.statusNow() == "running" {
			if !q.Force {
				writeJSON(w, http.StatusConflict, map[string]any{"busy": true,
					"error": "A task is running (" + j.Title + "). Quitting now would stop it part-way, which can leave half-copied files in the mods folder. Wait for it to finish, or cancel it, then quit."})
				return
			}
			// quitting anyway: stop the task (and retoc with it), and give it a moment to end
			logf("quit requested while %q was running; cancelling it", j.Title)
			j.cancel()
			for end := time.Now().Add(quitWait); j.statusNow() == "running" && time.Now().Before(end); {
				time.Sleep(50 * time.Millisecond)
			}
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
		go func() { time.Sleep(300 * time.Millisecond); logf("quit requested"); exitProgram(0) }()
	})
	api("/api/settings", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			fail(w, 405, errors.New("POST only"))
			return
		}
		s := getSettings()
		if err := readJSON(r, &s); err != nil {
			fail(w, 400, err)
			return
		}
		if err := saveSettings(s); err != nil {
			fail(w, 500, err)
			return
		}
		invalidateLibrary()
		refreshSaveBackupMod(getSettings()) // the game-start backup follows a new save folder
		writeJSON(w, 200, currentState())
	})
	api("/api/pick", func(w http.ResponseWriter, r *http.Request) {
		var q struct{ Kind, Title, Filter, Start string }
		if err := readJSON(r, &q); err != nil {
			fail(w, 400, err)
			return
		}
		paths, err := pickDialog(q.Kind, q.Title, q.Filter, cleanPath(q.Start))
		if err != nil {
			fail(w, 500, err)
			return
		}
		writeJSON(w, 200, map[string]any{"paths": paths})
	})
	api("/api/open", func(w http.ResponseWriter, r *http.Request) {
		var q struct{ Path string }
		if err := readJSON(r, &q); err != nil {
			fail(w, 400, err)
			return
		}
		p := cleanPath(q.Path)
		if _, err := os.Stat(p); err != nil {
			fail(w, 404, errors.New("that folder does not exist yet"))
			return
		}
		if err := revealPath(p); err != nil {
			fail(w, 500, err)
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
	api("/api/upload", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			fail(w, 405, errors.New("POST only"))
			return
		}
		batch := r.URL.Query().Get("batch")
		name := filepath.Base(filepath.FromSlash(strings.ReplaceAll(r.URL.Query().Get("name"), `\`, "/")))
		if !reBatch.MatchString(batch) || name == "" || name == "." || name == ".." {
			fail(w, 400, errors.New("bad upload"))
			return
		}
		dir := filepath.Join(getSettings().inboxDir(), batch)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fail(w, 500, err)
			return
		}
		dst := filepath.Join(dir, name)
		f, err := os.Create(dst)
		if err != nil {
			fail(w, 500, err)
			return
		}
		_, err = io.Copy(f, r.Body)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			os.Remove(dst)
			fail(w, 500, fmt.Errorf("could not save %s: %v", name, err))
			return
		}
		writeJSON(w, 200, map[string]string{"path": dst})
	})
	api("/api/inbox/clean", func(w http.ResponseWriter, r *http.Request) {
		var q struct{ Batches []string }
		if err := readJSON(r, &q); err != nil {
			fail(w, 400, err)
			return
		}
		if !jobs.Running() {
			cleanInbox(getSettings(), q.Batches)
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
	api("/api/inspect", func(w http.ResponseWriter, r *http.Request) {
		var q struct{ Paths []string }
		if err := readJSON(r, &q); err != nil {
			fail(w, 400, err)
			return
		}
		rows := []inspectRow{}
		for _, p := range q.Paths {
			rows = append(rows, inspect(cleanPath(p)))
		}
		writeJSON(w, 200, map[string]any{"files": rows})
	})
	api("/api/thumb", func(w http.ResponseWriter, r *http.Request) {
		maxW := 1600
		if w, err := strconv.Atoi(r.URL.Query().Get("w")); err == nil {
			maxW = min(max(w, 32), 1600)
		}
		p, alpha := r.URL.Query().Get("path"), r.URL.Query().Get("alpha") == "1"
		if notChanged(w, r, fileKey(p), maxW, alpha) {
			return
		}
		b, err := makeThumb(p, maxW, alpha)
		if err != nil {
			w.Header().Del("ETag")
			http.Error(w, err.Error(), 404)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Write(b)
	})
	api("/api/texthumb", func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Query().Get("uexp")
		if notChanged(w, r, fileKey(p), "tex") {
			return
		}
		b, err := textureThumb(p, 1400)
		if err != nil {
			w.Header().Del("ETag")
			http.Error(w, err.Error(), 404)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Write(b)
	})
	api("/api/split", func(w http.ResponseWriter, r *http.Request) {
		var q splitReq
		if err := readJSON(r, &q); err != nil {
			fail(w, 400, err)
			return
		}
		s := getSettings()
		startJob(w, "Split pictures", nil, func(j *Job) (any, error) { return doSplit(j, s, q) })
	})
	api("/api/vehicles", func(w http.ResponseWriter, r *http.Request) {
		v := loadVehicles(getSettings())
		if v == nil {
			v = []Vehicle{}
		}
		writeJSON(w, 200, map[string]any{"vehicles": v})
	})
	api("/api/vehicles/find", func(w http.ResponseWriter, r *http.Request) {
		s := getSettings()
		startJob(w, "Find aircraft", []string{s.AESKey}, func(j *Job) (any, error) { return doFindVehicles(j, s) })
	})
	api("/api/preview", func(w http.ResponseWriter, r *http.Request) {
		var q struct{ Vehicle string }
		if err := readJSON(r, &q); err != nil {
			fail(w, 400, err)
			return
		}
		s := getSettings()
		startJob(w, "Preview skins", []string{s.AESKey}, func(j *Job) (any, error) { return doPreview(j, s, q.Vehicle) })
	})
	api("/api/previews", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, loadPreviews(getSettings(), r.URL.Query().Get("vehicle")))
	})
	api("/api/previews/clear", func(w http.ResponseWriter, r *http.Request) {
		var q struct{ Vehicle string }
		if err := readJSON(r, &q); err != nil {
			fail(w, 400, err)
			return
		}
		s := getSettings()
		if v, ok := findVehicle(s, q.Vehicle); ok && safeName(v.Name) != "" && !jobs.Running() {
			os.RemoveAll(previewDirFor(s, v))
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
	api("/api/previewimg", func(w http.ResponseWriter, r *http.Request) {
		s := getSettings()
		v, ok := findVehicle(s, r.URL.Query().Get("vehicle"))
		skin := safeName(r.URL.Query().Get("skin"))
		if !ok || skin == "" {
			http.NotFound(w, r)
			return
		}
		b, err := os.ReadFile(filepath.Join(previewDirFor(s, v), skin+".png"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(b)
	})
	api("/api/extract", func(w http.ResponseWriter, r *http.Request) {
		var q extractReq
		if err := readJSON(r, &q); err != nil {
			fail(w, 400, err)
			return
		}
		s := getSettings()
		startJob(w, "Extract", []string{s.AESKey}, func(j *Job) (any, error) { return doExtract(j, s, q) })
	})
	api("/api/library", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"skins": cachedLibrary(r.URL.Query().Get("fresh") == "1")})
	})
	api("/api/model/inspect", func(w http.ResponseWriter, r *http.Request) {
		var q struct{ Paths []string }
		if err := readJSON(r, &q); err != nil {
			fail(w, 400, err)
			return
		}
		mats, err := InspectModels(q.Paths)
		if err != nil {
			fail(w, 400, err)
			return
		}
		if mats == nil {
			mats = []ModelMaterial{}
		}
		writeJSON(w, 200, map[string]any{"materials": mats})
	})
	api("/api/model/build", func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			Paths     []string
			Keep      []string
			Name      string
			MoveLoose bool
			Cleanup   []string
		}
		if err := readJSON(r, &q); err != nil {
			fail(w, 400, err)
			return
		}
		name := safeName(q.Name)
		if name == "" {
			fail(w, 400, errors.New("give the model a name using letters, numbers, dashes or underscores"))
			return
		}
		keep := map[string]bool{}
		for _, k := range q.Keep {
			keep[k] = true
		}
		s := getSettings()
		out := filepath.Join(s.modelsDir(), name+".glb")
		tiles, notes, err := BuildPaintModel(q.Paths, keep, q.MoveLoose, out)
		if err != nil {
			fail(w, 400, err)
			return
		}
		cleanInbox(s, q.Cleanup)
		if notes == nil {
			notes = []string{}
		}
		writeJSON(w, 200, map[string]any{"path": out, "dir": s.modelsDir(), "tiles": tiles, "notes": notes})
	})
	api("/api/export", func(w http.ResponseWriter, r *http.Request) {
		var q exportReq
		if err := readJSON(r, &q); err != nil {
			fail(w, 400, err)
			return
		}
		s := getSettings()
		startJob(w, "Make tiles", nil, func(j *Job) (any, error) { return doExport(j, s, q) })
	})
	api("/api/plan", func(w http.ResponseWriter, r *http.Request) {
		var q planReq
		if err := readJSON(r, &q); err != nil {
			fail(w, 400, err)
			return
		}
		rows, n, err := doPlan(q)
		if err != nil {
			fail(w, 400, err)
			return
		}
		writeJSON(w, 200, map[string]any{"rows": rows, "pictures": n})
	})
	// which skin the pictures were painted on, worked out from their names
	api("/api/guessskin", func(w http.ResponseWriter, r *http.Request) {
		var q planReq
		if err := readJSON(r, &q); err != nil {
			fail(w, 400, err)
			return
		}
		files, err := paintedFiles(q)
		if err != nil {
			fail(w, 400, err)
			return
		}
		g, ok := guessSkin(files, cachedLibrary(false))
		writeJSON(w, 200, map[string]any{"found": ok, "dir": g.Dir, "label": g.Label, "matched": g.Matched, "total": g.Total})
	})
	api("/api/build", func(w http.ResponseWriter, r *http.Request) {
		var q planReq
		if err := readJSON(r, &q); err != nil {
			fail(w, 400, err)
			return
		}
		s := getSettings()
		startJob(w, "Build mod", []string{s.AESKey}, func(j *Job) (any, error) { return doBuild(j, s, q) })
	})
	// ---- pilots: helmets and radio portraits
	api("/api/pilots", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, loadPilotIndex(getSettings())) })
	api("/api/pilots/find", func(w http.ResponseWriter, r *http.Request) {
		s := getSettings()
		startJob(w, "Find pilots", []string{s.AESKey}, func(j *Job) (any, error) { return doFindPilots(j, s) })
	})
	api("/api/pilots/img", func(w http.ResponseWriter, r *http.Request) {
		s := getSettings()
		name := r.URL.Query().Get("name")
		var b []byte
		var err error
		switch r.URL.Query().Get("kind") {
		case "helmet":
			if !reGameName.MatchString(name) {
				err = errors.New("bad name")
			} else {
				b, err = os.ReadFile(filepath.Join(s.pilotsDir(), "previews", name+".png"))
			}
		case "portrait":
			b, err = portraitPNG(s, name)
		default:
			err = errors.New("unknown kind")
		}
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "private, max-age=600")
		w.Write(b)
	})
	api("/api/pilots/font", func(w http.ResponseWriter, r *http.Request) {
		b, err := gameFontOTF(getSettings())
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "font/otf")
		w.Header().Set("Cache-Control", "private, max-age=3600")
		w.Write(b)
	})
	api("/api/pilots/paint", func(w http.ResponseWriter, r *http.Request) {
		var q pilotPaintReq
		if err := readJSON(r, &q); err != nil {
			fail(w, 400, err)
			return
		}
		s := getSettings()
		startJob(w, "Helmet pictures", []string{s.AESKey}, func(j *Job) (any, error) { return doPilotPaint(j, s, q) })
	})
	api("/api/pilots/plan", func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			Pilot string      `json:"pilot"`
			Files []pilotFile `json:"files"`
		}
		if err := readJSON(r, &q); err != nil {
			fail(w, 400, err)
			return
		}
		writeJSON(w, 200, planPilot(loadPilotIndex(getSettings()).pilot(q.Pilot), q.Files))
	})
	api("/api/pilots/listdir", func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			Dir string `json:"dir"`
		}
		if err := readJSON(r, &q); err != nil {
			fail(w, 400, err)
			return
		}
		files, err := listImages(cleanPath(q.Dir))
		if err != nil {
			fail(w, 400, fmt.Errorf("cannot read %s", q.Dir))
			return
		}
		if files == nil {
			files = []string{}
		}
		writeJSON(w, 200, map[string]any{"files": files})
	})
	api("/api/pilots/build", func(w http.ResponseWriter, r *http.Request) {
		var q pilotBuildReq
		if err := readJSON(r, &q); err != nil {
			fail(w, 400, err)
			return
		}
		s := getSettings()
		startJob(w, "Build pilot mod", []string{s.AESKey}, func(j *Job) (any, error) { return doPilotBuild(j, s, q) })
	})
	// new radio portraits kept between runs (pilotsaved.go)
	api("/api/pilots/portraits", func(w http.ResponseWriter, r *http.Request) {
		s := getSettings()
		writeJSON(w, 200, map[string]any{"saved": listSavedPortraits(s), "inMods": portraitsInMods(s)})
	})
	api("/api/pilots/portraits/save", func(w http.ResponseWriter, r *http.Request) {
		var q savePortraitReq
		if err := readJSON(r, &q); err != nil {
			fail(w, 400, err)
			return
		}
		sp, err := savePortrait(getSettings(), q)
		if err != nil {
			fail(w, 400, err)
			return
		}
		writeJSON(w, 200, sp)
	})
	api("/api/pilots/portraits/remove", func(w http.ResponseWriter, r *http.Request) {
		var q struct{ Name string }
		if err := readJSON(r, &q); err != nil {
			fail(w, 400, err)
			return
		}
		if err := removeSavedPortrait(getSettings(), q.Name); err != nil && !os.IsNotExist(err) {
			fail(w, 400, err)
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
	api("/api/pilots/portraits/recover", func(w http.ResponseWriter, r *http.Request) {
		var q struct{ Mod string }
		if err := readJSON(r, &q); err != nil {
			fail(w, 400, err)
			return
		}
		got, err := recoverPortraits(getSettings(), q.Mod)
		if err != nil {
			fail(w, 400, err)
			return
		}
		writeJSON(w, 200, map[string]any{"names": got})
	})
	api("/api/report", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"text": buildReport()})
	})
	api("/api/mods", func(w http.ResponseWriter, r *http.Request) {
		s := getSettings()
		c := s.checks()["mods"]
		mods := []InstalledMod{}
		if c.OK {
			if found := scanMods(s.ModsDir); found != nil {
				mods = found
			}
		}
		// what each mod is as far as skin slots go; the merged skin list standing in for other
		// mods' lists is how it is meant to work, so that is not shown as a conflict
		for i := range mods {
			mods[i].Kind = modKind(mods[i])
		}
		for i := range mods {
			var keep []ModConflict
			for _, cf := range mods[i].Conflicts {
				if onlyLists(cf.Files) && !cf.Same && (mods[i].Name == skinListMod || cf.With == skinListMod) {
					continue
				}
				keep = append(keep, cf)
			}
			if keep == nil {
				keep = []ModConflict{}
			}
			mods[i].Conflicts = keep
		}
		_, err := os.Stat(s.removedModsDir())
		writeJSON(w, 200, map[string]any{"mods": mods, "dir": s.ModsDir, "ok": c.OK, "note": c.Note, "removedDir": s.removedModsDir(), "haveRemoved": err == nil})
	})
	api("/api/mods/remove", func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			Name  string
			Loose bool
		}
		if err := readJSON(r, &q); err != nil || r.Method != http.MethodPost {
			fail(w, 400, errors.New("bad request"))
			return
		}
		if jobs.Running() {
			fail(w, http.StatusConflict, errBusy)
			return
		}
		dest, err := removeMod(getSettings(), q.Name, q.Loose)
		if err != nil {
			fail(w, 400, err)
			return
		}
		logf("mod %q moved to %s", q.Name, dest)
		writeJSON(w, 200, map[string]string{"movedTo": dest})
	})
	slotRoutes(api)
	linkRoutes(api)
	saveRoutes(api)
	skinReportRoutes(api)
	pilotNameRoutes(api)
	api("/api/job", func(w http.ResponseWriter, r *http.Request) {
		j := jobs.Get(r.URL.Query().Get("id"))
		if j == nil {
			fail(w, 404, errors.New("no such task"))
			return
		}
		if r.URL.Query().Get("cancel") == "1" {
			j.cancel()
		}
		since, _ := strconv.Atoi(r.URL.Query().Get("since"))
		writeJSON(w, 200, j.view(since))
	})
	return mux
}
