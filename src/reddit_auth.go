package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const redditEncryptedFile = "reddit-auth.dpapi"

// keepOnlyRedditCookies discards every non-Reddit credential before storage or
// invocation. A browser's full profile/password database is never accessed.
func keepOnlyRedditCookies(data []byte) ([]byte, error) {
	if len(data) == 0 || len(data) >= 2<<20 {
		return nil, errors.New("쿠키 파일은 최대 2MB까지 지원해요")
	}
	first := strings.TrimPrefix(string(data), "\ufeff")
	if !strings.HasPrefix(first, "# Netscape HTTP Cookie File") && !strings.HasPrefix(first, "# HTTP Cookie File") {
		return nil, errors.New("cookies.txt 형식만 지원해요")
	}
	var out bytes.Buffer
	out.WriteString("# Netscape HTTP Cookie File\n")
	found := 0
	scanner := bufio.NewScanner(strings.NewReader(first))
	scanner.Buffer(make([]byte, 4096), 2<<20)
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if len(line) > 8192 {
			continue
		}
		candidate := line
		httpOnly := false
		if strings.HasPrefix(candidate, "#HttpOnly_") {
			candidate = strings.TrimPrefix(candidate, "#HttpOnly_")
			httpOnly = true
		} else if strings.HasPrefix(candidate, "#") || strings.TrimSpace(candidate) == "" {
			continue
		}
		fields := strings.Split(candidate, "\t")
		if len(fields) != 7 {
			continue
		}
		// Exact domain boundary: not reddit.com.evil, myreddit.com, or local hosts.
		domain := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(fields[0])), ".")
		if domain != "reddit.com" {
			continue
		}
		if fields[1] != "TRUE" && fields[1] != "FALSE" {
			continue
		}
		if fields[3] != "TRUE" && fields[3] != "FALSE" {
			continue
		}
		if fields[5] == "" || fields[6] == "" {
			continue
		}
		if httpOnly {
			out.WriteString("#HttpOnly_")
		}
		out.WriteString(candidate)
		out.WriteByte('\n')
		found++
	}
	if err := scanner.Err(); err != nil {
		return nil, errors.New("쿠키 파일을 읽을 수 없어")
	}
	if found == 0 {
		return nil, errors.New("파일에 reddit.com 로그인 정보가 없어. 레딧 쿠키만 내보내 줘")
	}
	return out.Bytes(), nil
}

func (a *App) cookieStorePath() string { return filepath.Join(a.dataDir, redditEncryptedFile) }
func (a *App) loadRememberedRedditAuth() {
	stored, err := os.ReadFile(a.cookieStorePath())
	if err != nil || len(stored) > 2<<20 {
		return
	}
	decrypted, err := unprotectRedditData(stored)
	if err != nil {
		return
	}
	onlyReddit, err := keepOnlyRedditCookies(decrypted)
	for i := range decrypted {
		decrypted[i] = 0
	}
	if err != nil {
		return
	}
	a.redditCookies = onlyReddit
	a.state.RedditAuthReady = true
}
func (a *App) removeOrphanedCookieFiles() {
	list, _ := filepath.Glob(filepath.Join(a.dataDir, "reddit-auth-session-*.txt"))
	for _, f := range list {
		_ = os.Remove(f)
	}
	// Clean historical v1.3.0 temporary files from interrupted downloads.
	list, _ = filepath.Glob(filepath.Join(a.dataDir, "cookies-session-*.txt"))
	for _, f := range list {
		_ = os.Remove(f)
	}
}
func (a *App) copyRedditCookies() []byte {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]byte(nil), a.redditCookies...)
}
func (a *App) uploadRedditCookiesHandler(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, (2<<20)+8192)
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data;") {
		writeError(w, 415, "쿠키 파일을 선택해 줘")
		return
	}
	mr, err := r.MultipartReader()
	if err != nil {
		writeError(w, 400, "쿠키 파일 형식이 맞지 않아")
		return
	}
	part, err := mr.NextPart()
	if err != nil || part.FormName() != "cookies" || !strings.HasSuffix(strings.ToLower(part.FileName()), ".txt") {
		writeError(w, 400, "cookies.txt 파일을 선택해 줘")
		return
	}
	raw, err := io.ReadAll(io.LimitReader(part, 2<<20))
	if err != nil {
		writeError(w, 400, "쿠키 파일을 읽지 못했어")
		return
	}
	filtered, err := keepOnlyRedditCookies(raw)
	for i := range raw {
		raw[i] = 0
	}
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	remember := false
	if next, err := mr.NextPart(); err == nil && next.FormName() == "remember" {
		val, _ := io.ReadAll(io.LimitReader(next, 10))
		remember = string(val) == "yes"
	}
	if remember {
		encrypted, err := protectRedditData(filtered)
		if err != nil {
			writeError(w, 500, "이 PC에서 안전하게 저장할 수 없어. 기억하기 체크를 해제하고 다시 시도해 줘")
			return
		}
		if err := os.MkdirAll(a.dataDir, 0700); err != nil {
			writeError(w, 500, "설정 폴더에 저장하지 못했어")
			return
		}
		path := a.cookieStorePath()
		temp := path + ".new"
		if err := os.WriteFile(temp, encrypted, 0600); err != nil {
			writeError(w, 500, "레딧 로그인 보관에 실패했어")
			return
		}
		if err := os.Rename(temp, path); err != nil {
			_ = os.Remove(temp)
			writeError(w, 500, "레딧 로그인 보관에 실패했어")
			return
		}
	} else {
		_ = os.Remove(a.cookieStorePath())
	}
	a.mu.Lock()
	for i := range a.redditCookies {
		a.redditCookies[i] = 0
	}
	a.redditCookies = filtered
	a.state.RedditAuthReady = true
	a.mu.Unlock()
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (a *App) clearRedditCookiesHandler(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	for i := range a.redditCookies {
		a.redditCookies[i] = 0
	}
	a.redditCookies = nil
	a.state.RedditAuthReady = false
	a.mu.Unlock()
	_ = os.Remove(a.cookieStorePath())
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// runDownloadWithRedditCookies creates an owner-only temporary cookie file,
// passes it only to the Reddit yt-dlp request and immediately deletes it.
func (a *App) runDownloadWithRedditCookies(id string, j *Job, data []byte) (error, string) {
	if !isRedditHost(j.Host) {
		return errors.New("레딧 링크만 로그인 기능을 사용할 수 있어"), ""
	}
	if err := os.MkdirAll(a.dataDir, 0700); err != nil {
		return err, ""
	}
	file, err := os.CreateTemp(a.dataDir, "reddit-auth-session-*.txt")
	if err != nil {
		return err, ""
	}
	path := file.Name()
	defer os.Remove(path)
	_ = file.Chmod(0600)
	_, err = file.Write(data)
	closeErr := file.Close()
	if err != nil {
		return err, ""
	}
	if closeErr != nil {
		return closeErr, ""
	}
	return a.runDownloadAttempt(id, j, false, path)
}

// Isolated cookie arguments: the URL always stays the last argument and the
// --cookies path is not user-controlled (created with os.CreateTemp).
func addCookieArg(args []string, path string) ([]string, error) {
	if len(args) == 0 || !strings.HasSuffix(path, ".txt") {
		return nil, fmt.Errorf("잘못된 레딧 인증 정보")
	}
	out := append([]string(nil), args[:len(args)-1]...)
	out = append(out, "--cookies", path, args[len(args)-1])
	return out, nil
}
