//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var (
	comctl32                = syscall.NewLazyDLL("comctl32.dll")
	pInitCommonControlsEx   = comctl32.NewProc("InitCommonControlsEx")
	pCreateMutexW           = kernel32.NewProc("CreateMutexW")
	pGetLastError           = kernel32.NewProc("GetLastError")
	pFindWindowW            = user32.NewProc("FindWindowW")
	pSetForegroundWindow    = user32.NewProc("SetForegroundWindow")
	pEnumDisplaySettingsExW = user32.NewProc("EnumDisplaySettingsExW")
)

// ------------------------------------------------------------------ tooltips

const (
	TTS_ALWAYSTIP      = 0x01
	TTS_NOPREFIX       = 0x02
	TTS_BALLOON        = 0x40
	TTF_IDISHWND       = 0x0001
	TTF_SUBCLASS       = 0x0010
	TTM_ADDTOOLW       = 0x0432
	TTM_SETMAXTIPWIDTH = 0x0418
	TTM_SETTITLEW      = 0x0421
	TTM_SETDELAYTIME   = 0x0403
	TTDT_AUTOPOP       = 2
	TTDT_INITIAL       = 3
)

type TOOLINFOW struct {
	CbSize     uint32
	UFlags     uint32
	Hwnd       uintptr
	UId        uintptr
	Rect       RECT
	Hinst      uintptr
	LpszText   *uint16
	LParam     uintptr
	LpReserved uintptr
}

var tipInfo, tipWarn uintptr

func newTooltip(title string, icon uintptr) uintptr {
	hInst, _, _ := pGetModuleHandleW.Call(0)
	t, _, _ := pCreateWindowExW.Call(0x00000008 /*WS_EX_TOPMOST*/, u16p("tooltips_class32"), 0,
		0x80000000|TTS_ALWAYSTIP|TTS_NOPREFIX|TTS_BALLOON, 0x80000000, 0x80000000, 0x80000000, 0x80000000, hMain, 0, hInst, 0)
	sendMsg(t, TTM_SETMAXTIPWIDTH, 0, uintptr(sc(340)))
	sendMsg(t, TTM_SETDELAYTIME, TTDT_AUTOPOP, 32000)
	sendMsg(t, TTM_SETDELAYTIME, TTDT_INITIAL, 450)
	sendMsg(t, TTM_SETTITLEW, icon, u16p(title))
	return t
}

func initTooltips() {
	type icc struct{ Size, Classes uint32 }
	c := icc{8, 0xFF}
	pInitCommonControlsEx.Call(uintptr(unsafe.Pointer(&c)))
	tipInfo = newTooltip("Information", 1)
	tipWarn = newTooltip("Warning", 2)
	setTipsActive(settings.Tooltips)
}

var tipTexts = map[uintptr]*uint16{} // keep strings alive

func addTip(h uintptr, text string, warn bool) {
	if h == 0 || text == "" {
		return
	}
	t := tipInfo
	if warn {
		t = tipWarn
	}
	p := u16(text)
	tipTexts[h] = p
	ti := TOOLINFOW{CbSize: uint32(unsafe.Sizeof(TOOLINFOW{})), UFlags: TTF_IDISHWND | TTF_SUBCLASS, Hwnd: hMain, UId: h, LpszText: p}
	sendMsg(t, TTM_ADDTOOLW, 0, uintptr(unsafe.Pointer(&ti)))
}

// ------------------------------------------------------------------ monitors

type Monitor struct {
	Device  string
	Name    string
	X, Y    int32
	W, H    int32
	Primary bool
	Ordinal int // Direct3D adapter index (primary = 0)
}

var monitors []Monitor

func enumMonitors() {
	monitors = nil
	var dd [840]byte
	*(*uint32)(unsafe.Pointer(&dd[0])) = 840
	others := 1
	for i := 0; i < 16; i++ {
		r, _, _ := pEnumDisplayDevicesW.Call(0, uintptr(i), uintptr(unsafe.Pointer(&dd[0])), 0)
		if r == 0 {
			break
		}
		flags := *(*uint32)(unsafe.Pointer(&dd[324]))
		if flags&1 == 0 { // not attached to desktop
			continue
		}
		dev := syscall.UTF16ToString(unsafe.Slice((*uint16)(unsafe.Pointer(&dd[4])), 32))
		var dm [220]byte
		*(*uint16)(unsafe.Pointer(&dm[68])) = 220
		ok, _, _ := pEnumDisplaySettingsExW.Call(u16p(dev), ^uintptr(0) /*ENUM_CURRENT_SETTINGS*/, uintptr(unsafe.Pointer(&dm[0])), 0)
		if ok == 0 {
			continue
		}
		m := Monitor{Device: dev, Primary: flags&4 != 0}
		m.X = *(*int32)(unsafe.Pointer(&dm[76]))
		m.Y = *(*int32)(unsafe.Pointer(&dm[80]))
		m.W = int32(*(*uint32)(unsafe.Pointer(&dm[172])))
		m.H = int32(*(*uint32)(unsafe.Pointer(&dm[176])))
		if m.Primary {
			m.Ordinal = 0
		} else {
			m.Ordinal = others
			others++
		}
		monitors = append(monitors, m)
	}
	for i := range monitors {
		m := &monitors[i]
		m.Name = fmt.Sprintf("Display %d", m.Ordinal+1)
		if m.Primary {
			m.Name += " (Main)"
		}
	}
}

func monitorModes(dev string) [][2]int {
	seen := map[[2]int]bool{}
	var dm [220]byte
	*(*uint16)(unsafe.Pointer(&dm[68])) = 220
	for i := 0; ; i++ {
		r, _, _ := pEnumDisplaySettingsExW.Call(u16p(dev), uintptr(i), uintptr(unsafe.Pointer(&dm[0])), 0)
		if r == 0 {
			break
		}
		w := int(*(*uint32)(unsafe.Pointer(&dm[172])))
		h := int(*(*uint32)(unsafe.Pointer(&dm[176])))
		if w >= 1024 && h >= 720 {
			seen[[2]int{w, h}] = true
		}
	}
	var out [][2]int
	for k := range seen {
		out = append(out, k)
	}
	return out
}

// ------------------------------------------------------------------ logging

var logFile *os.File

func initLog() {
	dir := filepath.Join(exeDir, "logs")
	os.MkdirAll(dir, 0755)
	if ents, err := os.ReadDir(dir); err == nil {
		for _, e := range ents {
			if info, err := e.Info(); err == nil && strings.HasPrefix(e.Name(), "launcher_") && time.Since(info.ModTime()) > time.Duration(logDayValues[clamp(settings.LogDays, 0, len(logDayValues)-1)])*24*time.Hour {
				os.Remove(filepath.Join(dir, e.Name()))
			}
		}
	}
	f, err := os.Create(filepath.Join(dir, "launcher_"+time.Now().Format("2006-01-02_15-04-05")+".log"))
	if err == nil {
		logFile = f
	}
	logf("%s v%s started", appTitle, appVersion)
}

func logf(format string, a ...any) {
	if logFile == nil {
		return
	}
	fmt.Fprintf(logFile, "[%s] %s\r\n", time.Now().Format("15:04:05"), fmt.Sprintf(format, a...))
	logFile.Sync()
}

// ------------------------------------------------------------------ single instance

var hMutex uintptr

func releaseInstance() {
	if hMutex != 0 {
		pCloseHandle.Call(hMutex)
		hMutex = 0
	}
}

func alreadyRunning(cls string) bool {
	hMutex, _, _ = pCreateMutexW.Call(0, 0, u16p("Local\\MKUKE_Launcher_SingleInstance"))
	e, _, _ := pGetLastError.Call()
	if e != 183 { // ERROR_ALREADY_EXISTS
		return false
	}
	if w, _, _ := pFindWindowW.Call(u16p(cls), 0); w != 0 {
		pShowWindow.Call(w, 9 /*SW_RESTORE*/)
		pSetForegroundWindow.Call(w)
	}
	return true
}

func confirm(h uintptr, text, title string) bool {
	r, _, _ := pMessageBoxW.Call(h, u16p(text), u16p(title), 0x4|0x20 /*MB_YESNO|MB_ICONQUESTION*/)
	return r == 6
}

func addRectTip(r RECT, text string) {
	p := u16(text)
	tipTexts[0xB4] = p
	ti := TOOLINFOW{CbSize: uint32(unsafe.Sizeof(TOOLINFOW{})), UFlags: TTF_SUBCLASS, Hwnd: hMain, UId: 0xB4, Rect: r, LpszText: p}
	sendMsg(tipInfo, TTM_ADDTOOLW, 0, uintptr(unsafe.Pointer(&ti)))
}

func setTipsActive(on bool) {
	v := uintptr(0)
	if on {
		v = 1
	}
	sendMsg(tipInfo, 0x0401 /*TTM_ACTIVATE*/, v, 0)
	sendMsg(tipWarn, 0x0401, v, 0)
}
