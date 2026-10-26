package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode/utf16"
)

// yt-dlp's %(field)j emits ASCII JSON, including surrogate pairs for emoji.
func downloaderASCIIJSON(t *testing.T, value string) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	for _, r := range string(data) {
		if r <= 127 {
			out.WriteRune(r)
		} else if r <= 0xffff {
			fmt.Fprintf(&out, "\\u%04x", r)
		} else {
			hi, lo := utf16.EncodeRune(r)
			fmt.Fprintf(&out, "\\u%04x\\u%04x", hi, lo)
		}
	}
	return out.String()
}

func TestDownloaderJSONPreservesUnicodeAndMetadataBoundaries(t *testing.T) {
	cases := []struct {
		name, title, path string
	}{
		{
			"Windows Korean path",
			"댓글 - 10대라는 소개 🐈",
			"C:\\Users\\동윤\\AppData\\Local\\ClipCatch\\download-work\\job-123\\1 - 댓글 [2112159052868489217].mkv",
		},
		{
			"Whitespace quotes and marker-like title",
			"  \"한국어\" | CLIP_PROGRESS|100%|none|0\nCLIP_FILE|not-a-file\\끝  ",
			" /tmp/한글 이름 | \"따옴표\"\\표시\n다음 줄 🐈.mkv ",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := &App{state: ClientState{Jobs: []*Job{{ID: "unicode", Status: "downloading"}}}}
			app.parseOutput("unicode", "CLIP_TITLE_JSON|"+downloaderASCIIJSON(t, tc.title))
			got := app.getJob("unicode")
			if got.Title != tc.title || got.File != "" || got.Percent != 0 || len(got.Media) != 0 {
				t.Fatalf("title was corrupted or interpreted as another record: %+v", got)
			}
			app.parseOutput("unicode", "CLIP_FILE_JSON|"+downloaderASCIIJSON(t, tc.path))
			got = app.getJob("unicode")
			if got.File != tc.path || len(got.Media) != 1 || got.Media[0].Original != tc.path {
				t.Fatalf("path did not survive JSON transport: %+v", got)
			}
		})
	}
}

func TestDownloaderRejectsMalformedJSONMetadata(t *testing.T) {
	payloads := []string{
		"null",
		"[]",
		"42",
		"\"\"",
		"\"unterminated",
		"\"path\" trailing-output",
		"\"unfinished CLIP_FILE|/tmp/false-file.mp4",
		"{\"CLIP_PROGRESS|100%|none|0\":true}",
	}
	for _, payload := range payloads {
		t.Run(payload, func(t *testing.T) {
			app := &App{state: ClientState{Jobs: []*Job{{ID: "unicode", Status: "downloading", Title: "unchanged"}}}}
			app.parseOutput("unicode", "CLIP_FILE_JSON|"+payload)
			app.parseOutput("unicode", "CLIP_TITLE_JSON|"+payload)
			got := app.getJob("unicode")
			if got.File != "" || len(got.Media) != 0 || got.Title != "unchanged" || got.Percent != 0 {
				t.Fatalf("invalid JSON changed metadata: %+v", got)
			}
		})
	}
}

func TestDownloaderUnicodeFilesSurviveProcessPipe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture; Unicode/Windows-path parser cases run on every platform")
	}
	tmp := t.TempDir()
	app := newApp(filepath.Join(tmp, "data"), filepath.Join(tmp, "saved"))
	job := &Job{
		ID: "unicode-pipe", URL: "https://x.com/example/status/2114548960178393202",
		Host: "x.com", WholePost: true, OutputMode: premiereMode, Status: "downloading",
	}
	app.state.Jobs = []*Job{job}
	if err := app.prepareManagedDownload(job); err != nil {
		t.Fatal(err)
	}
	paths := []string{
		filepath.Join(job.DownloadDir, "1 - 댓글 - 10대라는 소개 [2112159052868489217].mkv"),
		filepath.Join(job.DownloadDir, "2 - 한글 고양이 🐈 [2112099550286827521].mkv"),
	}
	for _, path := range paths {
		if err := os.WriteFile(path, []byte("download fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	title := "댓글 \"한글\" | 영상\n다음 줄 🐈"
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
	script := []string{
		"#!/bin/sh",
		"encoding=; json_title=; json_file=",
		"while [ \"$#\" -gt 0 ]; do",
		"  case \"$1\" in",
		"    --encoding) shift; encoding=$1 ;;",
		"    --print) shift; case \"$1\" in",
		"      'before_dl:CLIP_TITLE_JSON|%(title)j') json_title=1 ;;",
		"      'after_move:CLIP_FILE_JSON|%(filepath)j') json_file=1 ;;",
		"    esac ;;",
		"  esac",
		"  shift",
		"done",
		"[ \"$encoding\" = utf-8 ] && [ \"$json_title\" = 1 ] && [ \"$json_file\" = 1 ] || exit 93",
		"printf '%s\\n' " + quote("CLIP_TITLE_JSON|"+downloaderASCIIJSON(t, title)),
	}
	for _, path := range paths {
		script = append(script, "printf '%s\\n' "+quote("CLIP_FILE_JSON|"+downloaderASCIIJSON(t, path)))
	}
	app.ytDlp = filepath.Join(tmp, "fake-dlp")
	if err := os.WriteFile(app.ytDlp, []byte(strings.Join(script, "\n")+"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err, msg := app.runDownloadAttempt(job.ID, job, false); err != nil {
		t.Fatalf("downloader process failed: %v %s", err, msg)
	}
	got := app.getJob(job.ID)
	if got.Title != title || len(got.Media) != len(paths) {
		t.Fatalf("Unicode metadata was lost in the pipe: %+v", got)
	}
	for i, path := range paths {
		if got.Media[i].Original != path || !validPostSource(app.downloadRoot(got), got.Media[i].Original) {
			t.Errorf("recorded file %d no longer resolves to the downloaded file: %q", i, got.Media[i].Original)
		}
	}
}

func downloaderShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

func retryUnicodeFixtureJob(t *testing.T, app *App, id string) {
	t.Helper()
	app.state.Phase = "ready"
	app.state.AppUpdate = UpdateInfo{Status: "current"}
	w := httptest.NewRecorder()
	app.retryFailedHandler(w, httptest.NewRequest(http.MethodPost, "/api/retry-failed", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("retry was rejected: %d %s", w.Code, w.Body.String())
	}
	select {
	case queued := <-app.queue:
		if queued != id {
			t.Fatalf("retried the wrong job: %q", queued)
		}
	default:
		t.Fatal("failed job was not queued for retry")
	}
	app.runDownload(id)
}

func TestDownloaderRetryRebuildsStaleUnicodeMetadata(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix downloader fixture")
	}
	ff, _, fixture := postFixture(t, "libx264", "aac", "mkv")
	video, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	for _, firstSourceExists := range []bool{false, true} {
		t.Run(fmt.Sprintf("first-source-valid=%v", firstSourceExists), func(t *testing.T) {
			tmp := t.TempDir()
			app := newApp(filepath.Join(tmp, "data"), filepath.Join(tmp, "saved"))
			app.ffmpeg = ff
			job := &Job{
				ID: "unicode-retry", URL: "https://x.com/example/status/2114548960178393202", Host: "x.com",
				WholePost: true, OutputMode: premiereMode, Status: "error", ErrorCode: "CC-421",
			}
			app.state.Jobs = []*Job{job}
			if err := app.prepareManagedDownload(job); err != nil {
				t.Fatal(err)
			}
			paths := []string{
				filepath.Join(job.DownloadDir, "1 - 댓글 [2112159052868489217].mkv"),
				filepath.Join(job.DownloadDir, "2 - 한국어 🐈 [2112099550286827521].mkv"),
			}
			for _, path := range paths {
				if err := os.WriteFile(path, video, 0600); err != nil {
					t.Fatal(err)
				}
			}
			part := filepath.Join(job.DownloadDir, "unfinished.part")
			if err := os.WriteFile(part, []byte("preserved partial download"), 0600); err != nil {
				t.Fatal(err)
			}
			bad1 := filepath.Join(job.DownloadDir, "1 - ����.mkv")
			bad2 := filepath.Join(job.DownloadDir, "2 - ����.mkv")
			if firstSourceExists {
				bad1 = paths[0]
			}
			job.OriginalFile = bad1
			job.File = bad2
			job.MediaOverflow = true
			job.Media = []savedPostMedia{
				{Original: bad1, File: bad1, Error: "old path failure"},
				{Original: bad2, File: bad2, Error: "old path failure"},
			}
			script := []string{"#!/bin/sh", "[ -s " + downloaderShellQuote(part) + " ] || exit 90"}
			for _, path := range paths {
				script = append(script,
					"[ -s "+downloaderShellQuote(path)+" ] || exit 91",
					"printf '%s\\n' "+downloaderShellQuote("CLIP_FILE_JSON|"+downloaderASCIIJSON(t, path)))
			}
			app.ytDlp = filepath.Join(tmp, "retry-dlp")
			if err := os.WriteFile(app.ytDlp, []byte(strings.Join(script, "\n")+"\n"), 0700); err != nil {
				t.Fatal(err)
			}
			retryUnicodeFixtureJob(t, app, job.ID)
			got := app.getJob(job.ID)
			if got.Status != "done" || !got.EditReady || len(got.Media) != 2 || got.MediaOverflow || got.DownloadIncomplete || got.ErrorCode != "" {
				t.Fatalf("old metadata survived a new download: %+v", got)
			}
			for _, media := range got.Media {
				if media.Error != "" || !media.EditReady || !nonempty(media.File) || filepath.Ext(media.File) != ".mp4" {
					t.Fatalf("retried output was not prepared: %+v", media)
				}
			}
		})
	}
}

func TestDownloaderAutomaticFallbackRetainsFilesFromFirstAttempt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix downloader fixture")
	}
	ff, folder, first := postFixture(t, "libx264", "aac", "mkv")
	second := filepath.Join(folder, "두 번째.mkv")
	video, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, video, 0600); err != nil {
		t.Fatal(err)
	}
	app := newApp(t.TempDir(), folder)
	app.ffmpeg = ff
	job := &Job{
		ID: "fallback-media", URL: "https://www.reddit.com/comments/abcdef/",
		Host: "www.reddit.com", OutputMode: originalMode, Status: "queued",
	}
	app.state.Jobs = []*Job{job}
	script := strings.Join([]string{
		"#!/bin/sh",
		"case \" $* \" in",
		"  *' --force-ipv4 '*)",
		"    printf '%s\\n' " + downloaderShellQuote("CLIP_FILE_JSON|"+downloaderASCIIJSON(t, second)),
		"    exit 0 ;;",
		"  *)",
		"    printf '%s\\n' " + downloaderShellQuote("CLIP_FILE_JSON|"+downloaderASCIIJSON(t, first)),
		"    printf '%s\\n' " + downloaderShellQuote(redditDisconnectError) + " >&2",
		"    exit 1 ;;",
		"esac",
	}, "\n") + "\n"
	app.ytDlp = filepath.Join(t.TempDir(), "fallback-dlp")
	if err := os.WriteFile(app.ytDlp, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	app.runDownload(job.ID)
	got := app.getJob(job.ID)
	if got.Status != "done" || len(got.Media) != 2 || got.Media[0].Original != first || got.Media[1].Original != second {
		t.Fatalf("automatic fallback lost an already received file: %+v", got)
	}
	if !nonempty(first) || !nonempty(second) {
		t.Fatal("network retry removed an existing source")
	}
}

func TestDownloaderRetryUsesValidSavedSourcesWithoutNetwork(t *testing.T) {
	ff, folder, first := postFixture(t, "libx264", "aac", "mkv")
	second := filepath.Join(folder, "두 번째 원본.mkv")
	video, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, video, 0600); err != nil {
		t.Fatal(err)
	}
	app := newApp(t.TempDir(), folder)
	app.ffmpeg = ff
	// Any accidental network/download branch fails because this executable is absent.
	app.ytDlp = filepath.Join(t.TempDir(), "must-not-run-downloader")
	job := &Job{
		ID: "saved-source-retry", URL: "https://x.com/example/status/2114548960178393202",
		Host: "x.com", WholePost: true, OutputMode: premiereMode, Status: "error",
		OriginalFile: first, ErrorCode: "CC-421",
		Media: []savedPostMedia{
			{Original: first, File: first, Error: "old conversion failure"},
			{Original: second, File: second, Error: "old conversion failure"},
		},
	}
	app.state.Jobs = []*Job{job}
	retryUnicodeFixtureJob(t, app, job.ID)
	got := app.getJob(job.ID)
	if got.Status != "done" || !got.EditReady || len(got.Media) != 2 {
		t.Fatalf("valid saved-source retry did not finish: %+v", got)
	}
	if !nonempty(first) || !nonempty(second) {
		t.Fatal("saved-source conversion retry removed an original")
	}
}
