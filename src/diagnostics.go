package main

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Diagnostic output deliberately contains only predefined labels and status
// codes. It never includes cookies, original media URLs, filesystem paths or
// raw downloader logs which might contain sensitive tokens.
type DiagnosticCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
}
type DiagnosticState struct {
	Status    string            `json:"status"`
	Summary   string            `json:"summary"`
	CheckedAt string            `json:"checked_at,omitempty"`
	Checks    []DiagnosticCheck `json:"checks,omitempty"`
}

func classifyDownloadError(raw, host string) (string, string, string) {
	s := strings.ToLower(raw)
	switch {
	case strings.Contains(s, "429") || strings.Contains(s, "too many requests"):
		return "CC-102", "사이트에서 잠시 다운로드를 제한했어", "몇 분 뒤 다시 시도해 줘. 짧은 시간에 반복해서 요청하지 마."
	case needsAuth(raw):
		if isRedditHost(host) {
			return "CC-103", "레딧 로그인 확인이 필요해", "레딧 전용 인증을 연결한 뒤 해당 영상만 다시 시도해 줘."
		}
		return "CC-103", "로그인이 필요한 영상이야", "원본 사이트에서 공개 영상인지 먼저 확인해 줘."
	case isRedditHost(host) && strings.Contains(s, "failed to parse json"):
		return "CC-106", "레딧 영상 정보를 해석하지 못했어", "사이트 응답이나 다운로드 방식이 변경되었을 수 있어. 엔진 업데이트를 확인해 줘. 로그인 필요 여부는 이 오류만으로 알 수 없어."
	case isRedditConnectionDrop(raw) || strings.Contains(s, "connection refused") || strings.Contains(s, "network is unreachable") || strings.Contains(s, "timed out") || strings.Contains(s, "name resolution"):
		return "CC-101", "영상 서버 연결이 끊겼어", "원본 게시물이 브라우저에서 재생되는지 확인하고 다시 시도해 줘. 계속되면 네트워크 상태를 진단해 줘."
	case strings.Contains(s, "403") || strings.Contains(s, "forbidden"):
		return "CC-104", "사이트에서 영상 접근을 거부했어", "원본 게시물의 공개 상태를 확인해 줘. 접근 제한을 자동으로 우회할 수는 없어."
	case strings.Contains(s, "not available") || strings.Contains(s, "unavailable") || strings.Contains(s, "private video") || strings.Contains(s, "deleted") || strings.Contains(s, "404"):
		return "CC-105", "영상이 삭제됐거나 공개되지 않았어", "원본 페이지에서 영상이 실제로 재생되는지 확인해 줘."
	case strings.Contains(s, "javascript runtime") || strings.Contains(s, "ejs") || strings.Contains(s, "js challenge") || strings.Contains(s, "deno"):
		return "CC-109", "사이트 재생 방식에 필요한 구성요소가 없어", "현재 영상 엔진이 해당 사이트를 지원하는지 확인해 줘. 유튜브는 별도의 실행 환경이 필요할 수 있어."
	case strings.Contains(s, "ffmpeg") || strings.Contains(s, "ffprobe") || strings.Contains(s, "merger"):
		return "CC-107", "영상과 오디오 처리에 문제가 있어", "문제 해결에서 진단을 실행해 줘. 영상 처리 도구를 확인할 수 있어."
	case strings.Contains(s, "no space left") || strings.Contains(s, "permission denied") || strings.Contains(s, "access is denied") || strings.Contains(s, "disk full"):
		return "CC-108", "저장 공간 또는 폴더 권한 문제야", "저장 공간을 확인하고 다운로드 폴더에 파일을 저장할 수 있는지 점검해 줘."
	case strings.Contains(s, "unsupported url") || strings.Contains(s, "no suitable extractor") || strings.Contains(s, "unable to extract") || strings.Contains(s, "extractor error"):
		return "CC-106", "사이트의 영상 다운로드 방식이 바뀌었을 수 있어", "먼저 안정판 엔진 업데이트를 시도해 줘. 해결되지 않으면 시험판 복구를 선택할 수 있어."
	case strings.Contains(s, "executable file not found") || strings.Contains(s, "yt-dlp") && strings.Contains(s, "not found"):
		return "CC-201", "영상 다운로드 도구를 찾을 수 없어", "문제 해결에서 진단을 실행하고 엔진 설치 상태를 확인해 줘."
	default:
		return "CC-199", "다운로드를 마치지 못했어", "원본 링크가 재생되는지 확인하고 다시 시도해 줘. 계속 실패하면 진단 결과를 복사해 공유해 줘."
	}
}

func (a *App) runDiagnosticsHandler(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	if a.state.Diagnostics.Status == "checking" {
		a.mu.Unlock()
		writeJSON(w, 200, map[string]bool{"ok": true, "already_running": true})
		return
	}
	a.state.Diagnostics = DiagnosticState{Status: "checking", Summary: "다운로드 환경을 점검하는 중..."}
	a.mu.Unlock()
	go a.performDiagnostics()
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (a *App) performDiagnostics() {
	a.mu.Lock()
	tool := a.ytDlp
	ffmpeg := a.ffmpeg
	saveDir := a.saveDir
	phase := a.state.Phase
	a.mu.Unlock()
	checks := make([]DiagnosticCheck, 0, 5)
	add := func(name, status, msg string) {
		checks = append(checks, DiagnosticCheck{Name: name, Status: status, Message: msg})
	}
	if !nonempty(tool) {
		add("다운로드 엔진", "error", "다운로드 도구를 찾지 못했어. 프로그램을 재실행해 줘.")
	} else if !probeToolExecutable(tool, "--version") {
		add("다운로드 엔진", "error", "다운로드 도구를 실행할 수 없어. Windows 보안 차단을 확인해 줘.")
	} else {
		add("다운로드 엔진", "ok", "실행 확인됨")
	}
	if !nonempty(ffmpeg) {
		add("영상·오디오 처리", "error", "영상 처리 도구가 없어. 프로그램을 재실행해 줘.")
	} else if !probeToolExecutable(ffmpeg, "-version") {
		add("영상·오디오 처리", "error", "영상 처리 도구를 실행할 수 없어. Windows 보안 차단을 확인해 줘.")
	} else {
		add("영상·오디오 처리", "ok", "실행 확인됨")
	}
	if phase != "ready" {
		add("프로그램 상태", "warn", "다운로드 준비 또는 업데이트가 진행 중이야.")
	} else {
		add("프로그램 상태", "ok", "다운로드 준비 완료")
	}
	if err := os.MkdirAll(saveDir, 0755); err != nil {
		add("저장 폴더", "error", "폴더를 만들 수 없어. 접근 권한을 확인해 줘.")
	} else if f, err := os.CreateTemp(saveDir, ".clipcatch-check-"); err != nil {
		add("저장 폴더", "error", "폴더에 파일을 저장할 수 없어. 권한을 확인해 줘.")
	} else {
		name := f.Name()
		_ = f.Close()
		_ = os.Remove(name)
		add("저장 폴더", "ok", "파일 저장 가능")
	}
	// Networking checks are intentionally read-only; only official public URLs
	// and no user auth credentials are used.
	for _, item := range []struct{ name, url string }{{"업데이트 서버", "https://api.github.com"}, {"레딧 연결", "https://www.reddit.com"}} {
		status, notice := probePublicServer(item.url)
		add(item.name, status, notice)
	}
	failures := 0
	warnings := 0
	for _, c := range checks {
		if c.Status == "error" {
			failures++
		} else if c.Status == "warn" {
			warnings++
		}
	}
	summary := "주요 항목 모두 정상 · 사이트별 차단은 별개일 수 있어"
	result := "ok"
	if failures > 0 {
		summary = "확인할 문제 " + intString(failures) + "개 · 아래 조치 방법을 확인해 줘"
		result = "error"
	} else if warnings > 0 {
		summary = "확인이 필요한 항목 " + intString(warnings) + "개 있어"
		result = "warning"
	}
	a.mu.Lock()
	a.state.Diagnostics = DiagnosticState{Status: result, Summary: summary, CheckedAt: time.Now().Format(time.RFC3339), Checks: checks}
	a.mu.Unlock()
}

func intString(n int) string {
	if n == 0 {
		return "0"
	}
	// Small fixed diagnostic counts: prevent adding arbitrary raw values to reports.
	out := ""
	for n > 0 {
		out = string(byte('0'+n%10)) + out
		n /= 10
	}
	return out
}

func probeToolExecutable(target string, args ...string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, target, args...)
	cmd.Stdin = nil
	// Diagnostics require only the exit code; no subprocess output, paths or
	// credentials are returned or included in a shareable report.
	return cmd.Run() == nil
}

func probePublicServer(target string) (string, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, target, nil)
	if err != nil {
		return "warn", "접속 확인을 진행하지 못했어."
	}
	req.Header.Set("User-Agent", "ClipCatch/diagnostics")
	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "warn", "서버 연결을 확인하지 못했어. 네트워크를 점검해 줘."
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		return "warn", "사이트가 요청을 잠시 제한하고 있어."
	}
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		return "warn", "서버에는 연결됐지만 접근이 제한됐어."
	}
	if resp.StatusCode >= 500 {
		return "warn", "사이트 서버 오류가 발생했어."
	}
	return "ok", "서버 연결 확인됨"
}

// Whitelisted helpers for clipboard reports. Never include dataDir, saveDir,
// the raw job message or the original link in the generated report.
func buildDiagnosticReport(st ClientState) string {
	lines := []string{"ClipCatch 문제 진단", "프로그램 버전: " + st.Version,
		"다운로드 엔진: " + normalizeChannel(st.Engine.Channel),
		"엔진 상태: " + safeDiagnosticStatus(st.Engine.Status),
		"프로그램 상태: " + safeDiagnosticStatus(st.Phase),
		"진단 상태: " + safeDiagnosticStatus(st.Diagnostics.Status),
	}
	for _, c := range st.Diagnostics.Checks {
		lines = append(lines, safeCheckName(c.Name)+": "+safeDiagnosticStatus(c.Status))
	}
	codeCount := 0
	for _, j := range st.Jobs {
		if j.Status == "error" && j.ErrorCode != "" {
			if codeCount >= 8 {
				break
			}
			lines = append(lines, "최근 오류: "+safeErrorCode(j.ErrorCode))
			codeCount++
		}
	}
	return strings.Join(lines, "\n")
}
func safeErrorCode(code string) string {
	switch code {
	case "CC-101", "CC-102", "CC-103", "CC-104", "CC-105", "CC-106", "CC-107", "CC-108", "CC-109", "CC-199", "CC-201", "CC-312", "CC-313", "CC-314", "CC-315":
		return code
	}
	return "CC-199"
}
func safeCheckName(name string) string {
	switch name {
	case "다운로드 엔진", "영상·오디오 처리", "프로그램 상태", "저장 폴더", "업데이트 서버", "레딧 연결":
		return name
	}
	return "기타 항목"
}
func safeDiagnosticStatus(status string) string {
	switch status {
	case "ok", "warn", "error", "warning", "checking", "idle", "ready", "preparing", "offline", "missing", "working", "current", "available":
		return status
	}
	return "unknown"
}
