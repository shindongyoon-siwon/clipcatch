package main

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Only existing directories are handed to the OS. Never execute a shell string.
func checkedFolderPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" || strings.ContainsRune(path, 0) {
		return "", errors.New("invalid folder path")
	}
	full, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(full)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("not a directory")
	}
	return filepath.Clean(full), nil
}
func openDirectory(path string) error {
	full, err := checkedFolderPath(path)
	if err != nil {
		return err
	}
	return openDirectoryNative(full)
}
func (a *App) openSaveFolder(w http.ResponseWriter, r *http.Request) {
	serveOpenSaveFolder(w, r, a.saveDir, openDirectory)
}
func serveOpenSaveFolder(w http.ResponseWriter, r *http.Request, path string, launch func(string) error) {
	fail := func(err error) {
		writeJSON(w, 500, map[string]any{"ok": false, "error": "폴더를 열지 못했어. 아래 경로를 탐색기 주소창에 붙여 넣어 확인해 줘. " + err.Error(), "path": path})
	}
	if err := os.MkdirAll(path, 0755); err != nil {
		fail(err)
		return
	}
	full, err := checkedFolderPath(path)
	if err != nil {
		fail(err)
		return
	}
	if err = launch(full); err != nil {
		fail(err)
		return
	}
	// Shell acceptance is not proof that the window reached the foreground.
	writeJSON(w, 200, map[string]any{"ok": true, "requested": true, "path": full})
}
func (a *App) folderRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /folder-open.js", func(w http.ResponseWriter, r *http.Request) {
		data, err := pages.ReadFile("web/folder-open.js")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(data)
	})
}
