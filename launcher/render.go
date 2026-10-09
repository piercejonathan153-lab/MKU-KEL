//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Renderer indexes: 0 = DirectX 9 (no wrapper), 1 = dgVoodoo2 DX11, 2 = Microsoft D3D9On12 (DX12), 3 = DXVK (Vulkan)
const rendVulkan = 3
const rendD3D12 = 2

func d3d9Active() string   { return filepath.Join(exeDir, "D3D9.dll") }
func d3d9DgVoodoo() string { return filepath.Join(exeDir, "D3D9.dgvoodoo.dll") }
func d3d9DXVK() string     { return filepath.Join(exeDir, "D3D9.dxvk.dll") }
func d3d9On12() string     { return filepath.Join(exeDir, "D3D9.on12.dll") }
func on12Available() bool  { return exists(d3d9On12()) }
func dxvkConfPath() string { return filepath.Join(exeDir, "dxvk.conf") }

func isDXVKFile(p string) bool {
	b, err := os.ReadFile(p)
	return err == nil && strings.Contains(string(b), "dxvk.hud")
}

// prepareRendererFiles makes sure the stored copies (D3D9.dgvoodoo.dll / D3D9.dxvk.dll) exist,
// migrating from the older D3D9.dll / D3D9.dll.off layout.
func prepareRendererFiles() {
	copyFile := func(src, dst string) {
		if b, err := os.ReadFile(src); err == nil {
			os.WriteFile(dst, b, 0644)
		}
	}
	for _, p := range []string{d3d9Active(), d3d9Active() + ".off"} {
		if !exists(p) {
			continue
		}
		if isOn12File(p) {
			if !exists(d3d9On12()) {
				copyFile(p, d3d9On12())
			}
		} else if isDXVKFile(p) {
			if !exists(d3d9DXVK()) {
				copyFile(p, d3d9DXVK())
			}
		} else if !exists(d3d9DgVoodoo()) {
			copyFile(p, d3d9DgVoodoo())
		}
	}
}

func dgvAvailable() bool  { return exists(d3d9DgVoodoo()) && exists(confPath) }
func dxvkAvailable() bool { return exists(d3d9DXVK()) }

func rendererName(r int) string { return rendererNames[clamp(r, 0, len(rendererNames)-1)] }

// switchRenderer puts the right D3D9.dll in place (or removes it for plain DirectX 9).
func switchRenderer(r int) error {
	act := d3d9Active()
	ours := func() bool {
		m := fileMD5(act)
		return m != "" && (m == fileMD5(d3d9DgVoodoo()) || m == fileMD5(d3d9DXVK()) || m == fileMD5(d3d9On12()) || isOn12File(act))
	}
	if r == 0 {
		if exists(act) {
			if !ours() {
				return fmt.Errorf("D3D9.dll in the game folder is not one of the launcher's renderers, so it was left alone")
			}
			if err := os.Remove(act); err != nil {
				return fmt.Errorf("could not switch to DirectX 9 (is the game still running?): %v", err)
			}
		}
		return nil
	}
	src := d3d9DgVoodoo()
	if r == rendVulkan {
		src = d3d9DXVK()
	} else if r == rendD3D12 {
		src = d3d9On12()
	}
	if !exists(src) {
		return fmt.Errorf("%s is missing from the game folder, so %s is not available", filepath.Base(src), rendererName(r))
	}
	if fileMD5(act) == fileMD5(src) {
		return nil
	}
	if exists(act) && !ours() {
		return fmt.Errorf("D3D9.dll in the game folder is not one of the launcher's renderers, so it was not replaced")
	}
	b, err := os.ReadFile(src)
	if err == nil {
		err = os.WriteFile(act, b, 0644)
	}
	if err != nil {
		return fmt.Errorf("could not switch renderer (is the game still running?): %v", err)
	}
	logf("renderer files: %s active", filepath.Base(src))
	return nil
}

func writeDXVKConf() error {
	fps := fpsValues[clamp(settings.FPS, 0, len(fpsValues)-1)]
	vs := 0
	if settings.VSync {
		vs = 1
	}
	af := "-1"
	if v := forceAFValues[clamp(settings.ForceAF, 0, len(forceAFValues)-1)]; v != "appdriven" && v != "bilinear" && v != "trilinear" {
		af = v
	}
	hud := ""
	if settings.Watermark {
		hud = "fps,devinfo"
	}
	txt := strings.Join([]string{
		"# dxvk.conf - written by the " + appTitle,
		"# Changes made here are overwritten when you click APPLY SETTINGS or START GAME.",
		fmt.Sprintf("d3d9.maxFrameRate = %d", fps),
		fmt.Sprintf("d3d9.presentInterval = %d", vs),
		"d3d9.samplerAnisotropy = " + af,
		"d3d9.maxAvailableMemory = 4096",
		"dxvk.hud = " + hud,
		"",
	}, "\r\n")
	return os.WriteFile(dxvkConfPath(), []byte(txt), 0644)
}

func preferredRenderer() int {
	if dxvkAvailable() {
		return rendVulkan
	}
	return 1
}

// isOn12File recognises any build of the launcher's DirectX 12 proxy (older test builds included).
func isOn12File(p string) bool {
	b, err := os.ReadFile(p)
	return err == nil && len(b) < 64*1024 && strings.Contains(string(b), "d3d9on12_proxy.log")
}
