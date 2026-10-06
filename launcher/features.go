//go:build windows

package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var (
	comdlg32           = syscall.NewLazyDLL("comdlg32.dll")
	ntdll              = syscall.NewLazyDLL("ntdll.dll")
	pGetSaveFileNameW  = comdlg32.NewProc("GetSaveFileNameW")
	pGetOpenFileNameW  = comdlg32.NewProc("GetOpenFileNameW")
	pRtlGetVersion     = ntdll.NewProc("RtlGetVersion")
	pPostMessageW      = user32.NewProc("PostMessageW")
	pRegSetKeyValueW   = advapi32.NewProc("RegSetKeyValueW")
	pRegDeleteKeyValue = advapi32.NewProc("RegDeleteKeyValueW")
	pSetPriorityClass  = kernel32.NewProc("SetPriorityClass")
)

const (
	WM_APP_WELCOME = 0x8000 + 1
	WM_APP_UPDATE  = 0x8000 + 2
)

func fileMD5(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	h := md5.Sum(b)
	return hex.EncodeToString(h[:])
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

// ------------------------------------------------------------------ compatibility

const layersKey = `Software\Microsoft\Windows NT\CurrentVersion\AppCompatFlags\Layers`
const fsoFlag = "DISABLEDXMAXIMIZEDWINDOWEDMODE"

func regReadSZ(root uintptr, key, name string) string { return regString(root, key, name) }

// setFSO turns Windows' fullscreen optimizations off (or back on) for MKKE.exe, keeping any other compatibility flags.
func setFSO(disable bool) {
	exe := filepath.Join(exeDir, "MKKE.exe")
	cur := regReadSZ(HKEY_CURRENT_USER, layersKey, exe)
	var toks []string
	for _, t := range strings.Fields(cur) {
		if t != "~" && !strings.EqualFold(t, fsoFlag) {
			toks = append(toks, t)
		}
	}
	if disable {
		toks = append(toks, fsoFlag)
	}
	if len(toks) == 0 {
		if cur != "" {
			pRegDeleteKeyValue.Call(HKEY_CURRENT_USER, u16p(layersKey), u16p(exe))
		}
		return
	}
	val := "~ " + strings.Join(toks, " ")
	if val == cur {
		return
	}
	v, _ := syscall.UTF16FromString(val)
	pRegSetKeyValueW.Call(HKEY_CURRENT_USER, u16p(layersKey), u16p(exe), 1 /*REG_SZ*/, uintptr(unsafe.Pointer(&v[0])), uintptr(len(v)*2))
	logf("compat flags for MKKE.exe: %s", val)
}

func setHighPriority(pid uintptr) bool {
	hp, _, _ := pOpenProcess.Call(PROCESS_SET_INFORMATION, 0, pid)
	if hp == 0 {
		return false
	}
	defer pCloseHandle.Call(hp)
	r, _, _ := pSetPriorityClass.Call(hp, 0x80 /*HIGH_PRIORITY_CLASS*/)
	return r != 0
}

// ------------------------------------------------------------------ first run

var presetNames = map[int]string{idPresetPotato: "POTATO", idPresetConsole: "CONSOLE", idPresetVanilla: "VANILLA",
	idPresetOpt: "OPTIMIZED", idPresetUltra: "ULTRA", idPresetMax: "MAXIMUM"}

var numRe = regexp.MustCompile(`\d{3,4}`)

func pickPreset(gpu string) int {
	g := strings.ToUpper(gpu)
	n := 0
	if m := numRe.FindString(g); m != "" {
		n, _ = strconv.Atoi(m)
	}
	switch {
	case strings.Contains(g, "RTX"):
		return idPresetUltra
	case strings.Contains(g, "GTX"):
		if n >= 900 || (n >= 100 && n < 200) || n >= 1000 {
			return idPresetOpt
		}
		return idPresetVanilla
	case strings.Contains(g, "RX "):
		if n >= 6000 && n < 10000 {
			return idPresetUltra
		}
		return idPresetOpt
	case strings.Contains(g, "ARC"):
		return idPresetOpt
	case strings.Contains(g, "UHD"), strings.Contains(g, "HD GRAPHICS"), strings.Contains(g, "IRIS"),
		strings.Contains(g, "VEGA"), strings.Contains(g, "RADEON(TM) GRAPHICS"), strings.Contains(g, "RADEON GRAPHICS"):
		return idPresetConsole
	}
	return idPresetVanilla
}

var firstRunPreset int

func doFirstRun() {
	resetDisplay()
	firstRunPreset = pickPreset(gpuText)
	applyPreset(firstRunPreset)
	settings.Save(setPath)
	logf("first run: preset %s for %s", presetNames[firstRunPreset], gpuText)
	pPostMessageW.Call(hMain, WM_APP_WELCOME, 0, 0)
}

func showWelcome() {
	msgBox(hMain, fmt.Sprintf("Welcome to the %s!\n\nFirst-time setup picked these settings for you:\n\n"+
		"  -  %s preset for your %s\n  -  Borderless, Vulkan, VSync on\n  -  Your monitor's native resolution\n\n"+
		"Look around the tabs (hover any option for an explanation), then click START GAME.", appTitle,
		presetNames[firstRunPreset], strings.TrimSpace(gpuText)), appTitle, MB_ICONINFO)
}

// ------------------------------------------------------------------ restore vanilla

func restoreVanilla() error {
	var errs []string
	if err := patchExeState(false, false); err != nil {
		errs = append(errs, "MKKE.exe: "+err.Error())
	}
	if err := switchRenderer(0); err != nil {
		errs = append(errs, err.Error())
	}
	movies := filepath.Join(exeDir, "Movies")
	for _, m := range introMovies {
		a, b := filepath.Join(movies, m), filepath.Join(movies, m+".skip")
		if exists(b) && !exists(a) {
			os.Rename(b, a)
		}
	}
	setFSO(false)
	opts.remove("fullscreen")
	setReadOnly(optPath, false)
	if err := os.WriteFile(optPath, []byte(strings.Join(opts.lines, opts.nl)), 0644); err != nil {
		errs = append(errs, "options.ini: "+err.Error())
	}
	settings.Renderer, settings.Mode, settings.SkipIntro = 0, 0, false
	settings.Ryzen, settings.HighPriority, settings.DisableFSO = false, false, false
	settings.Save(setPath)
	// reflect in the UI
	setSel(idRenderer, 0)
	setSel(idMode, 0)
	for _, id := range []int{idSkipIntro, idRyzen, idHighPriority, idDisableFSO} {
		checkState[id] = false
		pInvalidateRect.Call(ctl[id], 0, 0)
	}
	updateHints()
	setDirty(false)
	logf("restore vanilla: %d problem(s)", len(errs))
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "\n"))
	}
	return nil
}

// ------------------------------------------------------------------ diagnostics

func windowsVersion() string {
	type osvi struct {
		Size, Major, Minor, Build, Platform uint32
		CSD                                 [128]uint16
	}
	v := osvi{Size: uint32(unsafe.Sizeof(osvi{}))}
	pRtlGetVersion.Call(uintptr(unsafe.Pointer(&v)))
	name := "Windows 10"
	if v.Major == 10 && v.Build >= 22000 {
		name = "Windows 11"
	} else if v.Major < 10 {
		name = fmt.Sprintf("Windows NT %d.%d", v.Major, v.Minor)
	}
	return fmt.Sprintf("%s (build %d)", name, v.Build)
}

func patchState(p exePatch) string {
	f, err := os.Open(filepath.Join(exeDir, "MKKE.exe"))
	if err != nil {
		return "cannot read"
	}
	defer f.Close()
	b := make([]byte, len(p.orig))
	f.ReadAt(b, p.off)
	switch string(b) {
	case string(p.orig):
		return "original"
	case string(p.new):
		return "patched"
	}
	return "UNKNOWN"
}

func writeDiagnostics() (string, error) {
	var b strings.Builder
	w := func(f string, a ...any) { fmt.Fprintf(&b, f+"\r\n", a...) }
	w("%s v%s - diagnostics report", appTitle, appVersion)
	w("Created: %s", time.Now().Format("2006-01-02 15:04:05"))
	w("")
	w("[System]")
	w("OS:  %s", windowsVersion())
	w("CPU: %s", cpuText)
	w("GPU: %s", gpuText)
	for _, m := range monitors {
		w("Monitor: %s  %dx%d at (%d,%d) primary=%v", m.Device, m.W, m.H, m.X, m.Y, m.Primary)
	}
	w("")
	w("[Game files]")
	w("Folder: %s", exeDir)
	w("MKKE.exe md5: %s", fileMD5(filepath.Join(exeDir, "MKKE.exe")))
	w("Patch windowed fix: %s | window pos: %s | window style: %s", patchState(patchWindowedFix), patchState(patchWinPos), patchState(patchWinStyle))
	act := "none (plain DirectX 9)"
	switch m := fileMD5(d3d9Active()); {
	case m == "":
	case m == fileMD5(d3d9DXVK()):
		act = "DXVK (Vulkan)"
	case m == fileMD5(d3d9DgVoodoo()):
		act = "dgVoodoo2"
	default:
		act = "UNKNOWN D3D9.dll " + m
	}
	w("Active renderer DLL: %s | dgVoodoo copy: %v, dgVoodoo.conf: %v | DXVK copy: %v, dxvk.conf: %v", act,
		exists(d3d9DgVoodoo()), exists(confPath), dxvkAvailable(), exists(dxvkConfPath()))
	for _, m := range introMovies {
		w("Movie %s: present=%v skipped=%v", m, exists(filepath.Join(exeDir, "Movies", m)), exists(filepath.Join(exeDir, "Movies", m+".skip")))
	}
	w("DLC Manager (DLC.exe): %v   MK9Addon.asi: %v", exists(filepath.Join(exeDir, "DLC.exe")), exists(filepath.Join(exeDir, "MK9Addon.asi")))
	w("Compat flags: %q", regReadSZ(HKEY_CURRENT_USER, layersKey, filepath.Join(exeDir, "MKKE.exe")))
	mods := installedMods()
	w("Installed mods (%d): %s", len(mods), strings.Join(mods, " | "))
	w("")
	if c, err := os.ReadFile(confPath); err == nil {
		w("[dgVoodoo.conf - key values]")
		for _, l := range strings.Split(string(c), "\n") {
			t := strings.TrimSpace(l)
			for _, k := range []string{"OutputAPI", "FullScreenMode", "AppControlledScreenMode", "FPSLimit", "ForceVerticalSync", "Antialiasing", "Filtering", "dgVoodooWatermark", "VRAM"} {
				if strings.HasPrefix(t, k) && strings.Contains(t, "=") {
					w("%s", t)
				}
			}
		}
		w("")
	}
	ro := "no"
	if a, _, _ := pGetFileAttributesW.Call(u16p(optPath)); a != 0xFFFFFFFF && a&1 != 0 {
		ro = "yes"
	}
	w("[options.ini]  (%s, read-only: %s)", optPath, ro)
	if c, err := os.ReadFile(optPath); err == nil {
		b.WriteString(strings.ReplaceAll(strings.ReplaceAll(string(c), "\r\n", "\n"), "\n", "\r\n"))
		w("")
	}
	w("")
	w("[MKKE_Launcher.ini]")
	if c, err := os.ReadFile(setPath); err == nil {
		b.Write(c)
	}
	p := filepath.Join(exeDir, "logs", "diagnostics_"+time.Now().Format("2006-01-02_15-04-05")+".txt")
	os.MkdirAll(filepath.Dir(p), 0755)
	return p, os.WriteFile(p, []byte(b.String()), 0644)
}

// ------------------------------------------------------------------ mods

func installedMods() []string {
	var out []string
	ents, _ := os.ReadDir(filepath.Join(exeDir, "DLC", "Installed"))
	for _, e := range ents {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".ini") {
			out = append(out, strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())))
		}
	}
	sort.Strings(out)
	return out
}

func modSummary() string {
	m := installedMods()
	switch len(m) {
	case 0:
		return "No DLC Manager mods installed."
	case 1:
		return "1 mod installed with DLC Manager."
	}
	return fmt.Sprintf("%d mods installed with DLC Manager.", len(m))
}

// ------------------------------------------------------------------ profiles

type OPENFILENAMEW struct {
	StructSize      uint32
	Owner           uintptr
	Instance        uintptr
	Filter          *uint16
	CustomFilter    *uint16
	MaxCustomFilter uint32
	FilterIndex     uint32
	File            *uint16
	MaxFile         uint32
	FileTitle       *uint16
	MaxFileTitle    uint32
	InitialDir      *uint16
	Title           *uint16
	Flags           uint32
	FileOffset      uint16
	FileExtension   uint16
	DefExt          *uint16
	CustData        uintptr
	FnHook          uintptr
	TemplateName    *uint16
	PvReserved      uintptr
	DwReserved      uint32
	FlagsEx         uint32
}

func profilesDir() string {
	d := filepath.Join(exeDir, "Profiles")
	os.MkdirAll(d, 0755)
	return d
}

func fileDialog(save bool, title string) string {
	buf := make([]uint16, 520)
	if save {
		copy(buf, syscall.StringToUTF16("My Settings"))
	}
	filter := syscall.StringToUTF16("MKUKE launcher profile (*.mkprofile)\x00*.mkprofile\x00All files\x00*.*\x00")
	ofn := OPENFILENAMEW{StructSize: uint32(unsafe.Sizeof(OPENFILENAMEW{})), Owner: hMain, Filter: &filter[0],
		File: &buf[0], MaxFile: uint32(len(buf)), InitialDir: u16(profilesDir()), Title: u16(title), DefExt: u16("mkprofile")}
	var r uintptr
	if save {
		ofn.Flags = 0x2 | 0x800 | 0x80000 // OVERWRITEPROMPT | PATHMUSTEXIST | EXPLORER
		r, _, _ = pGetSaveFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	} else {
		ofn.Flags = 0x1000 | 0x800 | 0x80000 // FILEMUSTEXIST | PATHMUSTEXIST | EXPLORER
		r, _, _ = pGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	}
	if r == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

var profileCombos = map[string]int{"DisplayMode": idMode, "Renderer": idRenderer, "VSync": idVSync, "FPSLimit": idFPS,
	"Letterboxing": idLetter, "Texture": idTexture, "Shadow": idShadow, "Aniso": idAniso, "Memory": idMemory,
	"ForceAA": idForceAA, "ForceAF": idForceAF, "Matchmaking": idDistance}
var profileChecks = map[string]int{"SkipIntro": idSkipIntro, "RyzenFix": idRyzen, "Watermark": idWatermark,
	"HighPriority": idHighPriority, "DisableFSO": idDisableFSO}

func saveProfile() {
	p := fileDialog(true, "Save settings profile")
	if p == "" {
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "; %s profile\r\n[Profile]\r\n", appTitle)
	r := resList[clamp(sel(idRes), 0, len(resList)-1)]
	fmt.Fprintf(&b, "Resolution=%dx%d\r\n", r[0], r[1])
	var keys []string
	for k := range profileCombos {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, "%s=%d\r\n", k, sel(profileCombos[k]))
	}
	keys = keys[:0]
	for k := range profileChecks {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := 0
		if checked(profileChecks[k]) {
			v = 1
		}
		fmt.Fprintf(&b, "%s=%d\r\n", k, v)
	}
	if err := os.WriteFile(p, []byte(b.String()), 0644); err != nil {
		msgBox(hMain, "Could not save the profile:\n\n"+err.Error(), appTitle, MB_ICONERROR)
		return
	}
	logf("profile saved: %s", p)
	msgBox(hMain, "Profile saved:\n\n"+filepath.Base(p)+"\n\nShare the file with friends - they can load it with LOAD PROFILE.", appTitle, MB_ICONINFO)
}

func loadProfile() {
	p := fileDialog(false, "Load settings profile")
	if p == "" {
		return
	}
	c, err := os.ReadFile(p)
	if err != nil {
		msgBox(hMain, "Could not read the profile:\n\n"+err.Error(), appTitle, MB_ICONERROR)
		return
	}
	n := 0
	for _, l := range strings.Split(string(c), "\n") {
		l = strings.TrimSpace(l)
		i := strings.Index(l, "=")
		if i < 0 || strings.HasPrefix(l, ";") {
			continue
		}
		k, v := strings.TrimSpace(l[:i]), strings.TrimSpace(l[i+1:])
		num, _ := strconv.Atoi(v)
		if id, ok := profileCombos[k]; ok {
			cnt := int(sendMsg(ctl[id], 0x0146 /*CB_GETCOUNT*/, 0, 0))
			if num >= 0 && num < cnt {
				setSel(id, num)
				n++
			}
		} else if id, ok := profileChecks[k]; ok {
			checkState[id] = num == 1
			pInvalidateRect.Call(ctl[id], 0, 0)
			n++
		} else if k == "Resolution" {
			var w, h int
			fmt.Sscanf(v, "%dx%d", &w, &h)
			for i, r := range resList {
				if r[0] == w && r[1] == h {
					setSel(idRes, i)
					n++
				}
			}
		}
	}
	for _, c := range comboCtls {
		pInvalidateRect.Call(c, 0, 0)
	}
	updateHints()
	setDirty(true)
	logf("profile loaded: %s (%d values)", p, n)
	msgBox(hMain, fmt.Sprintf("Loaded %d settings from %s.\n\nClick APPLY SETTINGS or START GAME to use them.", n, filepath.Base(p)), appTitle, MB_ICONINFO)
}

// ------------------------------------------------------------------ updates

const defaultUpdateRepo = "piercejonathan153-lab/MKU-KEL"

var updateResult struct {
	Tag, URL, Err string
	Manual        bool
}

func updateRepo() string {
	if r := strings.TrimSpace(settings.UpdateRepo); r != "" {
		return r
	}
	return defaultUpdateRepo
}

func versionNewer(tag, cur string) bool {
	parse := func(s string) []int {
		s = strings.TrimLeft(strings.ToLower(strings.TrimSpace(s)), "v")
		var out []int
		for _, p := range strings.Split(s, ".") {
			n, _ := strconv.Atoi(strings.TrimFunc(p, func(r rune) bool { return r < '0' || r > '9' }))
			out = append(out, n)
		}
		return out
	}
	a, b := parse(tag), parse(cur)
	for i := 0; i < len(a) || i < len(b); i++ {
		x, y := 0, 0
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			return x > y
		}
	}
	return false
}

func checkUpdates(manual bool) {
	repo := updateRepo()
	if repo == "" {
		if manual {
			msgBox(hMain, "Update checks will work once the launcher is published on GitHub.\n\n"+
				"(Set UpdateRepo=owner/repository in MKKE_Launcher.ini to point the launcher at your releases.)", appTitle, MB_ICONINFO)
		}
		return
	}
	go func() {
		updateResult.Manual = manual
		updateResult.Tag, updateResult.URL, updateResult.Err = "", "", ""
		c := &http.Client{Timeout: 8 * time.Second}
		req, _ := http.NewRequest("GET", "https://api.github.com/repos/"+repo+"/releases/latest", nil)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", "MKUKE-Launcher/"+appVersion)
		resp, err := c.Do(req)
		if err != nil {
			updateResult.Err = err.Error()
		} else {
			defer resp.Body.Close()
			var j struct {
				Tag  string `json:"tag_name"`
				HTML string `json:"html_url"`
			}
			if resp.StatusCode != 200 {
				updateResult.Err = resp.Status
			} else if err := json.NewDecoder(resp.Body).Decode(&j); err != nil {
				updateResult.Err = err.Error()
			} else {
				updateResult.Tag, updateResult.URL = j.Tag, j.HTML
			}
		}
		pPostMessageW.Call(hMain, WM_APP_UPDATE, 0, 0)
	}()
}

func showUpdateResult() {
	r := updateResult
	logf("update check: tag=%q err=%q", r.Tag, r.Err)
	if r.Err != "" {
		if r.Manual {
			msgBox(hMain, "Could not check for updates:\n\n"+r.Err, appTitle, MB_ICONWARNING)
		}
		return
	}
	if versionNewer(r.Tag, appVersion) {
		if confirm(hMain, fmt.Sprintf("A new version of the launcher is available: %s (you have v%s).\n\nOpen the download page?", r.Tag, appVersion), appTitle) {
			pShellExecuteW.Call(hMain, u16p("open"), u16p(r.URL), 0, 0, SW_SHOW)
		}
	} else if r.Manual {
		msgBox(hMain, "You have the latest version (v"+appVersion+").", appTitle, MB_ICONINFO)
	}
}
