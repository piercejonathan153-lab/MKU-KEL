//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

var (
	uxtheme                = syscall.NewLazyDLL("uxtheme.dll")
	dwmapi                 = syscall.NewLazyDLL("dwmapi.dll")
	pSetWindowTheme        = uxtheme.NewProc("SetWindowTheme")
	pDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")
	pRedrawWindow          = user32.NewProc("RedrawWindow")
	pGetAsyncKeyState      = user32.NewProc("GetAsyncKeyState")
	pGetComboBoxInfo       = user32.NewProc("GetComboBoxInfo")
	pPolyline              = gdi32.NewProc("Polyline")
)

const (
	WM_CTLCOLORLISTBOX = 0x0134
	WM_SETTINGCHANGE   = 0x001A
	HKEY_CURRENT_USER  = 0x80000001
)

// theme colours (set by setThemeColors)
var (
	colBg, colHoverBg, colPressBg, colDisabled, colHint, colTabHover uintptr
	bgBrush                                                          uintptr
	darkActive                                                       bool
	comboCtls                                                        []uintptr
)

func windowsUsesDark() bool {
	var v, n uint32 = 1, 4
	r, _, _ := pRegGetValueW.Call(HKEY_CURRENT_USER,
		u16p(`Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`), u16p("AppsUseLightTheme"),
		0x10 /*RRF_RT_REG_DWORD*/, 0, uintptr(unsafe.Pointer(&v)), uintptr(unsafe.Pointer(&n)))
	return r == 0 && v == 0
}

type Theme struct {
	Name                           string
	Bg, Text, Border, Accent, Hint uintptr
	Dark                           bool
}

var themes = []Theme{
	{"Light", rgb(255, 255, 255), rgb(20, 20, 20), rgb(205, 205, 205), rgb(30, 100, 200), rgb(150, 60, 30), false},
	{"Dark", rgb(30, 31, 36), rgb(232, 232, 236), rgb(72, 74, 84), rgb(95, 165, 255), rgb(245, 160, 110), true},
	{"Match Windows", 0, 0, 0, 0, 0, false}, // resolved to Light / Dark
	{"Midnight", rgb(16, 22, 38), rgb(225, 232, 245), rgb(52, 64, 94), rgb(90, 160, 255), rgb(255, 170, 120), true},
	{"OLED Black", rgb(0, 0, 0), rgb(235, 235, 235), rgb(58, 58, 58), rgb(95, 165, 255), rgb(245, 160, 110), true},
	{"Netherrealm Red", rgb(28, 12, 13), rgb(242, 226, 226), rgb(92, 40, 40), rgb(230, 60, 50), rgb(255, 190, 110), true},
	{"Outworld Purple", rgb(26, 18, 36), rgb(236, 226, 246), rgb(76, 56, 102), rgb(180, 110, 255), rgb(255, 180, 120), true},
	{"Jade Green", rgb(13, 28, 21), rgb(222, 242, 232), rgb(46, 86, 66), rgb(60, 200, 120), rgb(240, 200, 110), true},
	{"Shirai Ryu Fire", rgb(30, 20, 12), rgb(246, 232, 216), rgb(96, 62, 32), rgb(255, 140, 30), rgb(255, 214, 120), true},
	{"Elder Gods Gold", rgb(24, 24, 24), rgb(236, 230, 214), rgb(82, 74, 50), rgb(230, 180, 50), rgb(250, 160, 110), true},
	{"Lin Kuei Ice", rgb(236, 244, 252), rgb(15, 30, 50), rgb(178, 200, 222), rgb(20, 120, 200), rgb(150, 60, 30), false},
	{"Earthrealm Sand", rgb(245, 238, 224), rgb(45, 35, 25), rgb(205, 190, 165), rgb(160, 90, 30), rgb(150, 60, 30), false},
}

type NamedColor struct {
	Name string
	C    uintptr
}

var accentColors = []NamedColor{
	{"Theme Default", 0}, {"Blue", rgb(40, 120, 230)}, {"Red", rgb(220, 45, 45)}, {"Green", rgb(40, 170, 80)},
	{"Yellow", rgb(235, 200, 40)}, {"Orange", rgb(245, 130, 30)}, {"Purple", rgb(150, 80, 220)}, {"Pink", rgb(235, 80, 160)},
	{"Cyan", rgb(30, 190, 210)}, {"Gold", rgb(212, 170, 50)}, {"White", rgb(240, 240, 240)}, {"Gray", rgb(140, 140, 140)}, {"Black", rgb(25, 25, 25)},
}

var textColors = []NamedColor{
	{"Theme Default", 0}, {"White", rgb(245, 245, 245)}, {"Black", rgb(15, 15, 15)}, {"Light Gray", rgb(190, 190, 190)},
	{"Red", rgb(235, 70, 70)}, {"Green", rgb(70, 200, 100)}, {"Yellow", rgb(240, 210, 60)}, {"Orange", rgb(245, 150, 50)},
	{"Blue", rgb(80, 150, 240)}, {"Purple", rgb(170, 110, 240)}, {"Pink", rgb(240, 110, 180)}, {"Cyan", rgb(60, 200, 220)}, {"Gold", rgb(220, 180, 60)},
}

func currentTheme() Theme {
	i := clamp(settings.Theme, 0, len(themes)-1)
	if i == 2 {
		if windowsUsesDark() {
			return themes[1]
		}
		return themes[0]
	}
	return themes[i]
}

func wantDark() bool { return currentTheme().Dark }

func mix(a, b uintptr, t float64) uintptr {
	ch := func(x uintptr, sh uint) float64 { return float64((x >> sh) & 0xFF) }
	m := func(sh uint) uint8 { return uint8(ch(a, sh)*t + ch(b, sh)*(1-t) + 0.5) }
	return rgb(m(0), m(8), m(16))
}

func luminance(c uintptr) float64 {
	return 0.299*float64(c&0xFF) + 0.587*float64((c>>8)&0xFF) + 0.114*float64((c>>16)&0xFF)
}

// contrastOn returns black or white, whichever reads better on c.
func contrastOn(c uintptr) uintptr {
	if luminance(c) > 150 {
		return rgb(15, 15, 15)
	}
	return rgb(255, 255, 255)
}

func setThemeColors(_ bool) {
	t := currentTheme()
	darkActive = t.Dark
	colBg, colText, colBorder, colBlue, colHint = t.Bg, t.Text, t.Border, t.Accent, t.Hint
	if a := accentColors[clamp(settings.Accent, 0, len(accentColors)-1)]; a.C != 0 {
		colBlue = a.C
		colBorder = mix(a.C, colBg, 0.6)
	}
	if c := textColors[clamp(settings.TextColor, 0, len(textColors)-1)]; c.C != 0 {
		colText = c.C
	}
	colGray = mix(colText, colBg, 0.5)
	colHoverBg = mix(colBlue, colBg, 0.14)
	colPressBg = mix(colBlue, colBg, 0.28)
	colDisabled = mix(colText, colBg, 0.35)
	colTabHover = mix(colBlue, colText, 0.6)
	if bgBrush != 0 {
		pDeleteObject.Call(bgBrush)
	}
	bgBrush, _, _ = pCreateSolidBrush.Call(colBg)
}

// swatch returns the colour sample shown next to an item in the colour/theme dropdowns.
func swatch(id, i int) (fill, border uintptr, ok bool) {
	switch id {
	case idTheme:
		if i < 0 || i >= len(themes) {
			return
		}
		t := themes[i]
		if i == 2 {
			return rgb(255, 255, 255), rgb(30, 31, 36), true
		}
		return t.Bg, t.Accent, true
	case idAccent:
		if i <= 0 || i >= len(accentColors) {
			return
		}
		return accentColors[i].C, colGray, true
	case idTextColor:
		if i <= 0 || i >= len(textColors) {
			return
		}
		return textColors[i].C, colGray, true
	}
	return
}

const ODT_COMBOBOX = 3
const ODS_COMBOBOXEDIT = 0x1000

// drawComboItem paints one entry of an owner-drawn dropdown (the closed field or a list row).
func drawComboItem(d *DRAWITEMSTRUCT) {
	r := d.RcItem
	field := d.ItemState&ODS_COMBOBOXEDIT != 0
	bg, fg := colBg, colText
	if !field && d.ItemState&ODS_SELECTED != 0 {
		bg = mix(colBlue, colBg, 0.45)
		if d := luminance(bg) - luminance(colText); d > -90 && d < 90 {
			fg = contrastOn(bg)
		}
	}
	if !field {
		fillRect(d.HDC, r, bg)
	} else if d.ItemState&ODS_DISABLED != 0 {
		fg = colDisabled
	}
	if d.ItemID == 0xFFFFFFFF {
		return
	}
	drawComboText(d.HDC, r, d.HwndItem, int(d.CtlID), int(d.ItemID), fg)
}

func drawComboText(hdc uintptr, r RECT, h uintptr, id, item int, fg uintptr) {
	buf := make([]uint16, 128)
	n := sendMsg(h, 0x0148 /*CB_GETLBTEXT*/, uintptr(item), uintptr(unsafe.Pointer(&buf[0])))
	if int32(n) < 0 {
		return
	}
	txt := syscall.UTF16ToString(buf)
	tr := RECT{r.Left + sc(4), r.Top, r.Right - sc(2), r.Bottom}
	if f, b, ok := swatch(id, item); ok {
		s := sc(12)
		top := r.Top + (r.Bottom-r.Top-s)/2
		sr := RECT{tr.Left, top, tr.Left + s, top + s}
		fillRect(hdc, sr, f)
		frameRect(hdc, sr, b)
		tr.Left += s + sc(7)
	}
	drawText(hdc, txt, tr, fSmall, fg, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
}

// ---- fully custom-painted closed dropdown (so its face matches the theme background)

var (
	pSetWindowSubclass = comctl32.NewProc("SetWindowSubclass")
	pDefSubclassProc   = comctl32.NewProc("DefSubclassProc")
	pGetFocus          = user32.NewProc("GetFocus")
	pIsWindowEnabled   = user32.NewProc("IsWindowEnabled")
	pGetDlgCtrlID      = user32.NewProc("GetDlgCtrlID")
	comboProcCB        uintptr
	hoverCombo         uintptr
)

func subclassCombo(h uintptr) {
	if comboProcCB == 0 {
		comboProcCB = syscall.NewCallback(comboProc)
	}
	pSetWindowSubclass.Call(h, comboProcCB, 1, 0)
}

func comboProc(h, msg, wp, lp, id, ref uintptr) uintptr {
	switch msg {
	case WM_ERASEBKGND:
		return 1
	case WM_PAINT:
		var ps PAINTSTRUCT
		hdc, _, _ := pBeginPaint.Call(h, uintptr(unsafe.Pointer(&ps)))
		var cr RECT
		pGetClientRect.Call(h, uintptr(unsafe.Pointer(&cr)))
		mdc, _, _ := pCreateCompatibleDC.Call(hdc)
		bmp, _, _ := pCreateCompatibleBitmap.Call(hdc, uintptr(cr.Right), uintptr(cr.Bottom))
		old, _, _ := pSelectObject.Call(mdc, bmp)

		en, _, _ := pIsWindowEnabled.Call(h)
		foc, _, _ := pGetFocus.Call()
		dropped := sendMsg(h, 0x0157 /*CB_GETDROPPEDSTATE*/, 0, 0) != 0
		bg := colBg
		if h == hoverCombo && en != 0 {
			bg = colHoverBg
		}
		fillRect(mdc, cr, bg)
		border := colBorder
		if en != 0 && (foc == h || dropped || h == hoverCombo) {
			border = colBlue
		}
		frameRect(mdc, cr, border)
		fg := colText
		if en == 0 {
			fg = colDisabled
		}
		aw := sc(20)
		tr := RECT{cr.Left + sc(2), cr.Top, cr.Right - aw, cr.Bottom}
		if cur := int32(sendMsg(h, CB_GETCURSEL, 0, 0)); cur >= 0 {
			cid, _, _ := pGetDlgCtrlID.Call(h)
			drawComboText(mdc, tr, h, int(cid), int(cur), fg)
		}
		// chevron
		cx := cr.Right - aw/2 - sc(1)
		cy := (cr.Top + cr.Bottom) / 2
		pen, _, _ := pCreatePen.Call(0, uintptr(sc(1)+1), fg)
		op, _, _ := pSelectObject.Call(mdc, pen)
		pts := []int32{cx - sc(4), cy - sc(2), cx, cy + sc(2), cx + sc(4), cy - sc(2)}
		pPolyline.Call(mdc, uintptr(unsafe.Pointer(&pts[0])), 3)
		pSelectObject.Call(mdc, op)
		pDeleteObject.Call(pen)

		pBitBlt.Call(hdc, 0, 0, uintptr(cr.Right), uintptr(cr.Bottom), mdc, 0, 0, SRCCOPY)
		pSelectObject.Call(mdc, old)
		pDeleteObject.Call(bmp)
		pDeleteDC.Call(mdc)
		pEndPaint.Call(h, uintptr(unsafe.Pointer(&ps)))
		return 0
	case WM_MOUSEMOVE:
		if hoverCombo != h {
			prev := hoverCombo
			hoverCombo = h
			if prev != 0 {
				pInvalidateRect.Call(prev, 0, 0)
			}
			pInvalidateRect.Call(h, 0, 0)
			tme := TRACKMOUSEEVENT{CbSize: uint32(unsafe.Sizeof(TRACKMOUSEEVENT{})), DwFlags: TME_LEAVE, HwndTrack: h}
			pTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
		}
	case WM_MOUSELEAVE:
		if hoverCombo == h {
			hoverCombo = 0
			pInvalidateRect.Call(h, 0, 0)
		}
	case 0x0007, 0x0008, 0x000A: // WM_SETFOCUS, WM_KILLFOCUS, WM_ENABLE
		r, _, _ := pDefSubclassProc.Call(h, msg, wp, lp)
		pInvalidateRect.Call(h, 0, 0)
		return r
	}
	r, _, _ := pDefSubclassProc.Call(h, msg, wp, lp)
	return r
}

// applyTheme re-themes the whole window live.
func applyTheme() {
	dark := wantDark()
	setThemeColors(dark)
	if hMain == 0 {
		return
	}
	v := uint32(0)
	if dark {
		v = 1
	}
	if r, _, _ := pDwmSetWindowAttribute.Call(hMain, 20, uintptr(unsafe.Pointer(&v)), 4); r != 0 {
		pDwmSetWindowAttribute.Call(hMain, 19, uintptr(unsafe.Pointer(&v)), 4) // older Windows 10
	}
	for _, c := range comboCtls {
		themeCombo(c, dark)
	}
	for _, t := range []uintptr{tipInfo, tipWarn} {
		if t == 0 {
			continue
		}
		if dark {
			pSetWindowTheme.Call(t, u16p("DarkMode_Explorer"), 0)
		} else {
			pSetWindowTheme.Call(t, 0, 0)
		}
	}
	pSetWindowPos.Call(hMain, 0, 0, 0, 0, 0, 0x0001|0x0002|0x0004|0x0020 /*NOSIZE|NOMOVE|NOZORDER|FRAMECHANGED*/)
	pRedrawWindow.Call(hMain, 0, 0, 0x0001|0x0004|0x0080|0x0400|0x0100 /*INVALIDATE|ERASE|ALLCHILDREN|FRAME|UPDATENOW*/)
}

func themeCombo(c uintptr, dark bool) {
	type cbInfo struct {
		Size               uint32
		Item, Button       RECT
		State              uint32
		Combo, Item2, List uintptr
	}
	ci := cbInfo{Size: uint32(unsafe.Sizeof(cbInfo{}))}
	pGetComboBoxInfo.Call(c, uintptr(unsafe.Pointer(&ci)))
	if dark {
		pSetWindowTheme.Call(c, u16p("DarkMode_CFD"), 0)
		if ci.List != 0 {
			pSetWindowTheme.Call(ci.List, u16p("DarkMode_Explorer"), 0)
		}
	} else {
		pSetWindowTheme.Call(c, 0, 0)
		if ci.List != 0 {
			pSetWindowTheme.Call(ci.List, 0, 0)
		}
	}
}

func shiftHeld() bool {
	r, _, _ := pGetAsyncKeyState.Call(0x10)
	return r&0x8000 != 0
}

// drawCheckBox paints an owner-drawn checkbox (box + label) in theme colours.
func drawCheckBox(d *DRAWITEMSTRUCT, text string, on, hover bool) {
	r := d.RcItem
	fillRect(d.HDC, r, colBg)
	bs := sc(16)
	top := r.Top + (r.Bottom-r.Top-bs)/2
	box := RECT{r.Left + sc(2), top, r.Left + sc(2) + bs, top + bs}
	border := mix(colText, colBg, 0.45)
	if hover {
		border = colBlue
	}
	if on {
		fillRect(d.HDC, box, colBlue)
		pen, _, _ := pCreatePen.Call(0, uintptr(sc(2)), contrastOn(colBlue))
		old, _, _ := pSelectObject.Call(d.HDC, pen)
		pts := []int32{
			box.Left + sc(4), box.Top + sc(8),
			box.Left + sc(7), box.Top + sc(11),
			box.Left + sc(12), box.Top + sc(5),
		}
		pPolyline.Call(d.HDC, uintptr(unsafe.Pointer(&pts[0])), 3)
		pSelectObject.Call(d.HDC, old)
		pDeleteObject.Call(pen)
	} else {
		fillRect(d.HDC, box, colBg)
		frameRect(d.HDC, box, border)
	}
	tr := RECT{box.Right + sc(8), r.Top, r.Right, r.Bottom}
	col := colText
	if d.ItemState&ODS_DISABLED != 0 {
		col = colDisabled
	}
	drawText(d.HDC, text, tr, fLabel, col, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	if d.ItemState&ODS_FOCUS != 0 && hover == false && kbFocusVisible {
		frameRect(d.HDC, RECT{tr.Left - sc(3), r.Top + sc(1), r.Right, r.Bottom - sc(1)}, colBorder)
	}
}

var kbFocusVisible = false
