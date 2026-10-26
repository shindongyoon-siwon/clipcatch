package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPostMediaMissingFilenameKeepsCauseAndOriginal(t *testing.T) {
	dataDir, downloadDir := t.TempDir(), t.TempDir()
	original := filepath.Join(downloadDir, "고양이 원본.mp4")
	contents := []byte("original remains untouched")
	if err := os.WriteFile(original, contents, 0600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(downloadDir, "깨진_경로.mp4")
	a := newApp(dataDir, downloadDir)
	a.state.Jobs = []*Job{{ID: "missing-name", Status: "downloading", WholePost: true, OutputMode: premiereMode}}
	a.recordPostFile("missing-name", missing)
	a.finishPostMedia("missing-name", context.Background(), true)
	j := a.getJob("missing-name")
	if j.Status != "error" || j.ErrorCode != "CC-421" || len(j.Media) != 1 || j.Media[0].ErrorCode != "CC-421-MISSING" {
		t.Fatalf("missing file was not classified: %+v", j)
	}
	if j.Media[0].Error != postMediaErrorMessages["CC-421-MISSING"] || !strings.Contains(j.Detail, "0개") {
		t.Fatalf("missing file lost its fixed explanation: %+v", j)
	}
	for _, inaccurate := range []string{"성공한 파일", "원본은 보존했어", "완료된 파일은 그대로"} {
		if strings.Contains(j.ErrorAdvice, inaccurate) {
			t.Fatal("zero successes claimed a preserved output", j.ErrorAdvice)
		}
	}
	if after, err := os.ReadFile(original); err != nil || string(after) != string(contents) {
		t.Fatal("original file changed", err)
	}
	report := buildDiagnosticReport(a.state)
	for _, want := range []string{"최근 오류: CC-421", "파일 처리 요약: 0/1개 완료", "파일 오류 1: CC-421-MISSING"} {
		if !strings.Contains(report, want) {
			t.Fatalf("report omitted %q: %s", want, report)
		}
	}
	if strings.Contains(report, "CC-199") || strings.Contains(report, downloadDir) || strings.Contains(report, filepath.Base(missing)) {
		t.Fatal("diagnostic lost the code or exposed a path", report)
	}
	a.saveHistory()
	reloaded := newApp(dataDir, downloadDir).getJob(j.ID)
	if reloaded == nil || len(reloaded.Media) != 1 || reloaded.Media[0].ErrorCode != "CC-421-MISSING" {
		t.Fatal("per-file cause was lost when history reloaded", reloaded)
	}
}

func TestPostMediaPartialResultPreservesDownloaderClassification(t *testing.T) {
	d := t.TempDir()
	original, output := filepath.Join(d, "source.mp4"), filepath.Join(d, "prepared.mp4")
	for _, p := range []string{original, output} {
		if err := os.WriteFile(p, []byte("verified earlier"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	code, title, advice := classifyDownloadError("Account authentication is required", "x.com")
	a := newApp(t.TempDir(), d)
	a.state.Jobs = []*Job{{
		ID: "partial-cause", Status: "error", WholePost: true, OutputMode: premiereMode,
		DownloadIncomplete: true, ErrorCode: code, ErrorTitle: title, ErrorAdvice: advice, AuthRequired: true,
		Media: []savedPostMedia{
			{Original: original, File: output, EditReady: true},
			{Original: filepath.Join(d, "missing.mp4"), File: filepath.Join(d, "missing.mp4")},
		},
	}}
	a.finishPostMedia("partial-cause", context.Background(), true)
	j := a.getJob("partial-cause")
	if j.Status != "error" || j.ErrorCode != code || j.ErrorTitle != title || j.ErrorAdvice != advice || !j.AuthRequired {
		t.Fatalf("post summary hid the original download cause: %+v", j)
	}
	if !strings.Contains(j.Detail, "1/2개") || j.File != output || j.EditReady || j.Media[1].ErrorCode != "CC-421-MISSING" {
		t.Fatalf("partial result lost its completion summary: %+v", j)
	}
	if !nonempty(original) || !nonempty(output) || allSavedPostFiles(j) {
		t.Fatal("partial originals were removed or the whole post was marked complete")
	}
	report := buildDiagnosticReport(a.state)
	for _, want := range []string{"최근 오류: CC-103", "일부 다운로드 미완료", "파일 처리 요약: 1/2개 완료", "파일 오류 2: CC-421-MISSING"} {
		if !strings.Contains(report, want) {
			t.Fatalf("report omitted %q: %s", want, report)
		}
	}
}

func TestPostMediaNoFilesDoesNotHideDownloaderFailure(t *testing.T) {
	a := newApp(t.TempDir(), t.TempDir())
	code, title, advice := classifyDownloadError("HTTP 403 Forbidden", "x.com")
	a.state.Jobs = []*Job{{ID: "no-files", Status: "error", WholePost: true, DownloadIncomplete: true, ErrorCode: code, ErrorTitle: title, ErrorAdvice: advice}}
	a.finishPostMedia("no-files", context.Background(), true)
	j := a.getJob("no-files")
	if j.ErrorCode != code || j.ErrorTitle != title || j.ErrorAdvice != advice || !strings.Contains(j.Detail, "0개") {
		t.Fatalf("empty result overwrote the classified failure: %+v", j)
	}
}

func TestPostMediaToolFailureHasItsOwnDiagnosticCode(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "source.mp4")
	if err := os.WriteFile(p, []byte("source awaiting inspection"), 0600); err != nil {
		t.Fatal(err)
	}
	a := newApp(t.TempDir(), d)
	a.ffmpeg = ""
	a.state.Jobs = []*Job{{ID: "no-tools", Status: "downloading", WholePost: true, OutputMode: premiereMode, Media: []savedPostMedia{{Original: p, File: p}}}}
	a.finishPostMedia("no-tools", context.Background(), true)
	j := a.getJob("no-tools")
	if j.ErrorCode != "CC-421" || j.Media[0].ErrorCode != "CC-421-TOOLS" || !nonempty(p) {
		t.Fatalf("tool failure was hidden or original lost: %+v", j)
	}
}

func TestPostSourceDiagnosisRetainsFileBoundaryChecks(t *testing.T) {
	root := t.TempDir()
	valid, empty := filepath.Join(root, "valid.mp4"), filepath.Join(root, "empty.mp4")
	outside := filepath.Join(t.TempDir(), "outside.mp4")
	for p, contents := range map[string]string{valid: "video", empty: "", outside: "video"} {
		if err := os.WriteFile(p, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ path, code string }{
		{valid, ""}, {empty, "CC-421-EMPTY"}, {filepath.Join(root, "missing.mp4"), "CC-421-MISSING"},
		{outside, "CC-421-PATH"}, {"relative.mp4", "CC-421-PATH"}, {filepath.Join(root, "unsafe.exe"), "CC-421-FORMAT"},
	} {
		if code := postSourceErrorCode(root, tc.path); code != tc.code || validPostSource(root, tc.path) != (tc.code == "") {
			t.Fatalf("source boundary changed: %s returned %s, want %s", tc.path, code, tc.code)
		}
	}
	t.Run("symlink escape", func(t *testing.T) {
		link := filepath.Join(root, "escape.mp4")
		if err := os.Symlink(outside, link); err != nil {
			t.Skip("symlink creation unavailable", err)
		}
		if validPostSource(root, link) || postSourceErrorCode(root, link) != "CC-421-PATH" {
			t.Fatal("symlink escape accepted")
		}
	})
}

func TestPostMediaDiagnosticLegacyHistoryAndUnknownDataStayPrivate(t *testing.T) {
	const secret = "DoNotPublishThisFilenameOrToken"
	var legacy savedPostMedia
	if err := json.Unmarshal([]byte(`{"original":"private-original.mp4","file":"private-output.mp4","error":"저장 파일 경로·형식 확인 필요"}`), &legacy); err != nil {
		t.Fatal(err)
	}
	st := ClientState{Version: appVersion, Jobs: []*Job{{
		Status: "error", ErrorCode: "CC-421", URL: "https://example.invalid/?cookie=" + secret,
		Detail: secret, ErrorTitle: secret, ErrorAdvice: secret,
		Media: []savedPostMedia{
			legacy,
			{Original: "C:\\Users\\" + secret, File: secret + ".mp4", ErrorCode: "CC-421-MISSING", Error: secret},
			{Original: secret, ErrorCode: "CC-421-MISSING\n" + secret, Error: "https://example.invalid/" + secret},
		},
	}}}
	report := buildDiagnosticReport(st)
	for _, want := range []string{"최근 오류: CC-421", "파일 오류 1: CC-421-SOURCE", "파일 오류 2: CC-421-MISSING", "파일 오류 3: CC-421-UNKNOWN"} {
		if !strings.Contains(report, want) {
			t.Fatalf("diagnostic omitted a safe cause %q: %s", want, report)
		}
	}
	for _, forbidden := range []string{secret, "example.invalid", "private-original", "private-output", "Users", "cookie="} {
		if strings.Contains(report, forbidden) {
			t.Fatalf("diagnostic exposed %q: %s", forbidden, report)
		}
	}
}

func TestPostMediaDiagnosticLimitsPerFileDetails(t *testing.T) {
	media := make([]savedPostMedia, 10)
	for i := range media {
		media[i] = savedPostMedia{ErrorCode: "CC-421-MISSING", Error: postMediaErrorMessages["CC-421-MISSING"]}
	}
	report := buildDiagnosticReport(ClientState{Version: appVersion, Jobs: []*Job{{Status: "error", ErrorCode: "CC-421", Media: media}}})
	if strings.Count(report, ": CC-421-MISSING") != 8 || !strings.Contains(report, "추가 파일 오류: 2개") {
		t.Fatal("unbounded or incomplete file diagnostics", report)
	}
}
