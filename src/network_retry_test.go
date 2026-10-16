package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const redditDisconnectError = "ERROR: unable to download video data: ('Connection aborted.', RemoteDisconnected('Remote end closed connection without response'))"

func TestRedditDisconnectIsRetryableButNotAuth(t *testing.T) {
	for _, msg := range []string{
		redditDisconnectError,
		"ERROR: ConnectionResetError(104, 'Connection reset by peer')",
		"ERROR: read timed out",
	} {
		if !shouldRetryReddit(msg) || !isRedditConnectionDrop(msg) {
			t.Fatalf("missed network recovery for %q", msg)
		}
		if isRedditAccessBlock(msg) {
			t.Fatalf("network error classified as auth block: %q", msg)
		}
		out := friendlyError(msg)
		if !strings.Contains(out, "연결을 끊") {
			t.Fatalf("message did not explain network drop: %q", out)
		}
	}
	if shouldRetryReddit("ERROR: HTTP 429 Too Many Requests") {
		t.Fatal("429 must never retry")
	}
}

func TestRedditNetworkFallbackOptions(t *testing.T) {
	app := newApp(t.TempDir(), t.TempDir())
	reddit := &Job{Host: "www.reddit.com", URL: "https://www.reddit.com/comments/abc/"}
	normal := strings.Join(app.downloadArgsForAttempt(reddit, false), " ")
	fallback := strings.Join(app.downloadArgsForAttempt(reddit, false, true), " ")
	for _, option := range []string{"--retries 6", "--fragment-retries 6", "--retry-sleep http:2", "--retry-sleep fragment:2"} {
		if !strings.Contains(normal, option) || !strings.Contains(fallback, option) {
			t.Fatalf("missing recovery setting %q: %q", option, fallback)
		}
	}
	if strings.Contains(normal, "--force-ipv4") || !strings.Contains(fallback, "--force-ipv4") {
		t.Fatalf("IPv4 should only be forced for network fallback: %q | %q", normal, fallback)
	}
	for _, args := range []string{normal, fallback} {
		if strings.Contains(args, "--impersonate") || strings.Contains(args, "--cookies") {
			t.Fatalf("network fallback must not read browser cookies or impersonate: %q", args)
		}
	}
	youtube := &Job{Host: "youtube.com", URL: "https://youtube.com/watch?v=x"}
	if strings.Contains(strings.Join(app.downloadArgsForAttempt(youtube, false, true), " "), "--force-ipv4") {
		t.Fatal("must not change unrelated websites")
	}
}

func TestRedditConnectionAbortAutomaticallyRecoversOnIPv4(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix shell stub")
	}
	tmp := t.TempDir()
	tool := filepath.Join(tmp, "yt-dlp-stub")
	attempts := filepath.Join(tmp, "attempts.txt")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "` + attempts + `"
case " $* " in
 *" --force-ipv4 "*)
    printf 'CLIP_TITLE|bird video\n'
    printf 'CLIP_FILE|/tmp/bird.mp4\n'
    exit 0 ;;
 *)
    printf "ERROR: unable to download video data: ('Connection aborted.', RemoteDisconnected('Remote end closed connection without response'))\n" >&2
    exit 1 ;;
esac
`
	if err := os.WriteFile(tool, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	app := newApp(tmp, filepath.Join(tmp, "downloads"))
	app.ytDlp = tool
	app.state.Jobs = []*Job{{ID: "reddit", Host: "www.reddit.com", URL: "https://www.reddit.com/r/aww/comments/abc/", Status: "queued"}}
	app.runDownload("reddit")
	j := app.getJob("reddit")
	if j.Status != "done" || j.File != "/tmp/bird.mp4" || j.AuthRequired {
		t.Fatalf("network fallback failed: %+v", j)
	}
	lines, err := os.ReadFile(attempts)
	if err != nil {
		t.Fatal(err)
	}
	arr := strings.Split(strings.TrimSpace(string(lines)), "\n")
	if len(arr) != 2 || strings.Contains(arr[0], "--force-ipv4") || !strings.Contains(arr[1], "--force-ipv4") || strings.Contains(arr[1], "--impersonate") {
		t.Fatalf("unexpected attempts: %q", string(lines))
	}
}

func TestPersistentConnectionAbortNeverPromptsForLogin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix shell stub")
	}
	tmp := t.TempDir()
	tool := filepath.Join(tmp, "yt-dlp-stub")
	attempts := filepath.Join(tmp, "attempts.txt")
	script := `#!/bin/sh
printf 'attempt\n' >> "` + attempts + `"
printf "ERROR: unable to download video data: ('Connection aborted.', RemoteDisconnected('Remote end closed connection without response'))\n" >&2
exit 1
`
	if err := os.WriteFile(tool, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	app := newApp(tmp, filepath.Join(tmp, "downloads"))
	app.ytDlp = tool
	app.state.Jobs = []*Job{{ID: "reddit", Host: "www.reddit.com", URL: "https://www.reddit.com/comments/abc/", Status: "queued"}}
	app.runDownload("reddit")
	got := app.getJob("reddit")
	if got.Status != "error" || got.AuthRequired || !strings.Contains(got.Detail, "영상 서버가 연결을 끊") {
		t.Fatalf("wrong failure result: %+v", got)
	}
	b, _ := os.ReadFile(attempts)
	if strings.Count(string(b), "attempt") != 2 {
		t.Fatalf("expected exactly two attempts, got %q", string(b))
	}
}
