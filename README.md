# Mortal Kombat: Ultimate Komplete Edition Launcher

An unofficial modern launcher for **Mortal Kombat Komplete Edition** (PC, Steam v1.07).

**[Download v1.3.1](https://github.com/piercejonathan153-lab/MKU-KEL/raw/main/releases/MKUKE-Launcher-v1.3.1.zip)**  ·  all versions: [`releases/`](releases)

## Features
- Vulkan (DXVK), DirectX 11 / 12 (dgVoodoo2) or the original DirectX 9 renderer
- Fullscreen, Borderless fullscreen and a working Windowed mode
- Any resolution incl. ultrawide, monitor selection
- 60 FPS limiter, VSync, texture filtering / anti-aliasing overrides
- 6 graphics presets: Potato, Console, Vanilla, Optimized, Ultra, Maximum
- Skip intro movies, online matchmaking distance
- Ryzen freeze fix, high CPU priority, disable fullscreen optimizations
- Shareable settings profiles, DLC Manager shortcuts, diagnostics report
- 12 themes (incl. dark modes), custom accent and text colours
- Built-in update checker (reads `latest.json` in this repository)
- Restore Vanilla: undo every change with one click

## Installation
1. Steam → right-click Mortal Kombat Komplete Edition → Manage → Browse local files → open `DiscContentPC`.
2. Rename the original `MKLauncher.exe` to `MKLauncher_Original.exe`.
3. Copy the contents of the release zip's `DiscContentPC` folder into the game's `DiscContentPC` folder.
4. Start the game from Steam.

Recommended: **Borderless + Vulkan (DXVK) + VSync on.**

## Building from source
The launcher is a single Win32 Go program (no external dependencies).
```
cd launcher
GOOS=windows GOARCH=amd64 go build -ldflags "-H windowsgui -s -w" -o MKLauncher.exe .
```
`rsrc_windows_amd64.syso` holds the icon and manifest (made with `rsrc -manifest app.manifest -ico mk.ico`).

## Credits
- Vulkan rendering: [DXVK](https://github.com/doitsujin/dxvk) by Philip Rebohle and contributors (zlib license)
- DirectX 11 / 12 rendering: [dgVoodoo2](http://dege.freeweb.hu/dgVoodoo2/) by Dege
- Design inspired by BmLauncher by neatodev

*Mortal Kombat is a trademark of Warner Bros. Entertainment Inc. This is an unofficial fan project, not affiliated with WB Games or NetherRealm Studios.*

## Publishing an update
1. Bump `appVersion` in `launcher/main.go`, build, and zip the release.
2. Add the zip to `releases/` and update `latest.json` (`version`, `url`, `notes`).
3. Push to `main` - every launcher picks it up on its next update check.
