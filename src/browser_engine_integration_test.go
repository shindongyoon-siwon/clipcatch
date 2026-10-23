package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Tests the actual yt-dlp + FFmpeg path against tiny synthetic HTTP media.
// A test-only wrapper maps the allowlisted example CDN to the fixture server;
// production URL validation is neither changed nor bypassed by this test code.
func TestBrowserRealEngine(t *testing.T) {
	if os.Getenv("CLIPCATCH_REAL_ENGINE_TEST") != "1" {
		t.Skip("opt-in real yt-dlp fixture test")
	}
	if runtime.GOOS == "windows" {
		t.Skip("Unix fixture wrapper")
	}
	ffmpeg, e := exec.LookPath("ffmpeg")
	if e != nil {
		t.Fatal(e)
	}
	py, e := exec.LookPath("python3")
	if e != nil {
		t.Fatal(e)
	}
	if e = exec.Command(py, "-c", "import yt_dlp").Run(); e != nil {
		t.Fatal("yt-dlp Python module missing")
	}
	d := t.TempDir()
	media := filepath.Join(d, "media")
	if e = os.MkdirAll(media, 0700); e != nil {
		t.Fatal(e)
	}
	run := func(args ...string) {
		t.Helper()
		out, e := exec.Command(ffmpeg, append([]string{"-hide_banner", "-loglevel", "error"}, args...)...).CombinedOutput()
		if e != nil {
			t.Fatal(e, string(out))
		}
	}
	muxed := filepath.Join(media, "muxed.mp4")
	run("-f", "lavfi", "-i", "testsrc2=size=160x90:rate=10", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=44100", "-t", "1", "-c:v", "libx264", "-c:a", "aac", "-shortest", muxed)
	run("-i", muxed, "-an", "-c:v", "copy", filepath.Join(media, "DASH_720.mp4"))
	run("-i", muxed, "-vn", "-c:a", "copy", filepath.Join(media, "DASH_AUDIO_128.mp4"))
	run("-i", muxed, "-c", "copy", "-hls_time", "1", "-hls_list_size", "0", filepath.Join(media, "HLSPlaylist.m3u8"))
	server := httptest.NewServer(http.FileServer(http.Dir(media)))
	defer server.Close()
	wrapper := filepath.Join(d, "test-yt-dlp")
	script := fmt.Sprintf(`#!%s
import sys,subprocess,json,tempfile,os
args=sys.argv[1:]
prefix='https://v.redd.it/abcdefghijk12/'
base=%q+'/'
tmp=None
try:
    if '--load-info-json' in args:
        n=args.index('--load-info-json')+1
        with open(args[n],encoding='utf-8') as f: data=json.load(f)
        for item in data['formats']:
            assert item['url'].startswith(prefix)
            item['url']=base+item['url'][len(prefix):]
        f=tempfile.NamedTemporaryFile(mode='w',suffix='.json',delete=False)
        tmp=f.name;json.dump(data,f);f.close();args[n]=tmp
    else:
        assert args[-1].startswith(prefix)
        args[-1]=base+args[-1][len(prefix):]
    sys.exit(subprocess.call([sys.executable,'-m','yt_dlp']+args))
finally:
    if tmp: os.unlink(tmp)
`, py, server.URL)
	if e = os.WriteFile(wrapper, []byte(script), 0700); e != nil {
		t.Fatal(e)
	}
	cases := []browserMedia{
		{PageURL: testPostURL, Title: "Manifest sample", MediaURL: "https://v.redd.it/abcdefghijk12/HLSPlaylist.m3u8"},
		{PageURL: testPostURL, Title: "Separate audio sample", MediaURL: "https://v.redd.it/abcdefghijk12/DASH_720.mp4", AudioURL: "https://v.redd.it/abcdefghijk12/DASH_AUDIO_128.mp4"},
	}
	for i, source := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			a := newApp(filepath.Join(d, fmt.Sprint(i)), filepath.Join(d, fmt.Sprint(i), "downloads"))
			a.ytDlp = wrapper
			a.ffmpeg = ffmpeg
			a.state.Phase = "ready"
			j := &Job{ID: fmt.Sprint(i), URL: testPostURL, Host: "www.reddit.com", BrowserSource: true, Status: "queued"}
			a.state.Jobs = []*Job{j}
			a.browser.Sources[j.ID] = source
			a.runDownload(j.ID)
			got := a.getJob(j.ID)
			if got.Status != "done" {
				t.Fatalf("actual engine failed: %+v", got)
			}
			if !nonempty(got.File) || !strings.Contains(got.Detail, "\uc74c\uc131") {
				t.Fatal("missing verified video/audio")
			}
		})
	}
}
