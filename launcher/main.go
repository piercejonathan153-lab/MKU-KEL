//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"
)

const appTitle = "Mortal Kombat: Ultimate Komplete Edition Launcher"
const appVersion = "1.3.1"
const wndClass = "MKUKELauncherWnd"

var (
	exeDir, optPath, setPath, confPath string
	opts                               *Options
	settings                           Settings
	resList                            [][2]int
	hMain                              uintptr
	launchGame                         bool
	dpiScale                           = 1.0
	dirty                              bool
	curTab                             int
	bannerHover                        bool
	cpuText, gpuText                   string
	gpuColor                           uintptr
	hoverBtn                           uintptr
	hoverTab                           = -1
	coalPath                           string

	fTab, fHead, fBtn, fLabel, fSmall, fFoot, fBig uintptr

	ctl        = map[int]uintptr{}
	pageCtls   = map[int][]uintptr{}
	headerCtls = map[uintptr]bool{}
	hintCtls   = map[uintptr]bool{}
	ownerBtns  = map[int]string{}
	building   = -1
)

var colBlue, colText, colBorder, colGray uintptr

var tabNames = []string{"DISPLAY", "GRAPHICS", "EXTRA", "TOOLS", "SETTINGS", "ABOUT"}

// control ids
const (
	idRes = 200 + iota
	idMode
	idRenderer
	idVSync
	idFPS
	idLetter
	idPresetVanilla
	idPresetOpt
	idPresetUltra
	idPresetPotato
	idPresetConsole
	idPresetMax
	idHintDisplay
	idTexture
	idShadow
	idAniso
	idMemory
	idForceAA
	idForceAF
	idHintGfx
	idSkipIntro
	idMonitor
	idHDTex
	idDistance
	idOpenLogs
	idManual
	idRyzen
	idWatermark
	idOpenCfg
	idOpenGame
	idReset
	idOriginal
	idApply
	idTheme
	idTooltips
	idRememberTab
	idAskSave
	idConfirmReset
	idAfterLaunch
	idQuickLaunch
	idLogDays
	idAccent
	idTextColor
	idSkipLogo
	idHighPriority
	idDisableFSO
	idSaveProfile
	idLoadProfile
	idOpenProfiles
	idDLCManager
	idModList
	idOpenMods
	idModSummary
	idDiagnostics
	idRestoreVanilla
	idCheckUpdates
	idUpdateCheck
)

var (
	themeNames       []string
	afterLaunchNames = []string{"Close launcher", "Reopen when game closes"}
	logDayNames      = []string{"1 day", "3 days", "7 days", "30 days"}
	checkState       = map[int]bool{}
)

var (
	modeNames     = []string{"Fullscreen", "Borderless", "Windowed"}
	rendererNames = []string{"DirectX 9", "DirectX 11", "DirectX 12", "Vulkan (DXVK)"}
	fpsNames      = []string{"Default (60)", "60 (dgVoodoo)", "45 (slow-mo)", "30 (slow-mo)"}
	fpsValues     = []int{0, 60, 45, 30}
	onOff         = []string{"Enabled", "Disabled"}
	texNames      = []string{"Medium", "High", "Very High"}
	texValues     = []int{1024, 2048, 4096}
	shadowNames   = []string{"Low", "Medium", "High", "Very High"}
	shadowValues  = []int{512, 1024, 2048, 4096}
	anisoNames    = []string{"Off", "2x", "4x", "8x", "16x"}
	anisoValues   = []int{1, 2, 4, 8, 16}
	memNames      = []string{"Standard", "Unlimited"}
	forceAANames  = []string{"Game Controlled", "Off", "2x MSAA", "4x MSAA", "8x MSAA"}
	forceAAValues = []string{"appdriven", "off", "2x", "4x", "8x"}
	forceAFNames  = []string{"Game Controlled", "Bilinear", "Trilinear", "2x Anisotropic", "4x Anisotropic", "8x Anisotropic", "16x Anisotropic"}
	forceAFValues = []string{"appdriven", "bilinear", "trilinear", "2", "4", "8", "16"}
	introMovies   = []string{"WBIntroLogo.bik", "NRIntroLogo.bik", "HVSThunderLogoFIX.bik"}
	distanceNames = []string{"Nearby", "Far", "Worldwide"}
)

func sc(v int) int32 { return int32(float64(v)*dpiScale + 0.5) }

func main() {
	runtime.LockOSThread()
	pSetProcessDPIAware.Call()
	if pGetDpiForSystem.Find() == nil {
		if d, _, _ := pGetDpiForSystem.Call(); d > 0 {
			dpiScale = float64(d) / 96.0
		}
	}
	ex, _ := os.Executable()
	exeDir = filepath.Dir(ex)
	optPath = filepath.Join(os.Getenv("APPDATA"), "MKKE", "options.ini")
	setPath = filepath.Join(exeDir, "MKKE_Launcher.ini")
	confPath = filepath.Join(exeDir, "dgVoodoo.conf")

	if _, err := os.Stat(filepath.Join(exeDir, "MKKE.exe")); err != nil {
		msgBox(0, "MKKE.exe was not found next to this launcher.\n\nPut the launcher in the DiscContentPC folder.", appTitle, MB_ICONERROR)
		return
	}
	if alreadyRunning(wndClass) {
		return
	}
	coalPath = filepath.Join(exeDir, "Config", "coalesced.ini")
	firstRun := !exists(setPath)
	settings = LoadSettings(setPath)
	initLog()
	os.MkdirAll(filepath.Dir(optPath), 0755)
	opts = LoadOptions(optPath)
	prepareRendererFiles()
	enumMonitors()
	if len(monitors) == 0 {
		w, h := nativeRes()
		monitors = []Monitor{{Device: "", Name: fmt.Sprintf("Display 1  (%dx%d) *", w, h), W: int32(w), H: int32(h), Primary: true}}
	}
	settings.Monitor = clamp(settings.Monitor, 0, len(monitors)-1)
	mon := monitors[settings.Monitor]
	buildResList(mon.Device, int(mon.W), int(mon.H))
	loadBanner()
	cpuText = strings.ToUpper(strings.TrimSpace(regString(HKEY_LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\CentralProcessor\0`, "ProcessorNameString")))
	gpuText = strings.ToUpper(strings.TrimSpace(gpuName()))
	switch {
	case strings.Contains(gpuText, "NVIDIA"):
		gpuColor = rgb(80, 150, 30)
	case strings.Contains(gpuText, "AMD"), strings.Contains(gpuText, "RADEON"):
		gpuColor = rgb(200, 40, 40)
	default:
		gpuColor = rgb(30, 100, 200)
	}
	logf("CPU: %s | GPU: %s | monitors: %d", cpuText, gpuText, len(monitors))

	noQuick := false
	for _, a := range os.Args[1:] {
		if strings.EqualFold(a, "--no-quick") {
			noQuick = true
		}
	}
	quick := settings.QuickLaunch && !noQuick && !shiftHeld()
	if firstRun {
		quick = false
	}
	createUI(!quick)
	if firstRun {
		doFirstRun()
	} else if !quick && settings.CheckUpdates {
		checkUpdates(false)
	}
	if quick {
		if w, _ := findGameWindow(); w == 0 {
			if err := applyAll(); err == nil {
				logf("quick launch")
				pDestroyWindow.Call(hMain)
				runGame()
				return
			} else {
				msgBox(0, "Quick launch failed, opening the launcher instead:\n\n"+err.Error(), appTitle, MB_ICONWARNING)
			}
		}
		pShowWindow.Call(hMain, SW_SHOW)
	}
	var m MSG
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		if d, _, _ := pIsDialogMessageW.Call(hMain, uintptr(unsafe.Pointer(&m))); d != 0 {
			continue
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
	if launchGame {
		runGame()
	}
}

// ------------------------------------------------------------------ layout

const clientW, clientH = 560, 676

func tabRect(i int) RECT {
	x := 10 + i*90
	return RECT{sc(x), sc(8), sc(x + 86), sc(46)}
}
func panelRect() RECT  { return RECT{sc(10), sc(54), sc(550), sc(404)} }
func bannerRect() RECT { return RECT{sc(12), sc(466), sc(548), sc(645)} }

func mkFont(face string, px int, weight int) uintptr {
	f, _, _ := pCreateFontW.Call(uintptr(int32(-sc(px))), 0, 0, 0, uintptr(weight), 0, 0, 0, 1, 0, 0, 5, 0, u16p(face))
	return f
}

func createUI(show bool) {
	hInst, _, _ := pGetModuleHandleW.Call(0)
	cursor, _, _ := pLoadCursorW.Call(0, IDC_ARROW)
	icon, _, _ := pExtractIconW.Call(hInst, u16p(filepath.Join(exeDir, "MKKE.exe")), 0)
	setThemeColors(wantDark())
	cls := u16(wndClass)
	wc := WNDCLASSEXW{
		CbSize: uint32(unsafe.Sizeof(WNDCLASSEXW{})), LpfnWndProc: syscall.NewCallback(wndProc),
		HInstance: hInst, HIcon: icon, HIconSm: icon, HCursor: cursor, HbrBackground: bgBrush, LpszClassName: cls,
	}
	pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	fTab = mkFont("Bahnschrift SemiBold Condensed", 20, 600)
	fHead = mkFont("Bahnschrift SemiBold Condensed", 18, 600)
	fBtn = mkFont("Bahnschrift SemiBold Condensed", 19, 600)
	fBig = mkFont("Bahnschrift SemiBold Condensed", 26, 600)
	fFoot = mkFont("Bahnschrift SemiBold Condensed", 14, 600)
	fLabel = mkFont("Segoe UI Semibold", 13, 600)
	fSmall = mkFont("Segoe UI", 13, 400)

	style := uintptr(WS_OVERLAPPED | WS_CAPTION | WS_SYSMENU | WS_MINIMIZEBOX | WS_CLIPCHILDREN)
	wr := RECT{0, 0, sc(clientW), sc(clientH)}
	pAdjustWindowRectEx.Call(uintptr(unsafe.Pointer(&wr)), style, 0, WS_EX_CONTROLPARENT)
	w, h := wr.Right-wr.Left, wr.Bottom-wr.Top
	sw, _, _ := pGetSystemMetrics.Call(0)
	shh, _, _ := pGetSystemMetrics.Call(1)
	hMain, _, _ = pCreateWindowExW.Call(WS_EX_CONTROLPARENT, uintptr(unsafe.Pointer(cls)), u16p(appTitle), style,
		uintptr((int32(sw)-w)/2), uintptr((int32(shh)-h)/2), uintptr(w), uintptr(h), 0, 0, hInst, 0)

	buildDisplayPage()
	buildGraphicsPage()
	buildExtraPage()
	buildToolsPage()
	buildSettingsPage()
	buildAboutPage()

	building = -1
	ownerButton(idReset, "RESET DISPLAY", 12, 414, 172, 44)
	ownerButton(idOriginal, "ORIGINAL LAUNCHER", 194, 414, 172, 44)
	ownerButton(idApply, "APPLY SETTINGS", 376, 414, 172, 44)

	initTooltips()
	addAllTips()
	addRectTip(bannerRect(), "Click to apply your settings and start Mortal Kombat.\r\nShortcut: Enter starts the game, Esc closes the launcher.")
	start := 0
	if settings.RememberTab {
		start = clamp(settings.LastTab, 0, len(tabNames)-1)
	}
	showTab(start)
	setDirty(false)
	updateHints()
	applyTheme()
	if show {
		pShowWindow.Call(hMain, SW_SHOW)
		pUpdateWindow.Call(hMain)
	}
}

// ------------------------------------------------------------------ control helpers

func addCtl(id int, h uintptr) uintptr {
	if id != 0 {
		ctl[id] = h
	}
	if building >= 0 {
		pageCtls[building] = append(pageCtls[building], h)
	}
	return h
}

func child(class, text string, style uintptr, x, y, w, h int, id int, font uintptr) uintptr {
	hInst, _, _ := pGetModuleHandleW.Call(0)
	if building < 0 {
		style |= WS_VISIBLE
	}
	c, _, _ := pCreateWindowExW.Call(0, u16p(class), u16p(text), WS_CHILD|style,
		uintptr(sc(x)), uintptr(sc(y)), uintptr(sc(w)), uintptr(sc(h)), hMain, uintptr(id), hInst, 0)
	sendMsg(c, WM_SETFONT, font, 1)
	return addCtl(id, c)
}

func header(text string, x, y int) {
	h := child("STATIC", text, SS_LEFT, x, y, 510, 24, 0, fHead)
	headerCtls[h] = true
}

var labelFor = map[int]uintptr{}

func label(text string, x, y, w int) uintptr {
	return child("STATIC", text, SS_RIGHT|SS_CENTERIMAGE|SS_NOTIFY, x, y, w, 26, 0, fLabel)
}

func comboBox(id, x, y, w int, items []string, sel int) {
	c := child("COMBOBOX", "", CBS_DROPDOWNLIST|0x0010 /*CBS_OWNERDRAWFIXED*/ |0x0200 /*CBS_HASSTRINGS*/ |WS_TABSTOP|WS_VSCROLL, x, y, w, 320, id, fSmall)
	comboCtls = append(comboCtls, c)
	subclassCombo(c)
	for _, s := range items {
		sendMsg(c, CB_ADDSTRING, 0, u16p(s))
	}
	sendMsg(c, CB_SETCURSEL, uintptr(clamp(sel, 0, len(items)-1)), 0)
}

func checkBox(id int, text string, x, y, w int, on bool) {
	ownerBtns[id] = text
	checkState[id] = on
	child("BUTTON", text, BS_OWNERDRAW|WS_TABSTOP, x, y, w, 24, id, fLabel)
}

func ownerButton(id int, text string, x, y, w, h int) {
	ownerBtns[id] = text
	child("BUTTON", text, BS_OWNERDRAW|WS_TABSTOP, x, y, w, h, id, fBtn)
}

var lastHint uintptr

func setText0(h uintptr, s string) { pSetWindowTextW.Call(h, u16p(s)) }

func hint(id int, x, y, w, h int) {
	c := child("STATIC", "", SS_LEFT, x, y, w, h, id, fSmall)
	lastHint = c
	hintCtls[c] = true
}

func row2(lLabel string, lid int, lItems []string, lSel int, rLabel string, rid int, rItems []string, rSel int, y int) {
	if lLabel != "" {
		labelFor[lid] = label(lLabel, 16, y, 112)
		comboBox(lid, 136, y, 140, lItems, lSel)
	}
	if rLabel != "" {
		labelFor[rid] = label(rLabel, 286, y, 118)
		comboBox(rid, 412, y, 128, rItems, rSel)
	}
}

func idxOf(vals []int, v, def int) int {
	for i, x := range vals {
		if x == v {
			return i
		}
	}
	return def
}

func b2sel(b bool) int {
	if b {
		return 0
	}
	return 1
}

// ------------------------------------------------------------------ pages

func resItems() ([]string, int) {
	var rl []string
	cur := -1
	m := monitors[clamp(settings.Monitor, 0, len(monitors)-1)]
	for i, p := range resList {
		rl = append(rl, resLabel(p))
		if p[0] == opts.ResX && p[1] == opts.ResY {
			cur = i
		}
	}
	if cur < 0 {
		for i, p := range resList {
			if p[0] == int(m.W) && p[1] == int(m.H) {
				cur = i
			}
		}
	}
	return rl, cur
}

func buildDisplayPage() {
	building = 0
	header("BASIC", 22, 64)
	var mn []string
	for _, m := range monitors {
		mn = append(mn, m.Name)
	}
	rl, cur := resItems()
	row2("Monitor", idMonitor, mn, settings.Monitor, "Vertical Sync", idVSync, onOff, b2sel(settings.VSync), 96)
	sendMsg(ctl[idMonitor], 0x0160, uintptr(sc(200)), 0)
	row2("Resolution", idRes, rl, cur, "Framerate Limit", idFPS, fpsNames, settings.FPS, 134)
	sendMsg(ctl[idRes], 0x0160 /*CB_SETDROPPEDWIDTH*/, uintptr(sc(170)), 0)
	row2("Display Mode", idMode, modeNames, settings.Mode, "Letterboxing", idLetter, onOff, b2sel(opts.Letterbox), 172)
	row2("Renderer", idRenderer, rendererNames, settings.Renderer, "", 0, nil, 0, 210)

	header("GRAPHICS PRESETS", 22, 250)
	ownerButton(idPresetPotato, "POTATO", 20, 278, 168, 34)
	ownerButton(idPresetConsole, "CONSOLE", 196, 278, 168, 34)
	ownerButton(idPresetVanilla, "VANILLA", 372, 278, 168, 34)
	ownerButton(idPresetOpt, "OPTIMIZED", 20, 318, 168, 34)
	ownerButton(idPresetUltra, "ULTRA", 196, 318, 168, 34)
	ownerButton(idPresetMax, "MAXIMUM", 372, 318, 168, 34)
	hint(idHintDisplay, 22, 360, 516, 40)
}

func rebuildResCombo() {
	m := monitors[clamp(sel(idMonitor), 0, len(monitors)-1)]
	settings.Monitor = sel(idMonitor)
	buildResList(m.Device, int(m.W), int(m.H))
	opts.ResX, opts.ResY = int(m.W), int(m.H)
	rl, cur := resItems()
	c := ctl[idRes]
	sendMsg(c, CB_RESETCONTENT, 0, 0)
	for _, s := range rl {
		sendMsg(c, CB_ADDSTRING, 0, u16p(s))
	}
	sendMsg(c, CB_SETCURSEL, uintptr(clamp(cur, 0, len(rl)-1)), 0)
}

func buildGraphicsPage() {
	building = 1
	header("QUALITY", 22, 64)
	row2("Texture Quality", idTexture, texNames, idxOf(texValues, opts.GetInt("max_texture", 2048), 1),
		"Anisotropic Filter", idAniso, anisoNames, idxOf(anisoValues, opts.GetInt("anisotropy", 16), 4), 96)
	row2("Shadow Quality", idShadow, shadowNames, idxOf(shadowValues, opts.GetInt("shadow_size", 2048), 2),
		"Video Memory", idMemory, memNames, opts.GetInt("unlimited_memory", 0), 134)

	header("RENDERER OVERRIDES  (DX11 / DX12 / VULKAN)", 22, 184)
	row2("Anti-Aliasing", idForceAA, forceAANames, settings.ForceAA, "Texture Filter", idForceAF, forceAFNames, settings.ForceAF, 216)
	hint(idHintGfx, 22, 266, 516, 130)
}

func buildExtraPage() {
	building = 2
	header("STARTUP", 22, 64)
	checkBox(idSkipIntro, "Skip intro movies (WB Games, NetherRealm, THX)", 26, 92, 510, settings.SkipIntro)
	header("ONLINE", 22, 130)
	row2("Matchmaking", idDistance, distanceNames, clamp(opts.GetInt("distance_filter", 0), 0, 2), "", 0, nil, 0, 160)
	header("COMPATIBILITY", 22, 206)
	checkBox(idRyzen, "Ryzen freeze fix", 26, 234, 250, settings.Ryzen)
	checkBox(idHighPriority, "High CPU priority", 286, 234, 254, settings.HighPriority)
	checkBox(idWatermark, "Show renderer overlay", 26, 260, 250, settings.Watermark)
	checkBox(idDisableFSO, "Disable fullscreen optimizations", 286, 260, 254, settings.DisableFSO)
	hint(0, 22, 302, 516, 90)
	setText0(lastHint, "Hover over any option for a full explanation. Changes here are saved with APPLY SETTINGS or START GAME.")
}

func buildToolsPage() {
	building = 3
	header("PROFILES", 22, 64)
	ownerButton(idSaveProfile, "SAVE PROFILE", 20, 92, 168, 32)
	ownerButton(idLoadProfile, "LOAD PROFILE", 196, 92, 168, 32)
	ownerButton(idOpenProfiles, "PROFILES FOLDER", 372, 92, 168, 32)
	header("MODS", 22, 138)
	ownerButton(idDLCManager, "DLC MANAGER", 20, 166, 168, 32)
	ownerButton(idModList, "INSTALLED MODS", 196, 166, 168, 32)
	ownerButton(idOpenMods, "MODS FOLDER", 372, 166, 168, 32)
	hint(idModSummary, 22, 204, 516, 20)
	header("FOLDERS", 22, 232)
	ownerButton(idOpenCfg, "SETTINGS FOLDER", 20, 260, 168, 32)
	ownerButton(idOpenGame, "GAME FOLDER", 196, 260, 168, 32)
	ownerButton(idOpenLogs, "LAUNCHER LOGS", 372, 260, 168, 32)
	header("MAINTENANCE", 22, 306)
	ownerButton(idDiagnostics, "DIAGNOSTICS", 20, 334, 168, 32)
	ownerButton(idManual, "MANUAL MODE", 196, 334, 168, 32)
	ownerButton(idRestoreVanilla, "RESTORE VANILLA", 372, 334, 168, 32)
}

func buildSettingsPage() {
	building = 4
	themeNames = nil
	for _, t := range themes {
		themeNames = append(themeNames, t.Name)
	}
	var an, tn []string
	for _, c := range accentColors {
		an = append(an, c.Name)
	}
	for _, c := range textColors {
		tn = append(tn, c.Name)
	}
	header("APPEARANCE", 22, 64)
	row2("Theme", idTheme, themeNames, clamp(settings.Theme, 0, len(themes)-1), "Accent Color", idAccent, an, clamp(settings.Accent, 0, len(an)-1), 94)
	sendMsg(ctl[idTheme], 0x0160, uintptr(sc(180)), 0)
	row2("Text Color", idTextColor, tn, clamp(settings.TextColor, 0, len(tn)-1), "", 0, nil, 0, 132)
	header("INTERFACE", 22, 174)
	checkBox(idTooltips, "Show tooltips", 26, 202, 250, settings.Tooltips)
	checkBox(idRememberTab, "Open on the last tab I used", 286, 202, 254, settings.RememberTab)
	checkBox(idAskSave, "Ask to save when closing", 26, 228, 250, settings.AskSave)
	checkBox(idConfirmReset, "Ask before display reset", 286, 228, 254, settings.ConfirmReset)
	header("LAUNCHING", 22, 266)
	row2("After Launch", idAfterLaunch, afterLaunchNames, clamp(settings.AfterLaunch, 0, 1), "Keep Logs For", idLogDays, logDayNames, clamp(settings.LogDays, 0, 3), 296)
	sendMsg(ctl[idAfterLaunch], 0x0160, uintptr(sc(190)), 0)
	checkBox(idQuickLaunch, "Quick launch: skip this window next time (hold Shift to open it)", 26, 334, 510, settings.QuickLaunch)
	checkBox(idCheckUpdates, "Check for launcher updates at startup", 26, 360, 510, settings.CheckUpdates)
	h := child("STATIC", "Changes here save automatically.", SS_LEFT, 290, 136, 250, 20, 0, fSmall)
	hintCtls[h] = true
}

func buildAboutPage() {
	building = 5
	h := child("STATIC", "MORTAL KOMBAT: ULTIMATE KOMPLETE EDITION", SS_LEFT, 22, 66, 516, 32, 0, fBig)
	headerCtls[h] = true
	child("STATIC", "Launcher version "+appVersion, SS_LEFT, 22, 100, 516, 22, 0, fLabel)
	child("STATIC",
		"A modern launcher for Mortal Kombat Komplete Edition (PC) with Vulkan and DirectX 11 / 12 support, "+
			"borderless fullscreen, ultrawide resolutions, frame-rate and VSync options.\r\n\r\n"+
			"Vulkan rendering: DXVK by Philip Rebohle and contributors (zlib license).\r\n"+
			"DirectX 11 / 12 rendering: dgVoodoo2 by Dege.\r\n"+
			"Windowed mode: patched MKKE.exe (fixes the game ignoring its windowed setting).\r\n"+
			"Design inspired by BmLauncher by neatodev (CC BY-NC-SA 4.0).\r\n\r\n"+
			"Mortal Kombat is a trademark of Warner Bros. Entertainment Inc. This launcher is an unofficial fan tool.",
		SS_LEFT, 22, 136, 516, 210, 0, fSmall)
	ownerButton(idUpdateCheck, "CHECK FOR UPDATES", 340, 356, 200, 34)
}

func showTab(i int) {
	curTab = i
	if i == 3 {
		setText(idModSummary, modSummary()+"  Use DLC MANAGER to add or remove character, costume and arena mods.")
	}
	if settings.LastTab != i {
		settings.LastTab = i
		settings.Save(setPath)
	}
	for p, list := range pageCtls {
		for _, h := range list {
			if p == i {
				pShowWindow.Call(h, SW_SHOW)
			} else {
				pShowWindow.Call(h, SW_HIDE)
			}
		}
	}
	r := RECT{0, 0, sc(clientW), sc(54)}
	pInvalidateRect.Call(hMain, uintptr(unsafe.Pointer(&r)), 1)
}

// ------------------------------------------------------------------ state

func sel(id int) int      { return int(int32(sendMsg(ctl[id], CB_GETCURSEL, 0, 0))) }
func checked(id int) bool { return checkState[id] }
func setSel(id, v int)    { sendMsg(ctl[id], CB_SETCURSEL, uintptr(v), 0) }

func enable(id int, on bool) {
	v := uintptr(0)
	if on {
		v = 1
	}
	pEnableWindow.Call(ctl[id], v)
}

var (
	pSetTimer  = user32.NewProc("SetTimer")
	pKillTimer = user32.NewProc("KillTimer")
	savedFlash bool
)

const WM_TIMER = 0x0113

func flashSaved() {
	savedFlash = true
	pInvalidateRect.Call(ctl[idApply], 0, 1)
	pSetTimer.Call(hMain, 1, 1600, 0)
}

func setDirty(d bool) {
	dirty = d
	enable(idApply, d)
	pInvalidateRect.Call(ctl[idApply], 0, 1)
}

func setText(id int, s string) { pSetWindowTextW.Call(ctl[id], u16p(s)) }

func updateHints() {
	r := sel(idRenderer)
	dx9 := r == 0
	for _, id := range []int{idFPS, idVSync, idForceAA, idForceAF, idWatermark} {
		enable(id, !dx9)
	}
	if r == rendVulkan {
		enable(idForceAA, false)
	}
	for _, c := range comboCtls {
		pInvalidateRect.Call(c, 0, 0)
	}
	var d []string
	if (r == 1 || r == 2) && !dgvAvailable() {
		d = append(d, "dgVoodoo2 (D3D9.dgvoodoo.dll + dgVoodoo.conf) is missing from the game folder, so DirectX 11 / 12 won't work.")
	}
	if r == rendVulkan && !dxvkAvailable() {
		d = append(d, "DXVK (D3D9.dxvk.dll) is missing from the game folder, so Vulkan won't work.")
	}
	if dx9 {
		d = append(d, "DirectX 9 is the game's original renderer. Framerate limit, VSync and the overrides need DirectX 11 / 12 or Vulkan.")
	}
	if r == 1 || r == 2 {
		d = append(d, "Known issue: with DirectX 11 / 12 some God of War content (Kratos, his stage) renders black. Vulkan doesn't have this problem.")
	}
	if sel(idFPS) >= 2 && !dx9 {
		d = append(d, "The game's speed is tied to its frame rate: below 60 FPS, fights run in slow motion.")
	}
	if len(d) == 0 {
		d = append(d, "Recommended: Borderless + Vulkan with Vertical Sync on.\r\nTip: press Enter to start the game, or Esc to close the launcher.")
	}
	setText(idHintDisplay, strings.Join(d, "\r\n\r\n"))
	setText(idHintGfx, "Presets on the DISPLAY tab set these options.\r\n\r\n"+
		"The overrides force anti-aliasing or filtering on top of the game's own settings. "+
		"Leave them on Game Controlled unless edges or textures look rough.\r\n"+
		"Vulkan supports the texture filter override; forced anti-aliasing needs DirectX 11 / 12.")
}

// preset values: texture, shadow, aniso, memory, forceAA, forceAF (combo indexes)
var presetTable = map[int][6]int{
	idPresetPotato:  {0, 0, 0, 0, 1, 1}, // medium tex, 512 shadows, no aniso, AA off, bilinear
	idPresetConsole: {0, 1, 1, 0, 0, 2}, // PS3 / 360 look: medium tex, 1024 shadows, 2x, trilinear
	idPresetVanilla: {1, 1, 2, 0, 0, 0},
	idPresetOpt:     {2, 2, 4, 1, 0, 0},
	idPresetUltra:   {2, 3, 4, 1, 3, 6},
	idPresetMax:     {2, 3, 4, 1, 4, 6}, // everything maxed + 8x MSAA
}

func applyPreset(id int) {
	v := presetTable[id]
	for i, c := range []int{idTexture, idShadow, idAniso, idMemory, idForceAA, idForceAF} {
		setSel(c, v[i])
	}
	if id == idPresetMax && sel(idRenderer) == 0 {
		setSel(idRenderer, preferredRenderer())
	}
	logf("preset %d applied", id)
	updateHints()
	setDirty(true)
}

func resetDisplay() {
	nw, nh := nativeRes()
	for i, p := range resList {
		if p[0] == nw && p[1] == nh {
			setSel(idRes, i)
		}
	}
	setSel(idMode, 1)
	setSel(idRenderer, preferredRenderer())
	setSel(idVSync, 0)
	setSel(idFPS, 0)
	setSel(idLetter, 0)
	updateHints()
	setDirty(true)
}

// ------------------------------------------------------------------ window procedure

func wndProc(h, msg, wp, lp uintptr) uintptr {
	switch msg {
	case WM_ERASEBKGND:
		return 1
	case WM_PAINT:
		paint(h)
		return 0
	case WM_CTLCOLORSTATIC, WM_CTLCOLORBTN, WM_CTLCOLORLISTBOX:
		dc := wp
		pSetBkColor.Call(dc, colBg)
		switch {
		case headerCtls[lp]:
			pSetTextColor.Call(dc, colBlue)
		case hintCtls[lp]:
			pSetTextColor.Call(dc, colHint)
		default:
			pSetTextColor.Call(dc, colText)
		}
		return bgBrush
	case WM_SETTINGCHANGE:
		if settings.Theme == 2 && lp != 0 && wstr(lp) == "ImmersiveColorSet" {
			applyTheme()
		}
	case WM_DRAWITEM:
		d := (*DRAWITEMSTRUCT)(unsafe.Pointer(lp))
		if d.CtlType == ODT_COMBOBOX {
			drawComboItem(d)
		} else {
			drawOwnerButton(d)
		}
		return 1
	case 0x002C: // WM_MEASUREITEM
		type mis struct{ CtlType, CtlID, ItemID, ItemWidth, ItemHeight uint32 }
		m := (*mis)(unsafe.Pointer(lp))
		if m.CtlType == ODT_COMBOBOX {
			m.ItemHeight = uint32(sc(20))
		}
		return 1
	case WM_SETCURSOR:
		nb := uintptr(0)
		if wp != h {
			for id := range ownerBtns {
				if ctl[id] == wp {
					nb = wp
				}
			}
		}
		if nb != hoverBtn {
			old := hoverBtn
			hoverBtn = nb
			if old != 0 {
				pInvalidateRect.Call(old, 0, 0)
			}
			if nb != 0 {
				pInvalidateRect.Call(nb, 0, 0)
			}
		}
		if bannerHover {
			c, _, _ := pLoadCursorW.Call(0, IDC_HAND)
			pSetCursor.Call(c)
			return 1
		}
	case WM_MOUSEMOVE:
		x, y := int32(int16(lp&0xFFFF)), int32(int16((lp>>16)&0xFFFF))
		b := bannerRect()
		over := x >= b.Left && x < b.Right && y >= b.Top && y < b.Bottom
		ht := -1
		for i := range tabNames {
			r := tabRect(i)
			if x >= r.Left && x < r.Right && y >= r.Top && y < r.Bottom {
				ht = i
			}
		}
		if ht != hoverTab {
			hoverTab = ht
			tr := RECT{0, 0, sc(clientW), sc(54)}
			pInvalidateRect.Call(h, uintptr(unsafe.Pointer(&tr)), 0)
		}
		if over != bannerHover {
			bannerHover = over
			pInvalidateRect.Call(h, uintptr(unsafe.Pointer(&b)), 0)
		}
		tme := TRACKMOUSEEVENT{CbSize: uint32(unsafe.Sizeof(TRACKMOUSEEVENT{})), DwFlags: TME_LEAVE, HwndTrack: h}
		pTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
		return 0
	case WM_MOUSELEAVE:
		if hoverTab != -1 {
			hoverTab = -1
			tr := RECT{0, 0, sc(clientW), sc(54)}
			pInvalidateRect.Call(h, uintptr(unsafe.Pointer(&tr)), 0)
		}
		if bannerHover {
			bannerHover = false
			b := bannerRect()
			pInvalidateRect.Call(h, uintptr(unsafe.Pointer(&b)), 0)
		}
		return 0
	case WM_LBUTTONUP:
		x, y := int32(int16(lp&0xFFFF)), int32(int16((lp>>16)&0xFFFF))
		for i := range tabNames {
			r := tabRect(i)
			if x >= r.Left && x < r.Right && y >= r.Top && y < r.Bottom {
				showTab(i)
				return 0
			}
		}
		b := bannerRect()
		if x >= b.Left && x < b.Right && y >= b.Top && y < b.Bottom {
			startGame(h)
		}
		return 0
	case WM_APP_WELCOME:
		showWelcome()
		return 0
	case WM_APP_UPDATE:
		showUpdateResult()
		return 0
	case WM_TIMER:
		if wp == 1 {
			pKillTimer.Call(h, 1)
			savedFlash = false
			pInvalidateRect.Call(ctl[idApply], 0, 1)
		}
		return 0
	case WM_COMMAND:
		id := int(wp & 0xFFFF)
		code := (wp >> 16) & 0xFFFF
		for _, c := range comboCtls {
			if c == lp && lp != 0 {
				pInvalidateRect.Call(lp, 0, 0)
			}
		}
		if id == 1 || id == 2 { // Enter / Esc (IsDialogMessage sends IDOK / IDCANCEL)
			if code == 0 || code == BN_CLICKED {
				if id == 1 {
					startGame(h)
				} else {
					sendMsg(h, WM_CLOSE, 0, 0)
				}
			}
			return 0
		}
		if code == CBN_SELCHANGE {
			if isPref(id) {
				prefChanged(id)
				return 0
			}
			if id == idMonitor {
				rebuildResCombo()
			}
			updateHints()
			setDirty(true)
			return 0
		}
		if _, isChk := checkState[id]; isChk && code == 5 /*BN_DOUBLECLICKED*/ {
			code = BN_CLICKED
		}
		if code == BN_CLICKED {
			switch id {
			case idSkipIntro, idRyzen, idWatermark, idHighPriority, idDisableFSO:
				toggleCheck(id)
				setDirty(true)
			case idTooltips, idRememberTab, idAskSave, idConfirmReset, idQuickLaunch, idCheckUpdates:
				toggleCheck(id)
				prefChanged(id)
			case idPresetPotato, idPresetConsole, idPresetVanilla, idPresetOpt, idPresetUltra, idPresetMax:
				applyPreset(id)
			case idReset:
				if !settings.ConfirmReset || confirm(h, "Reset the DISPLAY settings to the recommended defaults?\n\n(Desktop resolution, Borderless, Vulkan, VSync on.)", appTitle) {
					resetDisplay()
				}
			case idSaveProfile:
				saveProfile()
			case idLoadProfile:
				loadProfile()
			case idOpenProfiles:
				pShellExecuteW.Call(h, u16p("open"), u16p(profilesDir()), 0, 0, SW_SHOW)
			case idDLCManager:
				dm := filepath.Join(exeDir, "DLC.exe")
				if !exists(dm) {
					msgBox(h, "DLC Manager (DLC.exe) was not found in the game folder.", appTitle, MB_ICONWARNING)
				} else {
					pShellExecuteW.Call(h, u16p("open"), u16p(dm), 0, u16p(exeDir), SW_SHOW)
				}
			case idModList:
				m := installedMods()
				txt := "No mods are installed with DLC Manager."
				if len(m) > 0 {
					txt = fmt.Sprintf("Installed with DLC Manager (%d):\n\n  -  %s", len(m), strings.Join(m, "\n  -  "))
				}
				msgBox(h, txt, "Installed Mods", MB_ICONINFO)
			case idOpenMods:
				d := filepath.Join(exeDir, "DLC", "Installed")
				if !exists(d) {
					d = filepath.Join(exeDir, "DLC")
				}
				pShellExecuteW.Call(h, u16p("open"), u16p(d), 0, 0, SW_SHOW)
			case idDiagnostics:
				if p, err := writeDiagnostics(); err != nil {
					msgBox(h, "Could not write the report:\n\n"+err.Error(), appTitle, MB_ICONERROR)
				} else {
					pShellExecuteW.Call(h, u16p("open"), u16p(p), 0, 0, SW_SHOW)
				}
			case idRestoreVanilla:
				if confirm(h, "Restore Vanilla puts the game back to how it was before this launcher:\n\n"+
					"  -  original MKKE.exe (patches removed)\n  -  DirectX 9 (Vulkan / dgVoodoo2 turned off)\n  -  intro movies restored\n"+
					"  -  options.ini unlocked, Windows compatibility flags removed\n\n"+
					"Your graphics settings, DLC Manager mods and DLC order are NOT touched.\n\nContinue?", "Restore Vanilla") {
					if err := restoreVanilla(); err != nil {
						msgBox(h, "Restored, but some items need attention:\n\n"+err.Error(), "Restore Vanilla", MB_ICONWARNING)
					} else {
						msgBox(h, "Done - the game is back to vanilla.\n\nTo use the launcher's features again, just pick your settings and click APPLY SETTINGS or START GAME.", "Restore Vanilla", MB_ICONINFO)
					}
				}
			case idUpdateCheck:
				checkUpdates(true)
			case idOpenLogs:
				pShellExecuteW.Call(h, u16p("open"), u16p(filepath.Join(exeDir, "logs")), 0, 0, SW_SHOW)
			case idManual:
				if confirm(h, "Manual Mode removes the read-only lock from options.ini so you can edit it by hand.\n\nThe launcher will close and unsaved changes will be lost. Continue?", "Manual Mode") {
					setReadOnly(optPath, false)
					logf("manual mode: options.ini unlocked")
					pShellExecuteW.Call(h, u16p("open"), u16p(filepath.Dir(optPath)), 0, 0, SW_SHOW)
					pDestroyWindow.Call(h)
				}
			case idApply:
				if err := applyAll(); err != nil {
					msgBox(h, "Could not apply settings:\n\n"+err.Error(), appTitle, MB_ICONERROR)
				} else {
					setDirty(false)
					flashSaved()
				}
			case idOriginal:
				orig := filepath.Join(exeDir, "MKLauncher_Original.exe")
				if _, err := os.Stat(orig); err != nil {
					msgBox(h, "MKLauncher_Original.exe was not found in the game folder.", appTitle, MB_ICONWARNING)
				} else {
					pShellExecuteW.Call(h, u16p("open"), u16p(orig), 0, u16p(exeDir), SW_SHOW)
					pDestroyWindow.Call(h)
				}
			case idOpenCfg:
				pShellExecuteW.Call(h, u16p("open"), u16p(filepath.Dir(optPath)), 0, 0, SW_SHOW)
			case idOpenGame:
				pShellExecuteW.Call(h, u16p("open"), u16p(exeDir), 0, 0, SW_SHOW)
			}
		}
		return 0
	case WM_CLOSE:
		if dirty && settings.AskSave {
			r, _, _ := pMessageBoxW.Call(h, u16p("You have unsaved changes.\n\nSave them before closing?"), u16p(appTitle), 0x3|0x20 /*MB_YESNOCANCEL|MB_ICONQUESTION*/)
			switch r {
			case 2: // cancel
				return 0
			case 6: // yes
				if err := applyAll(); err != nil {
					msgBox(h, "Could not apply settings:\n\n"+err.Error(), appTitle, MB_ICONERROR)
					return 0
				}
			}
		}
		pDestroyWindow.Call(h)
		return 0
	case WM_DESTROY:
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(h, msg, wp, lp)
	return r
}

func startGame(h uintptr) {
	if w, _ := findGameWindow(); w != 0 {
		msgBox(h, "Mortal Kombat is already running.", appTitle, MB_ICONINFO)
		return
	}
	if err := applyAll(); err != nil {
		msgBox(h, "Could not apply settings:\n\n"+err.Error(), appTitle, MB_ICONERROR)
		return
	}
	launchGame = true
	pDestroyWindow.Call(h)
}

func paint(h uintptr) {
	var ps PAINTSTRUCT
	hdc, _, _ := pBeginPaint.Call(h, uintptr(unsafe.Pointer(&ps)))
	var cr RECT
	pGetClientRect.Call(h, uintptr(unsafe.Pointer(&cr)))
	w, ht := cr.Right, cr.Bottom
	mdc, _, _ := pCreateCompatibleDC.Call(hdc)
	bmp, _, _ := pCreateCompatibleBitmap.Call(hdc, uintptr(w), uintptr(ht))
	old, _, _ := pSelectObject.Call(mdc, bmp)

	fillRect(mdc, cr, colBg)
	// tabs
	for i, n := range tabNames {
		r := tabRect(i)
		col := colText
		if i == hoverTab && i != curTab {
			col = colTabHover
		}
		if i == curTab {
			frameRect(mdc, r, colBorder)
			col = colBlue
		} else if i < len(tabNames)-1 && i+1 != curTab {
			hline(mdc, r.Right+sc(3), r.Right+sc(3)+1, r.Top+sc(6), colBorder)
			fillRect(mdc, RECT{r.Right + sc(3), r.Top + sc(6), r.Right + sc(3) + 1, r.Bottom - sc(6)}, colBorder)
		}
		drawText(mdc, n, r, fTab, col, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
	}
	frameRect(mdc, panelRect(), colBorder)
	drawBanner(mdc, bannerRect(), bannerHover)
	fr := RECT{sc(12), sc(650), sc(548), sc(674)}
	drawText(mdc, cpuText, fr, fFoot, colBlue, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	drawText(mdc, gpuText, fr, fFoot, gpuColor, DT_RIGHT|DT_VCENTER|DT_SINGLELINE)

	pBitBlt.Call(hdc, 0, 0, uintptr(w), uintptr(ht), mdc, 0, 0, SRCCOPY)
	pSelectObject.Call(mdc, old)
	pDeleteObject.Call(bmp)
	pDeleteDC.Call(mdc)
	pEndPaint.Call(h, uintptr(unsafe.Pointer(&ps)))
}

func drawOwnerButton(d *DRAWITEMSTRUCT) {
	if on, ok := checkState[int(d.CtlID)]; ok {
		drawCheckBox(d, ownerBtns[int(d.CtlID)], on, d.HwndItem == hoverBtn)
		return
	}
	r := d.RcItem
	bg := colBg
	border := colBorder
	if d.HwndItem == hoverBtn && d.ItemState&ODS_DISABLED == 0 {
		bg = colHoverBg
		border = colBlue
	}
	if d.ItemState&ODS_SELECTED != 0 {
		bg = colPressBg
	}
	fillRect(d.HDC, r, bg)
	frameRect(d.HDC, r, border)
	col := colText
	if d.ItemState&ODS_DISABLED != 0 {
		col = colDisabled
	}
	txt := ownerBtns[int(d.CtlID)]
	if int(d.CtlID) == idApply && savedFlash {
		txt, col = "✓  SAVED", rgb(40, 150, 60)
		frameRect(d.HDC, r, rgb(40, 150, 60))
	}
	drawText(d.HDC, txt, r, fBtn, col, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ------------------------------------------------------------------ apply

func applyAll() error {
	settings.Renderer = sel(idRenderer)
	settings.Mode = sel(idMode)
	settings.FPS = sel(idFPS)
	settings.VSync = sel(idVSync) == 0
	settings.ForceAA = sel(idForceAA)
	settings.ForceAF = sel(idForceAF)
	settings.SkipIntro = checked(idSkipIntro)
	settings.HighPriority = checked(idHighPriority)
	settings.DisableFSO = checked(idDisableFSO)
	settings.Ryzen = checked(idRyzen)
	settings.Watermark = checked(idWatermark)
	settings.Monitor = clamp(sel(idMonitor), 0, len(monitors)-1)
	mon := monitors[settings.Monitor]
	targetRect = RECT{mon.X, mon.Y, mon.X + mon.W, mon.Y + mon.H}
	if mon.Primary {
		opts.SetRaw("video_device", "4294967295")
	} else {
		opts.SetRaw("video_device", fmt.Sprint(mon.Ordinal))
	}
	opts.SetRaw("distance_filter", fmt.Sprint(clamp(sel(idDistance), 0, 2)))
	p := resList[clamp(sel(idRes), 0, len(resList)-1)]
	opts.ResX, opts.ResY = p[0], p[1]
	opts.Letterbox = sel(idLetter) == 0
	opts.SetRaw("max_texture", fmt.Sprint(texValues[clamp(sel(idTexture), 0, 2)]))
	opts.SetRaw("shadow_size", fmt.Sprint(shadowValues[clamp(sel(idShadow), 0, 3)]))
	opts.SetRaw("anisotropy", fmt.Sprint(anisoValues[clamp(sel(idAniso), 0, 4)]))
	if sel(idMemory) == 1 {
		opts.SetRaw("unlimited_memory", "true")
	} else {
		opts.SetRaw("unlimited_memory", "false")
	}

	if err := switchRenderer(settings.Renderer); err != nil {
		return err
	}
	if settings.Renderer == rendVulkan {
		if err := writeDXVKConf(); err != nil {
			return fmt.Errorf("could not write dxvk.conf: %v", err)
		}
	}

	if c, err := LoadConf(confPath); err == nil {
		api := "d3d11_fl11_0"
		if settings.Renderer == 2 {
			api = "d3d12_fl12_0"
		}
		tf := func(b bool) string {
			if b {
				return "true"
			}
			return "false"
		}
		c.Set("General", "OutputAPI", api)
		c.Set("General", "FullScreenMode", tf(settings.Mode == 0))
		c.Set("DirectX", "AppControlledScreenMode", tf(settings.Mode == 0))
		c.Set("GeneralExt", "WindowedAttributes", "")
		c.Set("GeneralExt", "FullscreenAttributes", "")
		c.Set("GeneralExt", "FPSLimit", fmt.Sprint(fpsValues[clamp(settings.FPS, 0, 3)]))
		c.Set("DirectX", "ForceVerticalSync", tf(settings.VSync))
		c.Set("DirectX", "dgVoodooWatermark", tf(settings.Watermark))
		c.Set("DirectX", "Antialiasing", forceAAValues[clamp(settings.ForceAA, 0, len(forceAAValues)-1)])
		c.Set("DirectX", "Filtering", forceAFValues[clamp(settings.ForceAF, 0, len(forceAFValues)-1)])
		c.Set("DirectX", "KeepFilterIfPointSampled", tf(settings.ForceAF != 0))
		if err := c.Save(); err != nil {
			return fmt.Errorf("could not write dgVoodoo.conf: %v", err)
		}
	}

	movies := filepath.Join(exeDir, "Movies")
	for _, m := range introMovies {
		a, b := filepath.Join(movies, m), filepath.Join(movies, m+".skip")
		if settings.SkipIntro {
			if _, err := os.Stat(a); err == nil {
				os.Rename(a, b)
			}
		} else if _, err := os.Stat(b); err == nil {
			os.Rename(b, a)
		}
	}

	setFSO(settings.DisableFSO)

	logf("apply: res=%dx%d mode=%d renderer=%d fps=%d vsync=%v monitor=%d hd=%v skipintro=%v ryzen=%v",
		opts.ResX, opts.ResY, settings.Mode, settings.Renderer, settings.FPS, settings.VSync, settings.Monitor, settings.HDTex, settings.SkipIntro, settings.Ryzen)
	if err := patchExe(settings.Mode == 1); err != nil {
		logf("exe patch failed: %v", err)
		if settings.Mode != 0 {
			return err
		}
	}
	if err := opts.Save(settings.Mode != 0); err != nil {
		return fmt.Errorf("could not write %s: %v", optPath, err)
	}
	return settings.Save(setPath)
}

func tipFor(id int, text string, warn bool) {
	addTip(ctl[id], text, warn) // only the dropdown / checkbox / button itself, not its label
}

func addAllTips() {
	tipFor(idMonitor, "Which monitor the game runs on. Borderless and Fullscreen open on this display.\n(Main) marks your primary monitor.", false)
	tipFor(idRes, "The resolution the game renders at. For Borderless, your monitor's own resolution looks sharpest. Ultrawide resolutions (21:9, 32:9) are listed too.", false)
	tipFor(idMode, "Fullscreen: classic exclusive fullscreen.\nBorderless: a window that covers the whole screen - alt-tab is instant and the game stays visible when you click another monitor.\nWindowed: a normal window with a title bar.", false)
	tipFor(idRenderer, "DirectX 9: the game's original renderer.\nDirectX 11 / 12: runs the game through dgVoodoo2 (some God of War content renders black).\nVulkan (DXVK): runs the game through DXVK - most accurate, recommended.\nThe Vulkan overlay will still say D3D9: that's the game's own API, which DXVK translates to Vulkan.", false)
	tipFor(idVSync, "Enabled: syncs frames to your monitor's refresh rate - no screen tearing (recommended).\nDisabled: slightly lower input delay, but you may see tearing.\nNeeds DirectX 11 / 12 or Vulkan.", false)
	tipFor(idFPS, "Default (60): the game's own built-in 60 FPS limit.\n60 (dgVoodoo): dgVoodoo2 also caps at 60 for steadier frame pacing - try this if you get micro stutters.\n45 (slow-mo): fights run at 75% speed. Only for very weak PCs.\n30 (slow-mo): fights run at half speed. Last resort.\nThe game's speed is tied to its frame rate, so it can't go above 60.", true)
	tipFor(idLetter, "Enabled: cutscenes and menus keep their shape, with black bars where needed.\nDisabled: the picture is stretched to fill the screen.", false)
	tipFor(idTexture, "Medium (1024): lowest memory use, softer textures.\nHigh (2048): the game's default.\nVery High (4096): every texture at full detail.", false)
	tipFor(idShadow, "512: blocky shadows, fastest.\n1024: the game's default.\n2048: sharp shadows.\n4096: sharpest, most demanding.", true)
	tipFor(idAniso, "1x: no extra filtering - floors blur at a distance.\n2x / 4x: a bit sharper.\n8x / 16x: floors and walls stay crisp at steep angles. 16x costs almost nothing on modern GPUs.", false)
	tipFor(idMemory, "Standard: the game's old console-era video memory budget.\nUnlimited: lets the game use more video memory, so textures don't get swapped out (recommended).", false)
	tipFor(idForceAA, "Game Controlled: uses the game's own anti-aliasing.\nOff: disables anti-aliasing completely.\n2x-8x MSAA: forces multisample anti-aliasing through dgVoodoo2. Higher is smoother but costs more performance.", false)
	tipFor(idForceAF, "Game Controlled: uses the game's own filtering (set by Anisotropic Filter above).\nBilinear / Trilinear: basic filtering, softer at a distance.\n2x-16x Anisotropic: forces sharper textures at angles on every surface. 16x is sharpest and cheap on modern GPUs.\nThe game's crisp 2D menu art is left untouched.", false)
	tipFor(idSkipIntro, "Skips the WB Games, NetherRealm and THX logo videos when the game starts. Untick to restore them.", false)
	tipFor(idDistance, "Nearby: only close opponents - best connection.\nFar: a wider area - more players, a bit more lag.\nWorldwide: anyone - the most players, but connections can be laggy.", false)
	tipFor(idRyzen, "Some newer AMD Ryzen CPUs freeze or stutter in MKKE. This limits the game to the first 8 CPU threads, which is the commonly reported fix. Leave it off if the game runs fine.", true)
	tipFor(idWatermark, "DirectX 11 / 12: shows the small dgVoodoo logo in the corner.\nVulkan: shows an FPS counter and your GPU in the top-left corner.\nUseful to confirm which renderer is running.", false)
	tipFor(idPresetVanilla, "The game's original look: high textures, medium shadows, 4x filtering.", false)
	tipFor(idPresetOpt, "Best quality for almost any modern PC: very high textures, high shadows, 16x filtering, unlimited video memory.", false)
	tipFor(idPresetPotato, "For very low-end PCs: lowest textures and shadows, no filtering, anti-aliasing off. Fastest setting.", false)
	tipFor(idPresetConsole, "Close to the PS3 / Xbox 360 version: medium textures, low-medium shadows, light filtering. For a full console feel, also pick 1280x720.", false)
	tipFor(idPresetMax, "Absolute maximum: every setting at its highest plus forced 8x MSAA and 16x filtering. Switches to DirectX 11 if you're on DirectX 9. Needs a strong GPU.", true)
	tipFor(idPresetUltra, "Maximum quality: very high shadows plus forced 4x MSAA and 16x filtering through dgVoodoo2. Needs DirectX 11 or 12.", true)
	tipFor(idOpenCfg, "Opens %APPDATA%\\MKKE, where the game keeps options.ini.", false)
	tipFor(idOpenGame, "Opens the DiscContentPC folder.", false)
	tipFor(idOpenLogs, "Opens the launcher's log files. Include the newest one when reporting a problem.", false)
	tipFor(idManual, "Removes the read-only lock from options.ini so you can edit it by hand, then closes the launcher.", true)
	tipFor(idReset, "Resets the DISPLAY tab to the recommended defaults.", false)
	tipFor(idOriginal, "Closes this launcher and opens the original Mortal Kombat launcher.", false)
	tipFor(idTheme, "Light / Dark: the classic looks.\nMatch Windows: follows your Windows light / dark setting automatically.\nThe rest are Mortal Kombat themed colour schemes - pick one and the launcher changes instantly.", false)
	tipFor(idAccent, "Colour of the headers, selected tab, borders, buttons and checkboxes.\nTheme Default uses the colour that comes with the theme.", false)
	tipFor(idTextColor, "Colour of the labels, button text and dropdown text.\nTheme Default uses the colour that comes with the theme.", false)
	tipFor(idAfterLaunch, "Close launcher: the launcher closes once the game is running.\nReopen when game closes: the launcher comes back after you quit the game, ready to change settings.", false)
	tipFor(idLogDays, "How long the launcher keeps its log files before deleting them.", false)
	tipFor(idQuickLaunch, "Starts the game immediately with your saved settings, without showing this window.\nHold SHIFT while opening the launcher to get back here.", true)
	tipFor(idHighPriority, "Tells Windows to give the game priority over background apps. Can reduce stutter when lots of programs are open.", false)
	tipFor(idDisableFSO, "Turns off Windows 'fullscreen optimizations' for MKKE.exe. Some older DirectX games stutter or have input lag with them on.\nOnly matters in Fullscreen mode.", false)
	tipFor(idSaveProfile, "Saves all your current settings to a profile file you can load later or share with friends.", false)
	tipFor(idLoadProfile, "Loads settings from a profile file. Your monitor choice is kept.", false)
	tipFor(idOpenProfiles, "Opens the Profiles folder next to the launcher.", false)
	tipFor(idDLCManager, "Opens DLC Manager to install or remove character, costume and arena mods.", false)
	tipFor(idModList, "Lists every mod currently installed with DLC Manager.", false)
	tipFor(idOpenMods, "Opens the DLC Manager mods folder.", false)
	tipFor(idDiagnostics, "Creates a report of your setup (Windows, GPU, game patches, settings, installed mods) and opens it.\nSend it along when reporting a problem.", false)
	tipFor(idRestoreVanilla, "Undoes everything the launcher changed in the game files: original MKKE.exe, DirectX 9 and the intro movies.\nYour mods and DLC order are kept.", true)
	tipFor(idCheckUpdates, "When the launcher starts, quietly check if a newer version has been released.", false)
	tipFor(idUpdateCheck, "Checks right now whether a newer version of the launcher has been released.", false)
	tipFor(idApply, "Saves your settings without starting the game. Clicking START GAME also saves them.", false)
}

func toggleCheck(id int) {
	checkState[id] = !checkState[id]
	pInvalidateRect.Call(ctl[id], 0, 0)
}

func isPref(id int) bool {
	switch id {
	case idTheme, idAccent, idTextColor, idAfterLaunch, idLogDays, idTooltips, idRememberTab, idAskSave, idConfirmReset, idQuickLaunch, idCheckUpdates:
		return true
	}
	return false
}

// prefChanged stores launcher preferences immediately (they never mark the game settings dirty).
func prefChanged(id int) {
	switch id {
	case idTheme:
		settings.Theme = sel(idTheme)
		applyTheme()
	case idAccent:
		settings.Accent = sel(idAccent)
		applyTheme()
	case idTextColor:
		settings.TextColor = sel(idTextColor)
		applyTheme()
	case idAfterLaunch:
		settings.AfterLaunch = sel(idAfterLaunch)
	case idLogDays:
		settings.LogDays = sel(idLogDays)
	case idTooltips:
		settings.Tooltips = checked(idTooltips)
		setTipsActive(settings.Tooltips)
	case idRememberTab:
		settings.RememberTab = checked(idRememberTab)
	case idAskSave:
		settings.AskSave = checked(idAskSave)
	case idConfirmReset:
		settings.ConfirmReset = checked(idConfirmReset)
	case idCheckUpdates:
		settings.CheckUpdates = checked(idCheckUpdates)
	case idQuickLaunch:
		settings.QuickLaunch = checked(idQuickLaunch)
		if settings.QuickLaunch {
			msgBox(hMain, "Quick launch is on.\n\nNext time you open the launcher, the game starts straight away with your saved settings.\n\nTo get back to this window, hold SHIFT while opening the launcher.", appTitle, MB_ICONINFO)
		}
	}
	logf("preference %d changed", id)
	if err := settings.Save(setPath); err != nil {
		logf("could not save preferences: %v", err)
	}
}

func wstr(p uintptr) string {
	var out []uint16
	for i := 0; i < 64; i++ {
		c := *(*uint16)(unsafe.Pointer(p + uintptr(i*2)))
		if c == 0 {
			break
		}
		out = append(out, c)
	}
	return syscall.UTF16ToString(out)
}
