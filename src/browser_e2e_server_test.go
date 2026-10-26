package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// Opt-in local fixture server, never compiled into ClipCatch.exe.
// Its fake download process is intentionally NOT a live Reddit download test.
func TestBrowserE2EServer(t *testing.T) {
	ready := os.Getenv("CLIPCATCH_E2E_READY")
	if ready == "" {
		t.Skip("local browser fixture server only")
	}
	if runtime.GOOS == "windows" {
		t.Skip("Unix fixture engine")
	}
	ffmpeg, e := exec.LookPath("ffmpeg")
	if e != nil {
		t.Fatal(e)
	}
	d := t.TempDir()
	file := filepath.Join(d, "test-video.mp4")
	if out, e := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=10", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=44100", "-t", "0.5", "-c:v", "libx264", "-c:a", "aac", "-shortest", file).CombinedOutput(); e != nil {
		t.Fatal(e, string(out))
	}
	engine := filepath.Join(d, "fixture-engine")
	script := "#!/bin/sh\n" +
		"dest=\u0027\u0027\n" +
		"while [ \"$#\" -gt 0 ]; do\n" +
		"  if [ \"$1\" = \"-P\" ]; then shift; dest=\"$1\"; fi\n" +
		"  shift\n" +
		"done\n" +
		"[ -n \"$dest\" ] || exit 3\n" +
		"cp " + fmt.Sprintf("%q", file) + " \"$dest/fixture-video.mp4\" || exit 4\n" +
		"printf 'CLIP_TITLE|Browser test video\\n'\n" +
		"printf 'CLIP_PROGRESS|100%%|1MiB/s|00:00\\n'\n" +
		"printf 'CLIP_FILE|%s\\n' \"$dest/fixture-video.mp4\"\n"
	if e = os.WriteFile(engine, []byte(script), 0700); e != nil {
		t.Fatal(e)
	}
	a := newApp(d, d)
	a.state.Phase = "ready"
	a.state.AppUpdate.Status = "current"
	// Page-open/focus checks must stay inside this synthetic browser fixture.
	release := discoveryRelease(t, appVersion)
	hint := discoveryHint(t, appVersion)
	a.updateHooks = updateCheckHooks{
		probe: func(context.Context) (updateHint, error) { return hint, nil },
		fetch: func(context.Context) (verifiedRelease, error) { return release, nil },
	}
	a.ytDlp = engine
	a.ffmpeg = ffmpeg
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	a.server = &http.Server{Handler: http.HandlerFunc(a.serveHTTP), ReadHeaderTimeout: 5 * time.Second}
	defer a.server.Close()
	go a.worker()
	if e = os.WriteFile(ready, []byte("http://"+l.Addr().String()), 0600); e != nil {
		t.Fatal(e)
	}
	fmt.Println("BROWSER_E2E_READY")
	timer := time.AfterFunc(100*time.Second, func() { a.server.Close() })
	defer timer.Stop()
	if e = a.server.Serve(l); e != nil && e != http.ErrServerClosed {
		t.Fatal(e)
	}
}
