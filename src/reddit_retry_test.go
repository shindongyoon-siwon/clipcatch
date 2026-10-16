package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRedditAddressNormalization(t *testing.T) {
	url, host, err := cleanURL("https://redd.it/1witx57")
	if err != nil || host != "www.reddit.com" || url != "https://www.reddit.com/comments/1witx57/" {
		t.Fatalf("bad canonical URL: %q %q %v", url, host, err)
	}
	url, host, err = cleanURL("https://v.redd.it/abcDEF")
	if err != nil || host != "v.redd.it" || url != "https://v.redd.it/abcDEF" {
		t.Fatalf("bad raw video URL: %q %q %v", url, host, err)
	}
}

func TestRedditImpersonateOnlyAndNeverCookies(t *testing.T) {
	app := newApp(t.TempDir(), t.TempDir())
	jobs := []*Job{
		{Host: "www.reddit.com", URL: "https://www.reddit.com/comments/abcde/"},
		{Host: "youtube.com", URL: "https://www.youtube.com/watch?v=abcde"},
	}
	for _, j := range jobs {
		standard := strings.Join(app.downloadArgsForAttempt(j, false), " ")
		fallback := strings.Join(app.downloadArgsForAttempt(j, true), " ")
		for _, s := range []string{standard, fallback} {
			if strings.Contains(s, "--cookies") || strings.Contains(s, "--cookies-from-browser") {
				t.Fatalf("cookie access in %q", s)
			}
		}
		if strings.Contains(standard, "--impersonate") {
			t.Fatal("should not impersonate normally")
		}
		if isRedditHost(j.Host) && !strings.Contains(fallback, "--impersonate chrome") {
			t.Fatal("should retry reddit")
		}
		if !isRedditHost(j.Host) && strings.Contains(fallback, "--impersonate") {
			t.Fatal("should not impersonate youtube")
		}
	}
	if !shouldRetryReddit("ERROR: [Reddit] 1witx57: Account authentication is required") {
		t.Fatal("must retry auth error")
	}
	if !shouldRetryReddit("ERROR: HTTP Error 403: Blocked") {
		t.Fatal("must retry block")
	}
	if shouldRetryReddit("HTTP 429 Too Many Requests") {
		t.Fatal("no retry on rate limit")
	}
}

func TestRedditDownloadRetriesOnAuthError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix stub")
	}
	tmp := t.TempDir()
	tool := filepath.Join(tmp, "fake-dlp")
	attempts := filepath.Join(tmp, "attempts.txt")
	stub := `#!/bin/sh
printf 'attempt\n' >> "` + attempts + `"
args=" $* "
case "$args" in
 *" --impersonate chrome "*)
    printf 'CLIP_TITLE|animal video\n'
    printf 'CLIP_FILE|/tmp/animal.mp4\n'
    exit 0 ;;
 *) printf 'ERROR: [Reddit] abcde: Account authentication is required\n' >&2; exit 1 ;;
esac
`
	if err := os.WriteFile(tool, []byte(stub), 0700); err != nil {
		t.Fatal(err)
	}
	app := newApp(tmp, filepath.Join(tmp, "downloads"))
	app.ytDlp = tool
	app.state.Jobs = []*Job{{ID: "reddit", Host: "www.reddit.com", URL: "https://www.reddit.com/r/aww/comments/abcde/", Status: "queued"}}
	app.runDownload("reddit")
	j := app.getJob("reddit")
	if j.Status != "done" || j.File != "/tmp/animal.mp4" {
		t.Fatalf("retry did not recover: %+v", j)
	}
	data, err := os.ReadFile(attempts)
	if err != nil || strings.Count(string(data), "attempt") != 2 {
		t.Fatalf("attempt count: %q %v", string(data), err)
	}
}

func TestNonRedditFailureDoesNotRetry(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix stub")
	}
	tmp := t.TempDir()
	tool := filepath.Join(tmp, "fake-dlp")
	attempts := filepath.Join(tmp, "attempts.txt")
	stub := `#!/bin/sh
printf 'attempt\n' >> "` + attempts + `"
printf 'ERROR: [youtube] HTTP Error 403: Forbidden\n' >&2
exit 1
`
	if err := os.WriteFile(tool, []byte(stub), 0700); err != nil {
		t.Fatal(err)
	}
	app := newApp(tmp, filepath.Join(tmp, "downloads"))
	app.ytDlp = tool
	app.state.Jobs = []*Job{{ID: "yt", Host: "youtube.com", URL: "https://youtube.com/watch?v=abc", Status: "queued"}}
	app.runDownload("yt")
	data, _ := os.ReadFile(attempts)
	if strings.Count(string(data), "attempt") != 1 {
		t.Fatalf("unexpected retry: %q", string(data))
	}
}
