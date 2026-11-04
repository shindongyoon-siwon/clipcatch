package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

func clipSelectorValid(s string) bool {
	parts := strings.Split(s, "+")
	if len(parts) > 2 {
		return false
	}
	for _, p := range parts {
		if !clipIDRE.MatchString(p) {
			return false
		}
	}
	return len(parts) > 0
}
func (a *App) clipRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/clip-preview", a.clipPreview)
	mux.HandleFunc("POST /api/clip-queue", a.clipQueue)
	mux.HandleFunc("GET /sections.js", func(w http.ResponseWriter, r *http.Request) {
		b, e := pages.ReadFile("web/sections.js")
		if e != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Write(b)
	})
}

// Build a distinct unpacked helper only in the isolated section candidate.
// No stable extension directory, pairing state or metadata is ever modified.
func sectionExtensionAsset(name string, b []byte) []byte {
	if !isSectionBuild() {
		return b
	}
	s := string(b)
	switch name {
	case "manifest.json":
		var m map[string]any
		if json.Unmarshal(b, &m) != nil {
			return b
		}
		m["name"] = "ClipCatch Browser SECTION TEST"
		m["version"] = "2.1.0"
		m["version_name"] = appVersion
		v, e := json.MarshalIndent(m, "", "  ")
		if e == nil {
			return v
		}
	case "background.js":
		s = strings.ReplaceAll(s, "'stable'", "'section-test'")
		s = strings.ReplaceAll(s, "app!=='clipcatch'", "app!=='clipcatch-section-test'")
		s = strings.Replace(s, "if(['18765','18766'].includes(new URL(code.base).port))", "if(new URL(code.base).port!=='18767')", 1)
	case "popup.js":
		s = strings.ReplaceAll(s, "channel!=='stable'", "channel!=='section-test'")
	case "popup.html":
		s = strings.Replace(s, "<b>ClipCatch </b>", "<b>SECTION TEST </b>", 1)
	}
	return []byte(s)
}
