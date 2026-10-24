//go:build windows

package main

// The file and folder chooser.
//
// The chooser is shown by a short-lived copy of this program (started with pickHelperArg) so
// that a fault inside the Windows dialog code can never take the main program down. The helper
// makes the Skin Studio window the owner of the dialog, which is what puts the dialog in front
// of that window and gives it the keyboard. A small watcher then checks that the dialog really
// did come to the front and pulls it forward if it did not.

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

const pickHelperArg = "pick-dialog"

var (
	modUser32   = syscall.NewLazyDLL("user32.dll")
	modOle32    = syscall.NewLazyDLL("ole32.dll")
	modShell32  = syscall.NewLazyDLL("shell32.dll")
	modKernel32 = syscall.NewLazyDLL("kernel32.dll")

	procGetForegroundWindow           = modUser32.NewProc("GetForegroundWindow")
	procSetForegroundWindow           = modUser32.NewProc("SetForegroundWindow")
	procBringWindowToTop              = modUser32.NewProc("BringWindowToTop")
	procGetWindowTextW                = modUser32.NewProc("GetWindowTextW")
	procGetClassNameW                 = modUser32.NewProc("GetClassNameW")
	procFindWindowExW                 = modUser32.NewProc("FindWindowExW")
	procIsWindow                      = modUser32.NewProc("IsWindow")
	procIsWindowVisible               = modUser32.NewProc("IsWindowVisible")
	procEnableWindow                  = modUser32.NewProc("EnableWindow")
	procGetWindowThreadProcessId      = modUser32.NewProc("GetWindowThreadProcessId")
	procAttachThreadInput             = modUser32.NewProc("AttachThreadInput")
	procSetWindowPos                  = modUser32.NewProc("SetWindowPos")
	procPeekMessageW                  = modUser32.NewProc("PeekMessageW")
	procSetProcessDPIAware            = modUser32.NewProc("SetProcessDPIAware")
	procSetProcessDpiAwarenessContext = modUser32.NewProc("SetProcessDpiAwarenessContext")

	procGetCurrentThreadId = modKernel32.NewProc("GetCurrentThreadId")

	procCoInitializeEx   = modOle32.NewProc("CoInitializeEx")
	procCoUninitialize   = modOle32.NewProc("CoUninitialize")
	procCoCreateInstance = modOle32.NewProc("CoCreateInstance")
	procCoTaskMemFree    = modOle32.NewProc("CoTaskMemFree")

	procSHCreateItemFromParsingName = modShell32.NewProc("SHCreateItemFromParsingName")
)

// ---------------------------------------------------------------------------------------------
// Main program side
// ---------------------------------------------------------------------------------------------

var pickMu sync.Mutex

func windowText(h uintptr) string {
	buf := make([]uint16, 512)
	n, _, _ := procGetWindowTextW.Call(h, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if int(n) > len(buf) {
		n = uintptr(len(buf))
	}
	return syscall.UTF16ToString(buf[:n])
}

func windowClass(h uintptr) string {
	buf := make([]uint16, 256)
	n, _, _ := procGetClassNameW.Call(h, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if int(n) > len(buf) {
		n = uintptr(len(buf))
	}
	return syscall.UTF16ToString(buf[:n])
}

// isStudioWindow reports whether a window is the browser window that shows this program.
func isStudioWindow(h uintptr) bool {
	if h == 0 {
		return false
	}
	if v, _, _ := procIsWindowVisible.Call(h); v == 0 {
		return false
	}
	switch windowClass(h) {
	case "Chrome_WidgetWin_1", "MozillaWindowClass": // Edge, Chrome and relatives; Firefox
	default:
		return false
	}
	return strings.Contains(windowText(h), appName)
}

// findStudioWindow returns the window the chooser should belong to, or 0 when it cannot tell.
func findStudioWindow() uintptr {
	if fg, _, _ := procGetForegroundWindow.Call(); isStudioWindow(fg) {
		return fg
	}
	var h uintptr
	for i := 0; i < 4000; i++ {
		h, _, _ = procFindWindowExW.Call(0, h, 0, 0)
		if h == 0 {
			break
		}
		if isStudioWindow(h) {
			return h
		}
	}
	return 0
}

func parsePicked(out string) []string {
	var res []string
	// Each chosen path comes back as "P:" plus base64 text, so nothing on the way can garble it.
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r", ""), "\n") {
		line = strings.TrimSpace(line)
		if i := strings.Index(line, "P:"); i >= 0 {
			if b, err := base64.StdEncoding.DecodeString(line[i+2:]); err == nil && len(b) > 0 {
				res = append(res, string(b))
			}
		}
	}
	return res
}

func pickFilter(filter string) string {
	filters := map[string]string{
		"images":   "Images (*.png;*.tga)|*.png;*.tga|All files (*.*)|*.*",
		"exe":      "Programs (*.exe)|*.exe|All files (*.*)|*.*",
		"usmap":    "Mappings (*.usmap)|*.usmap|All files (*.*)|*.*",
		"raw":      "Raw textures (*.uasset;*.uexp;*.ubulk)|*.uasset;*.uexp;*.ubulk|All files (*.*)|*.*",
		"glb":      "glTF models (*.glb)|*.glb|All files (*.*)|*.*",
		"zip":      "Skin mods (*.zip)|*.zip|All files (*.*)|*.*",
		"pictures": "Pictures (*.png;*.jpg;*.jpeg;*.tga)|*.png;*.jpg;*.jpeg;*.tga|All files (*.*)|*.*",
		"json":     "Settings (settings.json)|settings.json|All files (*.*)|*.*",
	}
	if f := filters[filter]; f != "" {
		return f
	}
	return "All files (*.*)|*.*"
}

// pickDialog shows a file or folder chooser and returns what was chosen (empty if cancelled).
func pickDialog(kind, title, filter, start string) ([]string, error) {
	if !pickMu.TryLock() {
		return nil, errors.New("a file chooser is already open; look for it on the taskbar")
	}
	defer pickMu.Unlock()
	f := pickFilter(filter)
	owner := findStudioWindow()
	res, err := pickNative(kind, title, f, start, owner)
	if err == nil {
		return res, nil
	}
	logf("built-in chooser failed (%v); trying the PowerShell one", err)
	return pickPowerShell(kind, title, f, start)
}

func pickNative(kind, title, filter, start string, owner uintptr) ([]string, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(exe, pickHelperArg)
	cmd.Env = append(os.Environ(),
		"AC8_KIND="+kind, "AC8_TITLE="+title, "AC8_FILTER="+filter, "AC8_START="+start,
		"AC8_OWNER="+strconv.FormatUint(uint64(owner), 10))
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err = cmd.Run()
	if owner != 0 {
		// A chooser switches its owner window off while it is open. Should the helper ever stop
		// half-way, this makes sure the Skin Studio window answers to the mouse again.
		if ok, _, _ := procIsWindow.Call(owner); ok != 0 {
			procEnableWindow.Call(owner, 1)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("%v %s", err, strings.TrimSpace(errb.String()))
	}
	return parsePicked(out.String()), nil
}

// ---------------------------------------------------------------------------------------------
// Fallback: the same chooser through PowerShell, used only if the built-in one fails to start.
// ---------------------------------------------------------------------------------------------

const psOwner = `
Add-Type -AssemblyName System.Windows.Forms
[System.Windows.Forms.Application]::EnableVisualStyles()
$owner = New-Object System.Windows.Forms.Form -Property @{TopMost = $true; ShowInTaskbar = $false; FormBorderStyle = 'None'; StartPosition = 'CenterScreen'; Width = 1; Height = 1; Opacity = 0}
$owner.Show()
$owner.Activate()
`

const psPickFiles = psOwner + `
$d = New-Object System.Windows.Forms.OpenFileDialog
$d.Title = $env:AC8_TITLE
$d.Filter = $env:AC8_FILTER
$d.Multiselect = ($env:AC8_MULTI -eq '1')
if ($env:AC8_START -and (Test-Path -LiteralPath $env:AC8_START)) { $d.InitialDirectory = $env:AC8_START }
if ($d.ShowDialog($owner) -eq [System.Windows.Forms.DialogResult]::OK) { foreach ($f in $d.FileNames) { [Console]::Out.WriteLine('P:' + [Convert]::ToBase64String([System.Text.Encoding]::UTF8.GetBytes($f))) } }
$owner.Dispose()
`

const psPickFolder = psOwner + `
$d = New-Object System.Windows.Forms.FolderBrowserDialog
$d.Description = $env:AC8_TITLE
$d.ShowNewFolderButton = $true
if ($env:AC8_START -and (Test-Path -LiteralPath $env:AC8_START)) { $d.SelectedPath = $env:AC8_START }
if ($d.ShowDialog($owner) -eq [System.Windows.Forms.DialogResult]::OK) { [Console]::Out.WriteLine('P:' + [Convert]::ToBase64String([System.Text.Encoding]::UTF8.GetBytes($d.SelectedPath))) }
$owner.Dispose()
`

func psEncode(script string) string {
	u := utf16.Encode([]rune(script))
	b := make([]byte, len(u)*2)
	for i, v := range u {
		b[i*2], b[i*2+1] = byte(v), byte(v>>8)
	}
	return base64.StdEncoding.EncodeToString(b)
}

func pickPowerShell(kind, title, filter, start string) ([]string, error) {
	script := psPickFiles
	if kind == "folder" {
		script = psPickFolder
	}
	multi := "0"
	if kind == "files" {
		multi = "1"
	}
	cmd := exec.Command("powershell.exe", "-NoProfile", "-STA", "-ExecutionPolicy", "Bypass", "-EncodedCommand", psEncode(script))
	cmd.Env = append(os.Environ(), "AC8_TITLE="+title, "AC8_FILTER="+filter, "AC8_MULTI="+multi, "AC8_START="+start)
	// Hidden, but with a console of its own: PowerShell needs one to write its answer.
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return nil, errors.New("could not open the file chooser; type or paste the path instead")
	}
	return parsePicked(out.String()), nil
}

// ---------------------------------------------------------------------------------------------
// Helper side: everything below runs only in the short-lived helper copy.
// ---------------------------------------------------------------------------------------------

type guid struct {
	d1     uint32
	d2, d3 uint16
	d4     [8]byte
}

type filterSpec struct{ name, spec *uint16 }

var (
	clsidFileOpenDialog = guid{0xDC1C5A9C, 0xE88A, 0x4DDE, [8]byte{0xA5, 0xA1, 0x60, 0xF8, 0x2A, 0x20, 0xAE, 0xF7}}
	iidIFileOpenDialog  = guid{0xD57C7288, 0xD4AD, 0x4768, [8]byte{0xBE, 0x02, 0x9D, 0x96, 0x95, 0x32, 0xD9, 0x60}}
	iidIShellItem       = guid{0x43826D1E, 0xE718, 0x42EE, [8]byte{0xBC, 0x55, 0xA1, 0xE2, 0x61, 0xC3, 0x7B, 0xFE}}
)

// Everything Windows writes into lives here, at package level, so its address never changes.
var pk struct {
	dlg, folder, item, arr uintptr
	name                   uintptr
	opts, count            uint32
	specs                  []filterSpec
	strs                   []*uint16
	msg                    [8]uint64
}

// Positions of the functions used, counted from the start of each interface.
const (
	vRelease = 2
	// IFileOpenDialog
	vShow         = 3
	vSetFileTypes = 4
	vSetOptions   = 9
	vGetOptions   = 10
	vSetFolder    = 12
	vSetTitle     = 17
	vGetResult    = 20
	vGetResults   = 27
	// IShellItem
	vGetDisplayName = 5
	// IShellItemArray
	vGetCount  = 7
	vGetItemAt = 8
)

const (
	fosNoChangeDir     = 0x8
	fosPickFolders     = 0x20
	fosForceFileSystem = 0x40
	fosAllowMulti      = 0x200
	fosPathMustExist   = 0x800
	fosFileMustExist   = 0x1000
	sigdnFileSysPath   = 0x80058000
	hrCancelled        = 0x800704C7
)

func failed(hr uintptr) bool { return int32(uint32(hr)) < 0 }

// vcall calls function number idx of a COM object.
//
//go:uintptrescapes
func vcall(obj uintptr, idx uintptr, args ...uintptr) uintptr {
	vtbl := *(*uintptr)(unsafe.Pointer(obj))
	fn := *(*uintptr)(unsafe.Pointer(vtbl + idx*unsafe.Sizeof(uintptr(0))))
	all := make([]uintptr, 0, len(args)+1)
	all = append(all, obj)
	all = append(all, args...)
	r, _, _ := syscall.SyscallN(fn, all...)
	return r
}

func release(obj uintptr) {
	if obj != 0 {
		vcall(obj, vRelease)
	}
}

// keepStr turns text into the form Windows wants and keeps it alive until the helper ends.
func keepStr(s string) *uint16 {
	p, err := syscall.UTF16PtrFromString(strings.ReplaceAll(s, "\x00", ""))
	if err != nil {
		p, _ = syscall.UTF16PtrFromString("")
	}
	pk.strs = append(pk.strs, p)
	return p
}

func utf16At(p uintptr) string {
	if p == 0 {
		return ""
	}
	var u []uint16
	for i := uintptr(0); i < 33000; i++ {
		c := *(*uint16)(unsafe.Pointer(p + i*2))
		if c == 0 {
			break
		}
		u = append(u, c)
	}
	return string(utf16.Decode(u))
}

// itemPath reads the file system path of a shell item.
func itemPath(item uintptr) string {
	pk.name = 0
	if hr := vcall(item, vGetDisplayName, uintptr(sigdnFileSysPath), uintptr(unsafe.Pointer(&pk.name))); failed(hr) || pk.name == 0 {
		return ""
	}
	s := utf16At(pk.name)
	procCoTaskMemFree.Call(pk.name)
	pk.name = 0
	return s
}

// startFolder picks the folder the chooser opens in: the given folder, the folder of the given
// file, or the nearest folder above it that exists.
func startFolder(start string) string {
	p := strings.TrimSpace(start)
	if p == "" {
		return ""
	}
	p = filepath.Clean(p)
	if !filepath.IsAbs(p) {
		return ""
	}
	for i := 0; i < 6; i++ {
		if st, err := os.Stat(p); err == nil {
			if st.IsDir() {
				return p
			}
			return filepath.Dir(p)
		}
		parent := filepath.Dir(p)
		if parent == p {
			break
		}
		p = parent
	}
	return ""
}

func setDPIAware() {
	// Without this the dialog is drawn small and then stretched, which looks blurry.
	if procSetProcessDpiAwarenessContext.Find() == nil {
		const perMonitorV2 = ^uintptr(3) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 is -4
		if r, _, _ := procSetProcessDpiAwarenessContext.Call(perMonitorV2); r != 0 {
			return
		}
	}
	if procSetProcessDPIAware.Find() == nil {
		procSetProcessDPIAware.Call()
	}
}

var dialogClass, _ = syscall.UTF16PtrFromString("#32770") // the window class of standard dialogs

// findOwnDialog returns the visible dialog window that belongs to this helper, or 0.
func findOwnDialog() uintptr {
	me := uint32(os.Getpid())
	var h uintptr
	for i := 0; i < 4000; i++ {
		h, _, _ = procFindWindowExW.Call(0, h, uintptr(unsafe.Pointer(dialogClass)), 0)
		if h == 0 {
			return 0
		}
		var pid uint32
		procGetWindowThreadProcessId.Call(h, uintptr(unsafe.Pointer(&pid)))
		if pid != me {
			continue
		}
		if v, _, _ := procIsWindowVisible.Call(h); v != 0 {
			return h
		}
	}
	return 0
}

// windowPid returns the id of the program a window belongs to (0 for no window).
func windowPid(h uintptr) uint32 {
	var pid uint32
	if h != 0 {
		procGetWindowThreadProcessId.Call(h, uintptr(unsafe.Pointer(&pid)))
	}
	return pid
}

// bringToFront waits for the dialog to appear and makes sure it is the active window.
func bringToFront(owner uintptr) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// Give this thread a message queue, which AttachThreadInput needs.
	procPeekMessageW.Call(uintptr(unsafe.Pointer(&pk.msg[0])), 0, 0, 0, 0)

	var dlg uintptr
	for i := 0; i < 160 && dlg == 0; i++ { // up to 8 seconds
		time.Sleep(50 * time.Millisecond)
		dlg = findOwnDialog()
	}
	if dlg == 0 {
		return
	}
	const (
		hwndTopmost = ^uintptr(0)      // -1
		swpFlags    = 0x1 | 0x2 | 0x10 // no size, no move, no activate
	)
	if owner == 0 {
		// No owner window to sit in front of, so keep the dialog above other windows.
		procSetWindowPos.Call(dlg, hwndTopmost, 0, 0, 0, 0, swpFlags)
	}
	cur, _, _ := procGetCurrentThreadId.Call()
	me := uint32(os.Getpid())
	for try := 0; try < 3; try++ {
		time.Sleep(80 * time.Millisecond)
		if ok, _, _ := procIsWindow.Call(dlg); ok == 0 {
			return
		}
		fg, _, _ := procGetForegroundWindow.Call()
		if fg == dlg || windowPid(fg) == me {
			return
		}
		var fgThread uintptr
		if fg != 0 {
			fgThread, _, _ = procGetWindowThreadProcessId.Call(fg, 0)
		}
		attached := false
		if fgThread != 0 && fgThread != cur {
			r, _, _ := procAttachThreadInput.Call(cur, fgThread, 1)
			attached = r != 0
		}
		procBringWindowToTop.Call(dlg)
		procSetForegroundWindow.Call(dlg)
		if attached {
			procAttachThreadInput.Call(cur, fgThread, 0)
		}
	}
	// Still not in front: at least keep it above every other window so it cannot be lost.
	time.Sleep(80 * time.Millisecond)
	if fg, _, _ := procGetForegroundWindow.Call(); fg != dlg && windowPid(fg) != me {
		if ok, _, _ := procIsWindow.Call(dlg); ok != 0 {
			procSetWindowPos.Call(dlg, hwndTopmost, 0, 0, 0, 0, swpFlags)
		}
	}
}

// runPickHelper shows one chooser and prints what was chosen. Exit code 0 means it worked
// (including when the person pressed Cancel); anything else makes the main program fall back.
func runPickHelper() int {
	runtime.LockOSThread()
	kind := os.Getenv("AC8_KIND")
	title := os.Getenv("AC8_TITLE")
	filter := os.Getenv("AC8_FILTER")
	start := startFolder(os.Getenv("AC8_START"))
	owner64, _ := strconv.ParseUint(os.Getenv("AC8_OWNER"), 10, 64)
	owner := uintptr(owner64)
	if owner != 0 {
		if ok, _, _ := procIsWindow.Call(owner); ok == 0 {
			owner = 0
		}
	}
	fail := func(what string, hr uintptr) int {
		fmt.Fprintf(os.Stderr, "%s failed: 0x%08X\n", what, uint32(hr))
		return 3
	}

	setDPIAware()
	if hr, _, _ := procCoInitializeEx.Call(0, 0x2|0x4); failed(hr) { // apartment threaded, no OLE1 DDE
		return fail("CoInitializeEx", hr)
	}
	defer procCoUninitialize.Call()

	hr, _, _ := procCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidFileOpenDialog)), 0, 1,
		uintptr(unsafe.Pointer(&iidIFileOpenDialog)), uintptr(unsafe.Pointer(&pk.dlg)))
	if failed(hr) || pk.dlg == 0 {
		return fail("CoCreateInstance", hr)
	}
	defer release(pk.dlg)

	if hr := vcall(pk.dlg, vGetOptions, uintptr(unsafe.Pointer(&pk.opts))); failed(hr) {
		return fail("GetOptions", hr)
	}
	opts := pk.opts | fosForceFileSystem | fosPathMustExist | fosNoChangeDir
	if kind == "folder" {
		opts |= fosPickFolders
	} else {
		opts |= fosFileMustExist
		if kind == "files" {
			opts |= fosAllowMulti
		}
	}
	if hr := vcall(pk.dlg, vSetOptions, uintptr(opts)); failed(hr) {
		return fail("SetOptions", hr)
	}
	if title != "" {
		vcall(pk.dlg, vSetTitle, uintptr(unsafe.Pointer(keepStr(title))))
	}
	if kind != "folder" {
		parts := strings.Split(filter, "|")
		for i := 0; i+1 < len(parts); i += 2 {
			pk.specs = append(pk.specs, filterSpec{keepStr(parts[i]), keepStr(parts[i+1])})
		}
		if len(pk.specs) > 0 {
			vcall(pk.dlg, vSetFileTypes, uintptr(len(pk.specs)), uintptr(unsafe.Pointer(&pk.specs[0])))
		}
	}
	if start != "" {
		hr, _, _ := procSHCreateItemFromParsingName.Call(uintptr(unsafe.Pointer(keepStr(start))), 0,
			uintptr(unsafe.Pointer(&iidIShellItem)), uintptr(unsafe.Pointer(&pk.folder)))
		if !failed(hr) && pk.folder != 0 {
			vcall(pk.dlg, vSetFolder, pk.folder)
			defer release(pk.folder)
		}
	}

	go bringToFront(owner)

	hr = vcall(pk.dlg, vShow, owner)
	if uint32(hr) == hrCancelled {
		return 0
	}
	if failed(hr) {
		return fail("Show", hr)
	}

	var paths []string
	if kind == "files" {
		if hr := vcall(pk.dlg, vGetResults, uintptr(unsafe.Pointer(&pk.arr))); failed(hr) || pk.arr == 0 {
			return fail("GetResults", hr)
		}
		defer release(pk.arr)
		if hr := vcall(pk.arr, vGetCount, uintptr(unsafe.Pointer(&pk.count))); failed(hr) {
			return fail("GetCount", hr)
		}
		for i := uint32(0); i < pk.count; i++ {
			pk.item = 0
			if hr := vcall(pk.arr, vGetItemAt, uintptr(i), uintptr(unsafe.Pointer(&pk.item))); failed(hr) || pk.item == 0 {
				continue
			}
			if p := itemPath(pk.item); p != "" {
				paths = append(paths, p)
			}
			release(pk.item)
		}
	} else {
		if hr := vcall(pk.dlg, vGetResult, uintptr(unsafe.Pointer(&pk.item))); failed(hr) || pk.item == 0 {
			return fail("GetResult", hr)
		}
		if p := itemPath(pk.item); p != "" {
			paths = append(paths, p)
		}
		release(pk.item)
	}
	var b strings.Builder
	for _, p := range paths {
		b.WriteString("P:" + base64.StdEncoding.EncodeToString([]byte(p)) + "\n")
	}
	os.Stdout.WriteString(b.String())
	return 0
}
