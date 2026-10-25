package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"strings"
)

// Embedded ICC profiles require a color-managed conversion, not just retagging.
// Decline them in this initial local upscaler instead of silently losing color.
func qualityEmbeddedICC(path, ext string) bool {
	f, e := os.Open(path)
	if e != nil {
		return true
	}
	defer f.Close()
	data, e := io.ReadAll(io.LimitReader(f, 2<<20))
	if e != nil {
		return true
	}
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg":
		return bytes.Contains(data, []byte("ICC_PROFILE\x00"))
	case ".webp":
		return len(data) >= 16 && bytes.Contains(data, []byte("ICCP"))
	case ".png":
		if len(data) < 8 {
			return true
		}
		r := bufio.NewReader(bytes.NewReader(data[8:]))
		for i := 0; i < 200; i++ {
			var head [8]byte
			if _, e = io.ReadFull(r, head[:]); e != nil {
				return false
			}
			n := binary.BigEndian.Uint32(head[:4])
			typ := string(head[4:])
			if typ == "iCCP" {
				return true
			}
			if typ == "IDAT" || typ == "IEND" {
				return false
			}
			if n > 2<<20 {
				return true
			}
			if _, e = io.CopyN(io.Discard, r, int64(n)+4); e != nil {
				return false
			}
		}
	}
	return false
}
