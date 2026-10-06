//go:build windows

package main

import (
	"bytes"
	_ "embed"
	"image"
	"image/png"
	"syscall"
	"unsafe"
)

var (
	advapi32 = syscall.NewLazyDLL("advapi32.dll")

	pBeginPaint          = user32.NewProc("BeginPaint")
	pEndPaint            = user32.NewProc("EndPaint")
	pFillRect            = user32.NewProc("FillRect")
	pFrameRect           = user32.NewProc("FrameRect")
	pDrawTextW           = user32.NewProc("DrawTextW")
	pInvalidateRect      = user32.NewProc("InvalidateRect")
	pGetClientRect       = user32.NewProc("GetClientRect")
	pSetCursor           = user32.NewProc("SetCursor")
	pTrackMouseEvent     = user32.NewProc("TrackMouseEvent")
	pEnumDisplayDevicesW = user32.NewProc("EnumDisplayDevicesW")
	pAdjustWindowRectEx  = user32.NewProc("AdjustWindowRectEx")

	pSetTextColor           = gdi32.NewProc("SetTextColor")
	pSetBkMode              = gdi32.NewProc("SetBkMode")
	pSetBkColor             = gdi32.NewProc("SetBkColor")
	pSelectObject           = gdi32.NewProc("SelectObject")
	pCreateSolidBrush       = gdi32.NewProc("CreateSolidBrush")
	pCreatePen              = gdi32.NewProc("CreatePen")
	pDeleteObject           = gdi32.NewProc("DeleteObject")
	pCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	pCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	pDeleteDC               = gdi32.NewProc("DeleteDC")
	pBitBlt                 = gdi32.NewProc("BitBlt")
	pStretchBlt             = gdi32.NewProc("StretchBlt")
	pSetStretchBltMode      = gdi32.NewProc("SetStretchBltMode")
	pSetBrushOrgEx          = gdi32.NewProc("SetBrushOrgEx")
	pCreateDIBSection       = gdi32.NewProc("CreateDIBSection")
	pMoveToEx               = gdi32.NewProc("MoveToEx")
	pLineTo                 = gdi32.NewProc("LineTo")
	pRectangle              = gdi32.NewProc("Rectangle")

	pRegGetValueW = advapi32.NewProc("RegGetValueW")
)

const (
	WM_PAINT           = 0x000F
	WM_ERASEBKGND      = 0x0014
	WM_SETCURSOR       = 0x0020
	WM_DRAWITEM        = 0x002B
	WM_MOUSEMOVE       = 0x0200
	WM_LBUTTONDOWN     = 0x0201
	WM_LBUTTONUP       = 0x0202
	WM_MOUSELEAVE      = 0x02A3
	WM_CTLCOLORBTN     = 0x0135
	BS_OWNERDRAW       = 0x0000000B
	SS_RIGHT           = 0x0002
	SS_CENTERIMAGE     = 0x0200
	SS_NOTIFY          = 0x0100
	WS_CLIPCHILDREN    = 0x02000000
	DT_CENTER          = 0x01
	DT_RIGHT           = 0x02
	DT_VCENTER         = 0x04
	DT_SINGLELINE      = 0x20
	DT_LEFT            = 0x00
	DT_WORDBREAK       = 0x10
	DT_NOPREFIX        = 0x800
	ODS_SELECTED       = 0x0001
	ODS_DISABLED       = 0x0004
	ODS_FOCUS          = 0x0010
	TRANSPARENT        = 1
	HALFTONE           = 4
	SRCCOPY            = 0x00CC0020
	TME_LEAVE          = 0x2
	IDC_HAND           = 32649
	IDC_ARROW          = 32512
	HKEY_LOCAL_MACHINE = 0x80000002
	RRF_RT_REG_SZ      = 0x2
)

type PAINTSTRUCT struct {
	Hdc         uintptr
	FErase      int32
	RcPaint     RECT
	FRestore    int32
	FIncUpdate  int32
	RgbReserved [32]byte
}

type DRAWITEMSTRUCT struct {
	CtlType    uint32
	CtlID      uint32
	ItemID     uint32
	ItemAction uint32
	ItemState  uint32
	HwndItem   uintptr
	HDC        uintptr
	RcItem     RECT
	ItemData   uintptr
}

type TRACKMOUSEEVENT struct {
	CbSize      uint32
	DwFlags     uint32
	HwndTrack   uintptr
	DwHoverTime uint32
}

func rgb(r, g, b uint8) uintptr { return uintptr(r) | uintptr(g)<<8 | uintptr(b)<<16 }

func fillRect(dc uintptr, r RECT, color uintptr) {
	br, _, _ := pCreateSolidBrush.Call(color)
	pFillRect.Call(dc, uintptr(unsafe.Pointer(&r)), br)
	pDeleteObject.Call(br)
}

func frameRect(dc uintptr, r RECT, color uintptr) {
	br, _, _ := pCreateSolidBrush.Call(color)
	pFrameRect.Call(dc, uintptr(unsafe.Pointer(&r)), br)
	pDeleteObject.Call(br)
}

func drawText(dc uintptr, s string, r RECT, font uintptr, color uintptr, flags uintptr) {
	old, _, _ := pSelectObject.Call(dc, font)
	pSetTextColor.Call(dc, color)
	pSetBkMode.Call(dc, TRANSPARENT)
	p, _ := syscall.UTF16FromString(s)
	pDrawTextW.Call(dc, uintptr(unsafe.Pointer(&p[0])), uintptr(len(p)-1), uintptr(unsafe.Pointer(&r)), flags|DT_NOPREFIX)
	pSelectObject.Call(dc, old)
}

func hline(dc uintptr, x1, x2, y int32, color uintptr) {
	pen, _, _ := pCreatePen.Call(0, 1, color)
	old, _, _ := pSelectObject.Call(dc, pen)
	pMoveToEx.Call(dc, uintptr(x1), uintptr(y), 0)
	pLineTo.Call(dc, uintptr(x2), uintptr(y))
	pSelectObject.Call(dc, old)
	pDeleteObject.Call(pen)
}

//go:embed banner.png
var bannerPNG []byte

var bannerBmp, bannerW, bannerH uintptr

func loadBanner() {
	img, err := png.Decode(bytes.NewReader(bannerPNG))
	if err != nil {
		return
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	type BITMAPINFOHEADER struct {
		Size          uint32
		Width, Height int32
		Planes, Bits  uint16
		Compression   uint32
		SizeImage     uint32
		XPPM, YPPM    int32
		ClrUsed       uint32
		ClrImportant  uint32
	}
	bi := BITMAPINFOHEADER{Size: 40, Width: int32(w), Height: -int32(h), Planes: 1, Bits: 32}
	var bits uintptr
	hb, _, _ := pCreateDIBSection.Call(0, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if hb == 0 || bits == 0 {
		return
	}
	px := unsafe.Slice((*byte)(unsafe.Pointer(bits)), w*h*4)
	rgba, ok := img.(*image.RGBA)
	nrgba, ok2 := img.(*image.NRGBA)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var r, g, bb uint8
			switch {
			case ok:
				o := rgba.PixOffset(x+b.Min.X, y+b.Min.Y)
				r, g, bb = rgba.Pix[o], rgba.Pix[o+1], rgba.Pix[o+2]
			case ok2:
				o := nrgba.PixOffset(x+b.Min.X, y+b.Min.Y)
				r, g, bb = nrgba.Pix[o], nrgba.Pix[o+1], nrgba.Pix[o+2]
			default:
				cr, cg, cb, _ := img.At(x+b.Min.X, y+b.Min.Y).RGBA()
				r, g, bb = uint8(cr>>8), uint8(cg>>8), uint8(cb>>8)
			}
			i := (y*w + x) * 4
			px[i], px[i+1], px[i+2], px[i+3] = bb, g, r, 255
		}
	}
	bannerBmp, bannerW, bannerH = hb, uintptr(w), uintptr(h)
}

func drawBanner(dc uintptr, r RECT, hover bool) {
	if bannerBmp == 0 {
		fillRect(dc, r, rgb(40, 0, 0))
		return
	}
	mdc, _, _ := pCreateCompatibleDC.Call(dc)
	old, _, _ := pSelectObject.Call(mdc, bannerBmp)
	pSetStretchBltMode.Call(dc, HALFTONE)
	pSetBrushOrgEx.Call(dc, 0, 0, 0)
	pStretchBlt.Call(dc, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top),
		mdc, 0, 0, bannerW, bannerH, SRCCOPY)
	pSelectObject.Call(mdc, old)
	pDeleteDC.Call(mdc)
	if hover {
		for i := int32(0); i < sc(3); i++ {
			frameRect(dc, RECT{r.Left + i, r.Top + i, r.Right - i, r.Bottom - i}, rgb(255, 140, 30))
		}
	}
}

func regString(root uintptr, path, name string) string {
	buf := make([]uint16, 512)
	n := uint32(len(buf) * 2)
	r, _, _ := pRegGetValueW.Call(root, u16p(path), u16p(name), RRF_RT_REG_SZ, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n)))
	if r != 0 {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

func gpuName() string {
	var dd [840]byte
	*(*uint32)(unsafe.Pointer(&dd[0])) = 840
	first := ""
	for i := 0; i < 16; i++ {
		r, _, _ := pEnumDisplayDevicesW.Call(0, uintptr(i), uintptr(unsafe.Pointer(&dd[0])), 0)
		if r == 0 {
			break
		}
		name := syscall.UTF16ToString(unsafe.Slice((*uint16)(unsafe.Pointer(&dd[68])), 128))
		flags := *(*uint32)(unsafe.Pointer(&dd[324]))
		if first == "" {
			first = name
		}
		if flags&4 != 0 {
			return name
		}
	}
	return first
}
