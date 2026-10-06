//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")

	pRegisterClassExW         = user32.NewProc("RegisterClassExW")
	pCreateWindowExW          = user32.NewProc("CreateWindowExW")
	pDefWindowProcW           = user32.NewProc("DefWindowProcW")
	pGetMessageW              = user32.NewProc("GetMessageW")
	pTranslateMessage         = user32.NewProc("TranslateMessage")
	pDispatchMessageW         = user32.NewProc("DispatchMessageW")
	pIsDialogMessageW         = user32.NewProc("IsDialogMessageW")
	pPostQuitMessage          = user32.NewProc("PostQuitMessage")
	pSendMessageW             = user32.NewProc("SendMessageW")
	pShowWindow               = user32.NewProc("ShowWindow")
	pUpdateWindow             = user32.NewProc("UpdateWindow")
	pDestroyWindow            = user32.NewProc("DestroyWindow")
	pMessageBoxW              = user32.NewProc("MessageBoxW")
	pLoadCursorW              = user32.NewProc("LoadCursorW")
	pGetSystemMetrics         = user32.NewProc("GetSystemMetrics")
	pEnumWindows              = user32.NewProc("EnumWindows")
	pGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	pIsWindowVisible          = user32.NewProc("IsWindowVisible")
	pIsWindow                 = user32.NewProc("IsWindow")
	pGetWindow                = user32.NewProc("GetWindow")
	pGetWindowLongPtrW        = user32.NewProc("GetWindowLongPtrW")
	pSetWindowLongPtrW        = user32.NewProc("SetWindowLongPtrW")
	pSetWindowPos             = user32.NewProc("SetWindowPos")
	pGetWindowRect            = user32.NewProc("GetWindowRect")
	pMonitorFromWindow        = user32.NewProc("MonitorFromWindow")
	pGetMonitorInfoW          = user32.NewProc("GetMonitorInfoW")
	pEnumDisplaySettingsW     = user32.NewProc("EnumDisplaySettingsW")
	pSetProcessDPIAware       = user32.NewProc("SetProcessDPIAware")
	pEnableWindow             = user32.NewProc("EnableWindow")
	pSetWindowTextW           = user32.NewProc("SetWindowTextW")
	pGetDpiForSystem          = user32.NewProc("GetDpiForSystem")
	pIsIconic                 = user32.NewProc("IsIconic")

	pGetModuleHandleW           = kernel32.NewProc("GetModuleHandleW")
	pOpenProcess                = kernel32.NewProc("OpenProcess")
	pCloseHandle                = kernel32.NewProc("CloseHandle")
	pQueryFullProcessImageNameW = kernel32.NewProc("QueryFullProcessImageNameW")
	pSetProcessAffinityMask     = kernel32.NewProc("SetProcessAffinityMask")
	pGetProcessAffinityMask     = kernel32.NewProc("GetProcessAffinityMask")
	pGetFileAttributesW         = kernel32.NewProc("GetFileAttributesW")
	pSetFileAttributesW         = kernel32.NewProc("SetFileAttributesW")
	pCreateFontW                = gdi32.NewProc("CreateFontW")
	pGetStockObject             = gdi32.NewProc("GetStockObject")
	pExtractIconW               = shell32.NewProc("ExtractIconW")
	pShellExecuteW              = shell32.NewProc("ShellExecuteW")
)

const (
	WS_OVERLAPPED       = 0x00000000
	WS_CAPTION          = 0x00C00000
	WS_SYSMENU          = 0x00080000
	WS_THICKFRAME       = 0x00040000
	WS_MINIMIZEBOX      = 0x00020000
	WS_MAXIMIZEBOX      = 0x00010000
	WS_VISIBLE          = 0x10000000
	WS_CHILD            = 0x40000000
	WS_TABSTOP          = 0x00010000
	WS_VSCROLL          = 0x00200000
	WS_GROUP            = 0x00020000
	WS_EX_DLGMODALFRAME = 0x00000001
	WS_EX_WINDOWEDGE    = 0x00000100
	WS_EX_CLIENTEDGE    = 0x00000200
	WS_EX_STATICEDGE    = 0x00020000
	WS_EX_CONTROLPARENT = 0x00010000

	CBS_DROPDOWNLIST = 0x0003
	BS_AUTOCHECKBOX  = 0x0003
	BS_PUSHBUTTON    = 0x0000
	BS_DEFPUSHBUTTON = 0x0001
	SS_LEFT          = 0x0000

	WM_DESTROY        = 0x0002
	WM_CLOSE          = 0x0010
	WM_SETFONT        = 0x0030
	WM_SETICON        = 0x0080
	WM_COMMAND        = 0x0111
	WM_CTLCOLORSTATIC = 0x0138

	CB_ADDSTRING    = 0x0143
	CB_GETCURSEL    = 0x0147
	CB_RESETCONTENT = 0x014B
	CB_SETCURSEL    = 0x014E
	CBN_SELCHANGE   = 1
	BM_GETCHECK     = 0x00F0
	BM_SETCHECK     = 0x00F1
	BN_CLICKED      = 0

	GWL_STYLE   = -16
	GWL_EXSTYLE = -20

	SWP_NOZORDER      = 0x0004
	SWP_FRAMECHANGED  = 0x0020
	SWP_SHOWWINDOW    = 0x0040
	SWP_NOOWNERZORDER = 0x0200
	SWP_NOACTIVATE    = 0x0010

	SW_SHOW = 5
	SW_HIDE = 0

	MB_OK          = 0x0
	MB_ICONERROR   = 0x10
	MB_ICONWARNING = 0x30
	MB_ICONINFO    = 0x40

	MONITOR_DEFAULTTONEAREST          = 2
	GW_OWNER                          = 4
	PROCESS_QUERY_LIMITED_INFORMATION = 0x1000
	PROCESS_SET_INFORMATION           = 0x0200
	PROCESS_QUERY_INFORMATION         = 0x0400

	FILE_ATTRIBUTE_READONLY = 0x1
	INVALID_FILE_ATTRIBUTES = 0xFFFFFFFF
)

type WNDCLASSEXW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type POINT struct{ X, Y int32 }

type MSG struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      POINT
	Private uint32
}

type RECT struct{ Left, Top, Right, Bottom int32 }

type MONITORINFO struct {
	CbSize    uint32
	RcMonitor RECT
	RcWork    RECT
	DwFlags   uint32
}

func u16(s string) *uint16  { p, _ := syscall.UTF16PtrFromString(s); return p }
func u16p(s string) uintptr { return uintptr(unsafe.Pointer(u16(s))) }

func sendMsg(h uintptr, m uint32, w, l uintptr) uintptr {
	r, _, _ := pSendMessageW.Call(h, uintptr(m), w, l)
	return r
}

func msgBox(h uintptr, text, title string, flags uintptr) {
	pMessageBoxW.Call(h, u16p(text), u16p(title), flags)
}

func getWindowLong(h uintptr, idx int32) uintptr {
	r, _, _ := pGetWindowLongPtrW.Call(h, uintptr(idx))
	return r
}
func setWindowLong(h uintptr, idx int32, v uintptr) {
	pSetWindowLongPtrW.Call(h, uintptr(idx), v)
}

func setReadOnly(path string, ro bool) {
	a, _, _ := pGetFileAttributesW.Call(u16p(path))
	if uint32(a) == INVALID_FILE_ATTRIBUTES {
		return
	}
	if ro {
		a |= FILE_ATTRIBUTE_READONLY
	} else {
		a &^= FILE_ATTRIBUTE_READONLY
	}
	pSetFileAttributesW.Call(u16p(path), a)
}
