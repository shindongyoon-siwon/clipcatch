package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestClipFeedbackSeparatesCandidateLimits(t *testing.T) {
	good := clipFormat{ID: "video", URL: "https://video.twimg.com/a.mp4", Ext: "mp4", Protocol: "https", VCodec: "h264", ACodec: "aac"}
	webm := good
	webm.Ext = "webm"
	dash := good
	dash.Protocol = "http_dash_segments"
	for _, tc := range []struct {
		name   string
		c      clipChoice
		reason string
	}{
		{"video WebM", clipChoice{Selector: "v", Status: "unsupported", Video: clipInput{Format: webm}}, "video_webm"},
		{"audio WebM", clipChoice{Selector: "v+a", Status: "unsupported", Video: clipInput{Format: good}, Audio: &clipInput{Format: webm}}, "audio_webm"},
		{"stream", clipChoice{Selector: "v", Status: "unsupported", Video: clipInput{Format: dash}}, "video_protocol"},
		{"range", clipChoice{Selector: "v", Status: "unsupported", Video: clipInput{Format: good}}, "partial_response"},
		{"network unknown", clipChoice{Selector: "v", Status: "unknown", Video: clipInput{Format: good}}, "unconfirmed"},
		{"supported", clipChoice{Selector: "v", Status: "supported", Notice: "ready", Video: clipInput{Format: good}}, "ready"},
	} {
		reason, notice := clipChoiceFeedback(tc.c)
		if reason != tc.reason || notice == "" {
			t.Fatalf("%s: reason=%s notice=%q", tc.name, reason, notice)
		}
		if strings.Contains(notice, "https://") {
			t.Fatal("source URL leaked to user notice")
		}
	}
}

// Explicitly opt-in, metadata and one-byte/manifest preflight only. Never
// download a full video or call a cloud-runner result a user-PC reproduction.
func TestSectionLiveYouTubePreflight(t *testing.T) {
	tool := os.Getenv("CLIPCATCH_DIAGNOSTIC_YTDLP")
	if tool == "" {
		t.Skip("read-only public YouTube preflight requires explicit engine path")
	}
	a := newApp(t.TempDir(), t.TempDir())
	a.ytDlp = tool
	r := httptest.NewRequest("POST", "http://127.0.0.1:18767/api/clip-preview", strings.NewReader(`{"url":"https://www.youtube.com/watch?v=oR--AmI7iHc","quality":0}`))
	w := httptest.NewRecorder()
	a.clipPreview(w, r)
	var out clipPreviewResponse
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil {
		t.Fatalf("invalid preflight response status %d", w.Code)
	}
	evidence := map[string]any{"scope": "Windows cloud runner, metadata/preflight only", "video_id": "oR--AmI7iHc", "partial": out.Partial, "reason": out.Reason, "quality": out.Quality, "duration": out.Duration, "notice": out.Notice, "queued_jobs": len(a.state.Jobs)}
	if len(a.state.Jobs) != 0 {
		t.Fatal("preflight queued a download")
	}
	a.clips.mu.Lock()
	if p := a.clips.plans[out.Token]; p != nil {
		c := p.Choices[out.Quality]
		evidence["video_format"] = map[string]string{"ext": c.Video.Format.Ext, "protocol": c.Video.Format.Protocol}
		if c.Audio != nil {
			evidence["audio_format"] = map[string]string{"ext": c.Audio.Format.Ext, "protocol": c.Audio.Format.Protocol}
		}
	}
	a.clips.mu.Unlock()
	b, _ := json.Marshal(evidence)
	t.Log("YOUTUBE_PREFLIGHT " + string(b))
}
