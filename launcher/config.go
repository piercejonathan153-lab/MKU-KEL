package main

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// ---------- generic helpers (line-based, keep original line endings) ----------

func splitLines(s string) ([]string, string) {
	nl := "\n"
	if strings.Contains(s, "\r\n") {
		nl = "\r\n"
	}
	return strings.Split(s, nl), nl
}

// ---------- options.ini (%APPDATA%\MKKE\options.ini) ----------

type Options struct {
	Path       string
	lines      []string
	nl         string
	ResX, ResY int
	Letterbox  bool
}

var resRe = regexp.MustCompile(`\{\s*(\d+)\s*,\s*(\d+)\s*\}`)

func optKey(line string) string {
	i := strings.Index(line, "=")
	if i < 0 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(line[:i]))
}

func optVal(line string) string {
	i := strings.Index(line, "=")
	if i < 0 {
		return ""
	}
	return strings.TrimSpace(line[i+1:])
}

func LoadOptions(path string) *Options {
	o := &Options{Path: path, nl: "\r\n", ResX: 0, ResY: 0, Letterbox: true}
	b, err := os.ReadFile(path)
	if err != nil {
		o.lines = []string{"[Video]", "resolution\t\t\t = {1920, 1080}", "letterboxing\t\t = true", "configured\t\t\t = true", ""}
		return o
	}
	o.lines, o.nl = splitLines(string(b))
	for _, l := range o.lines {
		switch optKey(l) {
		case "resolution":
			if m := resRe.FindStringSubmatch(l); m != nil {
				o.ResX, _ = strconv.Atoi(m[1])
				o.ResY, _ = strconv.Atoi(m[2])
			}
		case "letterboxing":
			o.Letterbox = strings.EqualFold(optVal(l), "true")
		}
	}
	return o
}

func (o *Options) GetInt(key string, def int) int {
	for _, l := range o.lines {
		if optKey(l) == key {
			v := strings.TrimSpace(optVal(l))
			if n, err := strconv.Atoi(v); err == nil {
				return n
			}
			if strings.EqualFold(v, "true") {
				return 1
			}
			if strings.EqualFold(v, "false") {
				return 0
			}
		}
	}
	return def
}

func (o *Options) SetRaw(key, val string) { o.set(key, val) }

func (o *Options) set(key, val string) {
	for i, l := range o.lines {
		if optKey(l) == key {
			eq := strings.Index(l, "=")
			o.lines[i] = l[:eq+1] + " " + val
			return
		}
	}
	// insert after [Video]
	for i, l := range o.lines {
		if strings.EqualFold(strings.TrimSpace(l), "[Video]") {
			nl := append([]string{}, o.lines[:i+1]...)
			nl = append(nl, key+"\t\t = "+val)
			o.lines = append(nl, o.lines[i+1:]...)
			return
		}
	}
	o.lines = append([]string{"[Video]", key + "\t\t = " + val}, o.lines...)
}

func (o *Options) remove(key string) {
	out := o.lines[:0:0]
	for _, l := range o.lines {
		if optKey(l) != key {
			out = append(out, l)
		}
	}
	o.lines = out
}

// Save writes the file. windowed=true adds "fullscreen = false" (honoured by the
// patched MKKE.exe) and makes the file read-only so the game cannot strip it.
func (o *Options) Save(windowed bool) error {
	o.set("resolution", fmt.Sprintf("{%d, %d}", o.ResX, o.ResY))
	if o.Letterbox {
		o.set("letterboxing", "true")
	} else {
		o.set("letterboxing", "false")
	}
	if windowed {
		o.set("fullscreen", "false")
	} else {
		o.remove("fullscreen")
	}
	setReadOnly(o.Path, false)
	if err := os.WriteFile(o.Path, []byte(strings.Join(o.lines, o.nl)), 0644); err != nil {
		return err
	}
	if windowed {
		setReadOnly(o.Path, true)
	}
	return nil
}

// ---------- dgVoodoo.conf (section aware) ----------

type Conf struct {
	Path  string
	lines []string
	nl    string
}

func LoadConf(path string) (*Conf, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c := &Conf{Path: path}
	c.lines, c.nl = splitLines(string(b))
	return c, nil
}

func (c *Conf) Set(section, key, val string) bool {
	in := false
	for i, l := range c.lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			in = strings.EqualFold(t, "["+section+"]")
			continue
		}
		if !in || strings.HasPrefix(t, ";") {
			continue
		}
		eq := strings.Index(l, "=")
		if eq < 0 || !strings.EqualFold(strings.TrimSpace(l[:eq]), key) {
			continue
		}
		c.lines[i] = l[:eq+1] + " " + val
		return true
	}
	return false
}

func (c *Conf) Save() error {
	return os.WriteFile(c.Path, []byte(strings.Join(c.lines, c.nl)), 0644)
}

// ---------- launcher settings (MKKE_Launcher.ini next to the exe) ----------

type Settings struct {
	Renderer  int // 0=DX9 1=DX11 2=DX12
	Mode      int // 0=Fullscreen 1=Borderless 2=Windowed
	FPS       int // index into fpsValues
	VSync     bool
	Ryzen     bool
	ForceAA   int // 0=game 1=2x 2=4x 3=8x
	ForceAF   int // 0=game 1=16x
	SkipIntro bool
	Watermark bool
	Monitor   int // index into monitors
	LastTab   int
	HDTex     bool

	// launcher preferences (SETTINGS tab)
	Theme        int // 0=Light 1=Dark 2=Match Windows
	Tooltips     bool
	RememberTab  bool
	AskSave      bool
	ConfirmReset bool
	AfterLaunch  int // 0=close 1=reopen when the game closes
	QuickLaunch  bool
	LogDays      int // index into logDayValues
	Accent       int // index into accentColors (0 = theme default)
	TextColor    int // index into textColors (0 = theme default)
	SkipLogo     bool
	HighPriority bool
	DisableFSO   bool
	CheckUpdates bool
	UpdateRepo   string
}

var logDayValues = []int{1, 3, 7, 30}

func LoadSettings(path string) Settings {
	s := Settings{Renderer: 3, Mode: 1, FPS: 0, VSync: true,
		Theme: 2, Tooltips: true, CheckUpdates: true, RememberTab: true, AskSave: true, ConfirmReset: true, LogDays: 1}
	b, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	lines, _ := splitLines(string(b))
	for _, l := range lines {
		k, v := optKey(l), optVal(l)
		n, _ := strconv.Atoi(v)
		switch k {
		case "renderer":
			s.Renderer = n
		case "displaymode":
			s.Mode = n
		case "fpslimit":
			s.FPS = n
		case "vsync":
			s.VSync = v == "1"
		case "ryzenfix":
			s.Ryzen = v == "1"
		case "forceaa":
			s.ForceAA = n
		case "forceaf":
			s.ForceAF = n
		case "skipintro":
			s.SkipIntro = v == "1"
		case "watermark":
			s.Watermark = v == "1"
		case "monitor":
			s.Monitor = n
		case "lasttab":
			s.LastTab = n
		case "hdtextures":
			s.HDTex = v == "1"
		case "theme":
			s.Theme = n
		case "tooltips":
			s.Tooltips = v == "1"
		case "remembertab":
			s.RememberTab = v == "1"
		case "asksave":
			s.AskSave = v == "1"
		case "confirmreset":
			s.ConfirmReset = v == "1"
		case "afterlaunch":
			s.AfterLaunch = n
		case "quicklaunch":
			s.QuickLaunch = v == "1"
		case "logdays":
			s.LogDays = n
		case "accent":
			s.Accent = n
		case "textcolor":
			s.TextColor = n
		case "skiplogo":
			s.SkipLogo = v == "1"
		case "highpriority":
			s.HighPriority = v == "1"
		case "disablefso":
			s.DisableFSO = v == "1"
		case "checkupdates":
			s.CheckUpdates = v == "1"
		case "updaterepo":
			s.UpdateRepo = v
		}
	}
	return s
}

func (s Settings) Save(path string) error {
	b2i := func(b bool) int {
		if b {
			return 1
		}
		return 0
	}
	txt := fmt.Sprintf("[Launcher]\r\nRenderer=%d\r\nDisplayMode=%d\r\nFPSLimit=%d\r\nVSync=%d\r\nRyzenFix=%d\r\nForceAA=%d\r\nForceAF=%d\r\nSkipIntro=%d\r\nWatermark=%d\r\nMonitor=%d\r\nLastTab=%d\r\nHDTextures=%d\r\n\r\n[Preferences]\r\nTheme=%d\r\nTooltips=%d\r\nRememberTab=%d\r\nAskSave=%d\r\nConfirmReset=%d\r\nAfterLaunch=%d\r\nQuickLaunch=%d\r\nLogDays=%d\r\nAccent=%d\r\nTextColor=%d\r\nSkipLogo=%d\r\nHighPriority=%d\r\nDisableFSO=%d\r\nCheckUpdates=%d\r\nUpdateRepo=%s\r\n",
		s.Renderer, s.Mode, s.FPS, b2i(s.VSync), b2i(s.Ryzen), s.ForceAA, s.ForceAF, b2i(s.SkipIntro), b2i(s.Watermark), s.Monitor, s.LastTab, b2i(s.HDTex),
		s.Theme, b2i(s.Tooltips), b2i(s.RememberTab), b2i(s.AskSave), b2i(s.ConfirmReset), s.AfterLaunch, b2i(s.QuickLaunch), s.LogDays, s.Accent, s.TextColor, b2i(s.SkipLogo), b2i(s.HighPriority), b2i(s.DisableFSO), b2i(s.CheckUpdates), s.UpdateRepo)
	return os.WriteFile(path, []byte(txt), 0644)
}
