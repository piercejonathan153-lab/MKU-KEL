//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// ---------------------------------------------------------------- resolutions

func buildResList(dev string, w, h int) {
	seen := map[[2]int]bool{}
	for _, p := range monitorModes(dev) {
		seen[p] = true
	}
	for _, p := range [][2]int{{1280, 720}, {1920, 1080}, {2560, 1440}, {3840, 2160}, {2560, 1080}, {3440, 1440}, {3840, 1600}, {5120, 1440}} {
		seen[p] = true
	}
	if w > 0 {
		seen[[2]int{w, h}] = true
	}
	if opts.ResX > 0 {
		seen[[2]int{opts.ResX, opts.ResY}] = true
	}
	resList = resList[:0]
	for k := range seen {
		resList = append(resList, k)
	}
	sort.Slice(resList, func(a, b int) bool {
		if resList[a][0] != resList[b][0] {
			return resList[a][0] > resList[b][0]
		}
		return resList[a][1] > resList[b][1]
	})
}

func nativeRes() (int, int) {
	w, _, _ := pGetSystemMetrics.Call(0)
	h, _, _ := pGetSystemMetrics.Call(1)
	return int(w), int(h)
}

func resLabel(p [2]int) string {
	g := gcd(p[0], p[1])
	ar := fmt.Sprintf("%d:%d", p[0]/g, p[1]/g)
	switch ar {
	case "64:27", "43:18", "12:5":
		ar = "21:9"
	case "8:5":
		ar = "16:10"
	}
	return fmt.Sprintf("%dx%d (%s)", p[0], p[1], ar)
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// ---------------------------------------------------------------- game launch + borderless

func runGameInner() {
	pid, err := startMKKE()
	if err != nil {
		msgBox(0, "Could not start MKKE.exe:\n\n"+err.Error(), appTitle, MB_ICONERROR)
		return
	}
	logf("started MKKE.exe (pid %d), mode=%d renderer=%d", pid, settings.Mode, settings.Renderer)
	if settings.HighPriority {
		logf("high priority: %v", setHighPriority(uintptr(pid)))
	}
	if settings.Mode != 1 && !settings.Ryzen {
		return
	}
	start := time.Now()
	found, affinityDone := false, false
	moves, fallbacks := 0, 0
	stableSince := time.Time{}
	for time.Since(start) < 5*time.Minute {
		time.Sleep(300 * time.Millisecond)
		hw, pid := findGameWindow()
		if hw == 0 {
			if found && time.Since(stableSince) > 10*time.Second {
				return
			}
			continue
		}
		if !found {
			found = true
			stableSince = time.Now()
			logf("game window found")
		}
		if settings.Ryzen && !affinityDone {
			affinityDone = applyAffinity(pid)
			logf("ryzen fix applied: %v", affinityDone)
		}
		if settings.Mode != 1 {
			if affinityDone {
				return
			}
			continue
		}
		style := getWindowLong(hw, GWL_STYLE)
		var r RECT
		pGetWindowRect.Call(hw, uintptr(unsafe.Pointer(&r)))
		m := targetRect
		if style&WS_CAPTION != 0 {
			// exe patch not active: strip the frame ourselves, but never more than 3 times (avoids flicker loops)
			if fallbacks < 3 {
				fallbacks++
				logf("window still has a title bar, fallback borderless #%d", fallbacks)
				makeBorderless(hw)
				stableSince = time.Now()
			}
		} else if m.Right > m.Left && (r.Left != m.Left || r.Top != m.Top) && moves < 5 {
			moves++
			logf("moving game window to target monitor (%d,%d)", m.Left, m.Top)
			pSetWindowPos.Call(hw, 0, uintptr(m.Left), uintptr(m.Top), 0, 0, 0x0001|SWP_NOZORDER|SWP_NOACTIVATE)
			stableSince = time.Now()
		}
		if time.Since(stableSince) > 15*time.Second && (!settings.Ryzen || affinityDone) {
			logf("borderless window stable, launcher exiting")
			return
		}
	}
}

// ------------------------------------------------------------------ MKKE.exe byte patches

type exePatch struct {
	off       int64
	orig, new []byte
}

var (
	patchWindowedFix = exePatch{0x187de9, []byte{0xc6, 0x05, 0x80, 0x9b, 0xde, 0x00, 0x01}, []byte{0xc6, 0x05, 0x80, 0x9b, 0xde, 0x00, 0x00}}
	patchWinPos      = exePatch{0x13bf41, []byte{0xba, 0x64, 0x00, 0x00, 0x00}, []byte{0xba, 0x00, 0x00, 0x00, 0x00}}
	patchWinStyle    = exePatch{0x13bf48, []byte{0xc7, 0x45, 0xf8, 0x00, 0x00, 0xca, 0x00}, []byte{0xc7, 0x45, 0xf8, 0x00, 0x00, 0x00, 0x80}}
)

// patchExe makes sure the windowed-mode fix is present and switches the game's own
// windowed style between a normal window (title bar) and a borderless popup.
func patchExe(borderless bool) error { return patchExeState(true, borderless) }

// patchExeState sets every MKKE.exe patch; fix=false, borderless=false restores the original bytes.
func patchExeState(fix, borderless bool) error {
	f, err := os.OpenFile(filepath.Join(exeDir, "MKKE.exe"), os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("could not open MKKE.exe for writing (is the game still running?): %v", err)
	}
	defer f.Close()
	apply := func(p exePatch, want bool) error {
		buf := make([]byte, len(p.orig))
		if _, err := f.ReadAt(buf, p.off); err != nil {
			return err
		}
		target := p.orig
		if want {
			target = p.new
		}
		if string(buf) == string(target) {
			return nil
		}
		if string(buf) != string(p.orig) && string(buf) != string(p.new) {
			return fmt.Errorf("unexpected MKKE.exe version at 0x%x", p.off)
		}
		_, err := f.WriteAt(target, p.off)
		logf("MKKE.exe patch at 0x%x -> %v", p.off, want)
		return err
	}
	if err := apply(patchWindowedFix, fix); err != nil {
		return err
	}
	if err := apply(patchWinPos, borderless); err != nil {
		return err
	}
	return apply(patchWinStyle, borderless)
}

var enumResult, enumPid uintptr
var enumCB = syscall.NewCallback(func(h, l uintptr) uintptr {
	vis, _, _ := pIsWindowVisible.Call(h)
	if vis == 0 {
		return 1
	}
	if own, _, _ := pGetWindow.Call(h, GW_OWNER); own != 0 {
		return 1
	}
	var r RECT
	pGetWindowRect.Call(h, uintptr(unsafe.Pointer(&r)))
	if r.Right-r.Left < 320 || r.Bottom-r.Top < 240 {
		return 1
	}
	var pid uint32
	pGetWindowThreadProcessId.Call(h, uintptr(unsafe.Pointer(&pid)))
	if strings.EqualFold(filepath.Base(processPath(pid)), "MKKE.exe") {
		enumResult, enumPid = h, uintptr(pid)
		return 0
	}
	return 1
})

func findGameWindow() (uintptr, uintptr) {
	enumResult, enumPid = 0, 0
	pEnumWindows.Call(enumCB, 0)
	return enumResult, enumPid
}

func processPath(pid uint32) string {
	hp, _, _ := pOpenProcess.Call(PROCESS_QUERY_LIMITED_INFORMATION, 0, uintptr(pid))
	if hp == 0 {
		return ""
	}
	defer pCloseHandle.Call(hp)
	buf := make([]uint16, 1024)
	n := uint32(len(buf))
	r, _, _ := pQueryFullProcessImageNameW.Call(hp, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n)))
	if r == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:n])
}

var targetRect RECT

func makeBorderless(h uintptr) {
	if ic, _, _ := pIsIconic.Call(h); ic != 0 {
		return
	}
	style := getWindowLong(h, GWL_STYLE)
	ex := getWindowLong(h, GWL_EXSTYLE)
	const frame = WS_CAPTION | WS_THICKFRAME | WS_MINIMIZEBOX | WS_MAXIMIZEBOX | WS_SYSMENU
	const exFrame = WS_EX_DLGMODALFRAME | WS_EX_WINDOWEDGE | WS_EX_CLIENTEDGE | WS_EX_STATICEDGE

	m := targetRect
	if m.Right == m.Left {
		mon, _, _ := pMonitorFromWindow.Call(h, MONITOR_DEFAULTTONEAREST)
		mi := MONITORINFO{CbSize: uint32(unsafe.Sizeof(MONITORINFO{}))}
		pGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi)))
		m = mi.RcMonitor
	}
	var r RECT
	pGetWindowRect.Call(h, uintptr(unsafe.Pointer(&r)))

	if style&frame == 0 && ex&exFrame == 0 && r == m {
		return
	}
	setWindowLong(h, GWL_STYLE, style&^frame)
	setWindowLong(h, GWL_EXSTYLE, ex&^exFrame)
	pSetWindowPos.Call(h, 0, uintptr(m.Left), uintptr(m.Top), uintptr(m.Right-m.Left), uintptr(m.Bottom-m.Top),
		SWP_FRAMECHANGED|SWP_NOZORDER|SWP_NOOWNERZORDER|SWP_NOACTIVATE|SWP_SHOWWINDOW)
}

// Ryzen fix placeholder: restrict the game to the first 8 logical CPUs.
func applyAffinity(pid uintptr) bool {
	hp, _, _ := pOpenProcess.Call(PROCESS_SET_INFORMATION|PROCESS_QUERY_INFORMATION, 0, pid)
	if hp == 0 {
		return false
	}
	defer pCloseHandle.Call(hp)
	var procMask, sysMask uintptr
	pGetProcessAffinityMask.Call(hp, uintptr(unsafe.Pointer(&procMask)), uintptr(unsafe.Pointer(&sysMask)))
	want := sysMask & 0xFF
	if want == 0 {
		return true
	}
	r, _, _ := pSetProcessAffinityMask.Call(hp, want)
	return r != 0
}

// runGame starts the game, then (optionally) brings the launcher back once the game closes.
func runGame() {
	runGameInner()
	if settings.AfterLaunch != 1 {
		return
	}
	logf("waiting for the game to close (reopen launcher)")
	found := false
	gone := 0
	for i := 0; i < 6*60*60; i++ { // give up after 6 hours of waiting
		time.Sleep(time.Second)
		if hw, _ := findGameWindow(); hw != 0 {
			found, gone = true, 0
			continue
		}
		if !found {
			if i > 180 {
				logf("game window never appeared, not reopening")
				return
			}
			continue
		}
		gone++
		if gone >= 3 {
			break
		}
	}
	logf("game closed, reopening launcher")
	releaseInstance()
	ex, _ := os.Executable()
	c := exec.Command(ex, "--no-quick")
	c.Dir = exeDir
	c.Start()
}

var (
	pShellExecuteExW = shell32.NewProc("ShellExecuteExW")
	pGetProcessId    = kernel32.NewProc("GetProcessId")
)

type shellExecuteInfo struct {
	Size       uint32
	Mask       uint32
	Hwnd       uintptr
	Verb       *uint16
	File       *uint16
	Parameters *uint16
	Directory  *uint16
	Show       int32
	InstApp    uintptr
	IDList     uintptr
	Class      *uint16
	KeyClass   uintptr
	HotKey     uint32
	IconOrMon  uintptr
	Process    uintptr
}

// startMKKE launches MKKE.exe and returns its process id. If Windows says the game needs
// administrator rights (e.g. "Run as administrator" is ticked on MKKE.exe), it is started
// through the shell instead, which shows the normal UAC prompt.
func startMKKE() (int, error) {
	exe := filepath.Join(exeDir, "MKKE.exe")
	cmd := exec.Command(exe)
	cmd.Dir = exeDir
	err := cmd.Start()
	if err == nil {
		pid := cmd.Process.Pid
		cmd.Process.Release()
		return pid, nil
	}
	if errno, ok := underlyingErrno(err); !ok || errno != 740 { // ERROR_ELEVATION_REQUIRED
		return 0, err
	}
	logf("MKKE.exe requires administrator rights - starting it through the shell (UAC)")
	sei := shellExecuteInfo{Mask: 0x40 /*SEE_MASK_NOCLOSEPROCESS*/, Verb: u16("runas"), File: u16(exe), Directory: u16(exeDir), Show: SW_SHOW}
	sei.Size = uint32(unsafe.Sizeof(sei))
	if r, _, e := pShellExecuteExW.Call(uintptr(unsafe.Pointer(&sei))); r == 0 {
		return 0, fmt.Errorf("MKKE.exe needs administrator rights and could not be started that way (%v).\n\nTry running the launcher as administrator, or untick \"Run this program as an administrator\" in MKKE.exe's Properties > Compatibility", e)
	}
	pid := 0
	if sei.Process != 0 {
		p, _, _ := pGetProcessId.Call(sei.Process)
		pid = int(p)
		pCloseHandle.Call(sei.Process)
	}
	return pid, nil
}

func underlyingErrno(err error) (syscall.Errno, bool) {
	for e := err; e != nil; {
		if n, ok := e.(syscall.Errno); ok {
			return n, true
		}
		u, ok := e.(interface{ Unwrap() error })
		if !ok {
			return 0, false
		}
		e = u.Unwrap()
	}
	return 0, false
}
