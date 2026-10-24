package main

// The program's window is a local web page served on 127.0.0.1 and shown in an app window.

import (
	"bytes"
	"crypto/rand"
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

const preferredPort = 48189

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
	logFile, _ = os.Create(filepath.Join(dataDir, "ACE8SkinStudio.log"))
	loadSettings()
	logf("%s %s starting on %s", appName, appVersion, runtime.GOOS)

	// If the program is already running, bring its window up instead of starting a second copy.
	if *port == 0 {
		if resp, err := (&http.Client{Timeout: 700 * time.Millisecond}).Post(fmt.Sprintf("http://127.0.0.1:%d/api/show", preferredPort), "text/plain", strings.NewReader(dataDir)); err == nil {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode == 200 && strings.TrimSpace(string(b)) == "ok" {
				return 0
			}
		}
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
		logf("cannot listen: %v", err)
		fmt.Fprintln(os.Stderr, "cannot start:", err)
		return 1
	}
	baseURL = fmt.Sprintf("http://127.0.0.1:%d", ln.Addr().(*net.TCPAddr).Port)
	logf("listening on %s", baseURL)
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

func showWindow() error {
	return openURL(baseURL+"/?t="+token, !getSettings().DefaultBrowser)
}

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
		host := r.Host
		if !strings.HasPrefix(host, "127.0.0.1:") && !strings.HasPrefix(host, "localhost:") {
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
	// Another copy of the program asks this one to show its window (no token: it only opens our own page).
	mux.HandleFunc("/api/show", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasPrefix(r.Host, "127.0.0.1:") || r.Header.Get("Origin") != "" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		presence.heard("")
		showWindow()
		io.WriteString(w, "ok")
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
		writeJSON(w, 200, map[string]bool{"ok": true})
		go func() { time.Sleep(300 * time.Millisecond); logf("quit requested"); os.Exit(0) }()
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
		b, err := makeThumb(r.URL.Query().Get("path"), maxW, r.URL.Query().Get("alpha") == "1")
		if err != nil {
			http.Error(w, err.Error(), 404)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "private, max-age=600")
		w.Write(b)
	})
	api("/api/texthumb", func(w http.ResponseWriter, r *http.Request) {
		b, err := textureThumb(r.URL.Query().Get("uexp"), 1400)
		if err != nil {
			http.Error(w, err.Error(), 404)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "private, max-age=600")
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
	saveRoutes(api)
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
