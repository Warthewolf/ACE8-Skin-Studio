//go:build !windows

package main

// Linux (and other non-Windows systems): the window is a browser app window when Chrome, Chromium
// or Edge is installed, otherwise the default browser; files and folders are chosen with zenity or
// kdialog when one is installed; the game runs under Proton, so its saves are inside Steam's
// compatdata folder.

import (
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

func hideWindow(cmd *exec.Cmd) {}

// sameUserPeer tells whether a request comes from a program of the user running this one (or of
// the administrator, who can reach everything anyway). Any user can connect to 127.0.0.1; on Linux
// /proc/net/tcp says who owns the other end of a connection. When that cannot be told, the answer
// is yes.
func sameUserPeer(r *http.Request) bool {
	local, _ := r.Context().Value(http.LocalAddrContextKey).(net.Addr)
	if local == nil {
		return true
	}
	uid, ok := socketOwner(r.RemoteAddr, local.String())
	return !ok || uid == os.Getuid() || uid == 0
}

// socketOwner finds the user who owns the far end of a connection to this computer: the socket
// whose own address is remote and whose far end is local. ok is false when it cannot be told.
func socketOwner(remote, local string) (uid int, ok bool) {
	rip, rport, err1 := splitAddr(remote)
	lip, lport, err2 := splitAddr(local)
	if err1 != nil || err2 != nil {
		return 0, false
	}
	for _, f := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		// sl local_address rem_address st tx:rx tr:when retrnsmt uid ...
		for _, line := range strings.Split(string(b), "\n") {
			fs := strings.Fields(line)
			if len(fs) < 8 || fs[3] == "06" { // 06: a connection already closed (its owner is not kept)
				continue
			}
			if procAddrIs(fs[1], rip, rport) && procAddrIs(fs[2], lip, lport) {
				if u, err := strconv.Atoi(fs[7]); err == nil {
					return u, true
				}
			}
		}
	}
	return 0, false
}

func splitAddr(a string) (net.IP, int, error) {
	h, p, err := net.SplitHostPort(a)
	if err != nil {
		return nil, 0, err
	}
	ip := net.ParseIP(h)
	if ip == nil {
		return nil, 0, errors.New("not an address")
	}
	port, err := strconv.Atoi(p)
	return ip, port, err
}

// procAddrIs tells whether an address as /proc/net/tcp writes it is ip:port. It is in hexadecimal,
// each group of 4 bytes as a number in this computer's own byte order, then ":" and the port.
func procAddrIs(s string, ip net.IP, port int) bool {
	h, p, found := strings.Cut(s, ":")
	if !found {
		return false
	}
	if n, err := strconv.ParseUint(p, 16, 16); err != nil || int(n) != port {
		return false
	}
	raw, err := hex.DecodeString(h)
	if err != nil || (len(raw) != 4 && len(raw) != 16) {
		return false
	}
	for i := 0; i < len(raw); i += 4 {
		binary.NativeEndian.PutUint32(raw[i:], binary.BigEndian.Uint32(raw[i:]))
	}
	return net.IP(raw).Equal(ip)
}

// openURL shows the program's window: an app window of a Chromium browser when one is installed,
// otherwise the default browser.
func openURL(url string, appWindow bool) error {
	if appWindow {
		for _, b := range []string{"microsoft-edge", "microsoft-edge-stable", "google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "brave-browser"} {
			if p, err := exec.LookPath(b); err == nil {
				cmd := exec.Command(p, "--app="+url, "--window-size=1280,900")
				if err := cmd.Start(); err == nil {
					go cmd.Wait()
					return nil
				}
			}
		}
	}
	for _, c := range []string{"xdg-open", "open"} {
		if p, err := exec.LookPath(c); err == nil {
			cmd := exec.Command(p, url)
			if err := cmd.Start(); err == nil {
				go cmd.Wait()
				return nil
			}
		}
	}
	return errors.New("no browser opener found")
}

func revealPath(p string) error {
	if st, err := os.Stat(p); err == nil && !st.IsDir() {
		p = filepath.Dir(p)
	}
	for _, c := range []string{"xdg-open", "open"} {
		if x, err := exec.LookPath(c); err == nil {
			cmd := exec.Command(x, p)
			if err := cmd.Start(); err == nil {
				go cmd.Wait()
				return nil
			}
		}
	}
	return errors.New("cannot open folders on this system")
}

// ------------------------------------------------------------------ choosing files

func chooserTool() string {
	for _, t := range []string{"zenity", "kdialog"} {
		if p, err := exec.LookPath(t); err == nil {
			return p
		}
	}
	return ""
}

// canPick: a file chooser is offered when zenity or kdialog is installed.
var canPick = chooserTool() != ""

var pickMu sync.Mutex

var pickPatterns = map[string][2]string{
	"images":   {"Images", "*.png *.tga *.PNG *.TGA"},
	"exe":      {"retoc", "retoc*"},
	"usmap":    {"Mappings", "*.usmap"},
	"raw":      {"Raw textures", "*.uasset *.uexp *.ubulk"},
	"glb":      {"glTF models", "*.glb"},
	"zip":      {"Skin mods", "*.zip"},
	"pictures": {"Pictures", "*.png *.jpg *.jpeg *.tga *.PNG *.JPG *.JPEG *.TGA"},
	"json":     {"Settings", "settings.json"},
}

// pickDialog shows a file or folder chooser and returns what was chosen (empty if cancelled).
func pickDialog(kind, title, filter, start string) ([]string, error) {
	tool := chooserTool()
	if tool == "" {
		return nil, errors.New("no file chooser was found (install zenity or kdialog); type or paste the path instead")
	}
	if !pickMu.TryLock() {
		return nil, errors.New("a file chooser is already open")
	}
	defer pickMu.Unlock()
	pat, hasPat := pickPatterns[filter]
	var args []string
	if filepath.Base(tool) == "zenity" {
		args = []string{"--file-selection", "--title=" + title}
		switch kind {
		case "folder":
			args = append(args, "--directory")
		case "files":
			args = append(args, "--multiple", "--separator=\n")
		}
		if hasPat && kind != "folder" {
			args = append(args, "--file-filter="+pat[0]+" | "+pat[1], "--file-filter=All files | *")
		}
		if start != "" {
			args = append(args, "--filename="+strings.TrimSuffix(start, "/")+"/")
		}
	} else {
		if start == "" {
			start, _ = os.UserHomeDir()
		}
		switch kind {
		case "folder":
			args = []string{"--getexistingdirectory", start}
		case "files":
			args = []string{"--getopenfilename", start, "", "--multiple", "--separate-output"}
		default:
			args = []string{"--getopenfilename", start, ""}
		}
		if hasPat && kind != "folder" {
			args[2] = pat[1] + "|" + pat[0]
		}
		args = append(args, "--title", title)
	}
	out, err := exec.Command(tool, args...).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 1 {
			return nil, nil // cancelled
		}
		return nil, err
	}
	var res []string
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			res = append(res, line)
		}
	}
	return res, nil
}

const pickHelperArg = "pick-dialog"

func runPickHelper() int { return 3 }

const exeName = "retoc"

// ------------------------------------------------------------------ the game under Proton

// findRunningGame names the game's program when it is running under Proton ("" when not).
func findRunningGame() string {
	es, err := os.ReadDir("/proc")
	if err != nil {
		return ""
	}
	for _, e := range es {
		if e.Name()[0] < '0' || e.Name()[0] > '9' {
			continue
		}
		b, err := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline"))
		if err != nil {
			continue
		}
		for _, arg := range strings.Split(string(b), "\x00") {
			name := filepath.Base(strings.ReplaceAll(arg, `\`, "/"))
			if isGameExe(name) {
				return name
			}
		}
	}
	return ""
}

// steamLibraries lists the Steam library folders on this computer.
func steamLibraries() []string {
	home, _ := os.UserHomeDir()
	var roots []string
	for _, r := range []string{".steam/steam", ".local/share/Steam", ".var/app/com.valvesoftware.Steam/.local/share/Steam"} {
		roots = append(roots, filepath.Join(home, r))
	}
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		if rp, err := filepath.EvalSymlinks(p); err == nil {
			p = rp
		}
		if !seen[p] {
			if st, err := os.Stat(filepath.Join(p, "steamapps")); err == nil && st.IsDir() {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	rePath := regexp.MustCompile(`^\s*"path"\s*"(.*)"\s*$`)
	for _, r := range roots {
		add(r)
		f, err := os.Open(filepath.Join(r, "steamapps", "libraryfolders.vdf"))
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			if m := rePath.FindStringSubmatch(sc.Text()); m != nil {
				add(strings.ReplaceAll(m[1], `\\`, `\`))
			}
		}
		f.Close()
	}
	return out
}

// platformSaveGamesDir finds the game's SaveGames folder inside Proton's prefix.
func platformSaveGamesDir() string {
	rel := filepath.Join("pfx", "drive_c", "users", "steamuser", "AppData", "Local", "BANDAI NAMCO Entertainment", "ACE COMBAT 8", "Saved", "SaveGames")
	libs := steamLibraries()
	// the prefix that has the game's saves
	for _, l := range libs {
		if ms, _ := filepath.Glob(filepath.Join(l, "steamapps", "compatdata", "*", rel)); len(ms) > 0 {
			return ms[0]
		}
	}
	return ""
}

// platformPaksDir finds the game's Paks folder in the Steam libraries.
func platformPaksDir() string {
	for _, l := range steamLibraries() {
		p := filepath.Join(l, "steamapps", "common", "ACE COMBAT 8", "Game", "Content", "Paks")
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			return p
		}
	}
	return ""
}

// Oodle for retoc (see oodle.go). The game's own copy is for Windows, so it cannot stand in here.
const (
	oodleName         = "liboo2corelinux64.so.9"
	oodleGameName     = ""
	oodlePlatformPath = "linux/lib/liboo2corelinux64.so.9"
	oodleSHA256       = "ed7e98f70be1254a80644efd3ae442ff61f854a2fe9debb0b978b95289884e9c"
)
