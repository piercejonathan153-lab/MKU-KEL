package main

// Codec for MKKE's Config\coalesced.ini (zlib + XOR 0xFF, length-prefixed strings).
// Mirrors the Python codec used during modding (loc3.py).

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"
)

type coalItem struct {
	kind byte // 'u' utf16, 'b' byte-counted, 'c' char-counted utf8
	data []byte
}

type Coalesced struct {
	head  []byte
	items []coalItem
}

func coalOK(z []byte, p int) bool {
	return p == len(z) || (p+7 <= len(z) && string(z[p+4:p+7]) == `..\`)
}

func LoadCoalesced(path string) (*Coalesced, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(raw) < 8 {
		return nil, errors.New("coalesced.ini too small")
	}
	zr, err := zlib.NewReader(bytes.NewReader(raw[4:]))
	if err != nil {
		return nil, err
	}
	z, err := io.ReadAll(zr)
	if err != nil {
		return nil, err
	}
	for i := range z {
		z[i] ^= 0xFF
	}
	c := &Coalesced{head: append([]byte{}, z[:4]...)}
	o := 4
	for o < len(z) {
		if o+4 > len(z) {
			return nil, errors.New("coalesced.ini truncated")
		}
		L := int(int32(binary.LittleEndian.Uint32(z[o:])))
		o += 4
		switch {
		case L < 0:
			n := -2 * L
			if o+n > len(z) {
				return nil, errors.New("bad utf16 item")
			}
			c.items = append(c.items, coalItem{'u', append([]byte{}, z[o:o+n]...)})
			o += n
		case len(c.items)%2 == 0 || (o+L <= len(z) && coalOK(z, o+L)):
			if o+L > len(z) {
				return nil, errors.New("bad item")
			}
			c.items = append(c.items, coalItem{'b', append([]byte{}, z[o:o+L]...)})
			o += L
		default:
			p := o
			for n := 0; n < L; n++ {
				if p >= len(z) {
					return nil, errors.New("bad char item")
				}
				ch := z[p]
				switch {
				case ch < 0x80:
					p++
				case ch>>5 == 6:
					p += 2
				case ch>>4 == 14:
					p += 3
				default:
					p += 4
				}
			}
			if !coalOK(z, p) {
				return nil, errors.New("char item mismatch")
			}
			c.items = append(c.items, coalItem{'c', append([]byte{}, z[o:p]...)})
			o = p
		}
	}
	return c, nil
}

func (c *Coalesced) Save(path string) error {
	var out bytes.Buffer
	out.Write(c.head)
	var tmp [4]byte
	for _, it := range c.items {
		var n int32
		switch it.kind {
		case 'u':
			n = -int32(len(it.data) / 2)
		case 'c':
			n = int32(utf8.RuneCount(it.data))
		default:
			n = int32(len(it.data))
		}
		binary.LittleEndian.PutUint32(tmp[:], uint32(n))
		out.Write(tmp[:])
		out.Write(it.data)
	}
	z := out.Bytes()
	for i := range z {
		z[i] ^= 0xFF
	}
	var comp bytes.Buffer
	binary.LittleEndian.PutUint32(tmp[:], uint32(len(z)))
	comp.Write(tmp[:])
	w, _ := zlib.NewWriterLevel(&comp, zlib.BestCompression)
	w.Write(z)
	w.Close()
	return os.WriteFile(path, comp.Bytes(), 0644)
}

// fileText returns the index of the text item following the file-name item that ends with name.
func (c *Coalesced) fileText(name string) int {
	for i := 0; i+1 < len(c.items); i += 2 {
		n := strings.TrimRight(string(c.items[i].data), "\x00")
		if strings.HasSuffix(strings.ToLower(n), strings.ToLower(name)) {
			return i + 1
		}
	}
	return -1
}

// Texture groups whose caps the "HD textures" option raises (vanilla value -> unlocked value).
var hdGroups = map[string][2]string{
	"TEXTUREGROUP_Character":          {"1024", "4096"},
	"TEXTUREGROUP_CharacterNormalMap": {"2048", "4096"},
	"TEXTUREGROUP_World":              {"512", "4096"},
	"TEXTUREGROUP_WorldNormalMap":     {"1024", "4096"},
	"TEXTUREGROUP_Floor":              {"1024", "4096"},
	"TEXTUREGROUP_Effects":            {"512", "2048"},
	"TEXTUREGROUP_LightAndShadowMap":  {"1024", "2048"},
}

var lodRe = regexp.MustCompile(`(?m)^(TEXTUREGROUP_[A-Za-z]+)=\(MinLODSize=(\d+),MaxLODSize=(\d+)`)

// SetHDTextures raises (or restores) the texture caps in both MK9Engine.ini and PC-MK9Engine.ini.
// Returns whether anything changed.
func (c *Coalesced) SetHDTextures(on bool) bool {
	changed := false
	for _, f := range []string{`\PC-MK9Engine.ini`, `\MK9Engine.ini`} {
		i := c.fileText(f)
		if i < 0 {
			continue
		}
		t := string(c.items[i].data)
		nt := lodRe.ReplaceAllStringFunc(t, func(m string) string {
			sm := lodRe.FindStringSubmatch(m)
			v, ok := hdGroups[sm[1]]
			if !ok {
				return m
			}
			want := v[0]
			if on {
				want = v[1]
			}
			return sm[1] + "=(MinLODSize=" + sm[2] + ",MaxLODSize=" + want
		})
		if nt != t {
			c.items[i].data = []byte(nt)
			changed = true
		}
	}
	return changed
}

func (c *Coalesced) HDTexturesOn() bool {
	i := c.fileText(`\PC-MK9Engine.ini`)
	if i < 0 {
		return false
	}
	for _, sm := range lodRe.FindAllStringSubmatch(string(c.items[i].data), -1) {
		if v, ok := hdGroups[sm[1]]; ok && sm[3] == v[1] {
			return true
		}
	}
	return false
}
