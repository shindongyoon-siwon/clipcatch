package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCheckUpdateClickDuringStartupReportsProgress(t *testing.T) {
	a := newApp(t.TempDir(), t.TempDir())
	a.state.AppUpdate.Status = "checking"
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8765/api/check-app-update", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://127.0.0.1:8765")
	w := httptest.NewRecorder()
	a.serveHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("startup check should respond with progress (200), got %d: %s", w.Code, w.Body.String())
	}
	var response map[string]bool
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response["already_running"] {
		t.Fatalf("expected already_running, got %s", w.Body.String())
	}
	if a.state.AppUpdate.Status != "checking" {
		t.Fatal("status unexpectedly changed")
	}
}

func TestCheckUpdateCannotInterruptInstall(t *testing.T) {
	a := newApp(t.TempDir(), t.TempDir())
	a.state.AppUpdate.Status = "downloading"
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8765/api/check-app-update", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://127.0.0.1:8765")
	w := httptest.NewRecorder()
	a.serveHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected conflict, got %d", w.Code)
	}
}

func TestButtonFeedbackAlwaysVisibleInFrontend(t *testing.T) {
	data, err := pages.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{
		"id=\"updateFeedback\"", "id=\"installAppUpdateFooter\"", "id=\"updateProgress\"",
		"프로그램 새 버전을 확인하는 중", "영상 엔진 업데이트를 시작하는 중",
		"최신 버전", "업데이트 실패", "새 버전 설치",
	} {
		if !strings.Contains(string(data), fragment) {
			t.Errorf("missing UI feedback: %s", fragment)
		}
	}
}
