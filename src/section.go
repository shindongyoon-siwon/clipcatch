package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Plans and signed media URLs are memory-only, owner-bound and short-lived.
// Persisted jobs contain only the user's exact times and safe format identifiers.
type clipSpec struct {
	Start   float64 `json:"start"`
	End     float64 `json:"end"`
	Quality int     `json:"quality"`
	Token   string  `json:"-"`
}
type clipFormat struct {
	ID       string            `json:"format_id"`
	URL      string            `json:"url"`
	Ext      string            `json:"ext"`
	Protocol string            `json:"protocol"`
	Width    int               `json:"width"`
	Height   int               `json:"height"`
	FPS      float64           `json:"fps"`
	Bitrate  float64           `json:"tbr"`
	VCodec   string            `json:"vcodec"`
	ACodec   string            `json:"acodec"`
	HDR      string            `json:"dynamic_range"`
	DRM      bool              `json:"has_drm"`
	Headers  map[string]string `json:"http_headers"`
}
type clipMetadata struct {
	ID       string            `json:"id"`
	Title    string            `json:"title"`
	Duration float64           `json:"duration"`
	Live     bool              `json:"is_live"`
	Formats  []clipFormat      `json:"formats"`
	Entries  []json.RawMessage `json:"entries"`
	Headers  map[string]string `json:"http_headers"`
	URL      string            `json:"url"`
	Ext      string            `json:"ext"`
	Width    int               `json:"width"`
	Height   int               `json:"height"`
}
type clipSegment struct {
	URL             string
	Start, Duration float64
}
type clipPlaylist struct {
	Init     string
	Segments []clipSegment
	Duration float64
}
type clipInput struct {
	Format clipFormat
	Size   int64
	List   *clipPlaylist
}
type clipChoice struct {
	Quality        int
	Video          clipInput
	Audio          *clipInput
	Selector       string
	Status, Notice string
}
type clipPlan struct {
	Token, URL, Owner string
	SourceURL         string
	Browser           *browserMedia
	At                time.Time
	Meta              clipMetadata
	Choices           map[int]clipChoice
	Silent            bool
}
type clipService struct {
	mu     sync.Mutex
	plans  map[string]*clipPlan
	slots  chan struct{}
	client *http.Client
}
type clipPreviewRequest struct {
	URL         string `json:"url"`
	Quality     int    `json:"quality"`
	Token       string `json:"token,omitempty"`
	MediaURL    string `json:"media_url,omitempty"`
	AudioURL    string `json:"audio_url,omitempty"`
	AllowSilent bool   `json:"allow_silent,omitempty"`
}
type clipPreviewResponse struct {
	Token     string  `json:"token,omitempty"`
	Title     string  `json:"title,omitempty"`
	Duration  float64 `json:"duration"`
	Qualities []int   `json:"qualities"`
	Quality   int     `json:"quality"`
	Partial   string  `json:"partial"`
	Reason    string  `json:"reason,omitempty"`
	Notice    string  `json:"notice"`
	Expires   int64   `json:"expires,omitempty"`
}

var clipIDRE = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,100}$`)

func newClipService() *clipService {
	return &clipService{plans: map[string]*clipPlan{}, slots: make(chan struct{}, 2), client: clipHTTPClient()}
}
func clipNotice(code string) string {
	switch code {
	case "ready":
		return "\ubd80\ubd84 \uc694\uccad \ud655\uc778\ub428. \uc800\uc7a5 \uc911\uc5d0\ub3c4 \uc804\uccb4 \uc751\ub2f5\uc744 \ucc28\ub2e8\ud574\uc694."
	case "checking":
		return "\uc601\uc0c1 \uc815\ubcf4\ub97c \ud655\uc778\ud558\uace0 \uc788\uc5b4\uc694."
	case "reddit":
		return "\ub808\ub527\uc740 ClipCatch Browser SECTION TEST\uc5d0\uc11c \uc7ac\uc0dd \uc601\uc0c1\uc744 \ucc3e\uc544 \uad6c\uac04\uc744 \uc124\uc815\ud574 \uc8fc\uc138\uc694."
	case "multi":
		return "\uc5ec\ub7ec \uc601\uc0c1\uc774 \ub4e0 \uac8c\uc2dc\ubb3c\uc774\uc5d0\uc694. \uad6c\uac04 \uc800\uc7a5\uc740 \uac1c\ubcc4 \uc601\uc0c1 \ub9c1\ud06c\ub85c \ud655\uc778\ud574 \uc8fc\uc138\uc694."
	case "range":
		return "\uc774 \ud654\uc9c8\uc758 \ubd80\ubd84 \ub2e4\uc6b4\ub85c\ub4dc\ub97c \uc9c0\uc6d0\ud558\uc9c0 \uc54a\uc544\uc694. \uc804\uccb4\ub85c \uc790\ub3d9 \uc804\ud658\ud558\uc9c0 \uc54a\uc544\uc694."
	case "unknown":
		return "\uc9c0\uae08\uc740 \uad6c\uac04 \uc800\uc7a5 \uac00\ub2a5 \uc5ec\ubd80\ub97c \ud655\uc778\ud558\uc9c0 \ubabb\ud588\uc5b4\uc694. \ub9c1\ud06c\ub294 \uc720\uc9c0\ub418\uba70 \ub2e4\uc2dc \ud655\uc778\ud560 \uc218 \uc788\uc5b4\uc694."
	case "expired":
		return "\uc601\uc0c1 \uc815\ubcf4\uac00 \ub9cc\ub8cc\ub410\uc5b4\uc694. \ub9c1\ud06c\ub97c \ub2e4\uc2dc \ud655\uc778\ud574 \uc8fc\uc138\uc694. \uc804\uccb4 \uc601\uc0c1\uc740 \ubc1b\uc9c0 \uc54a\uc558\uc5b4\uc694."
	case "times":
		return "\uc2dc\uc791\u00b7\uc885\ub8cc \uc2dc\uac04\uc744 \ud655\uc778\ud574 \uc8fc\uc138\uc694. \uc2dc\uc791\ubcf4\ub2e4 \uc885\ub8cc\uac00 \ub4a4\uc5ec\uc57c \ud558\uba70 \uc601\uc0c1 \uae38\uc774 \uc548\uc774\uc5b4\uc57c \ud574\uc694."
	case "budget":
		return "\uc804\uccb4 \ub2e4\uc6b4\ub85c\ub4dc\ub85c \uc774\uc5b4\uc9c8 \uc218 \uc788\uc5b4 \uad6c\uac04 \uc800\uc7a5\uc744 \uc911\ub2e8\ud588\uc5b4\uc694. \uc804\uccb4\ub85c \uc7ac\uc2dc\ub3c4\ud558\uc9c0 \uc54a\uc558\uc5b4\uc694."
	case "verify":
		return "\uc800\uc7a5 \uad6c\uac04\uc758 \uc601\uc0c1\u00b7\uc74c\uc131\u00b7\uae38\uc774 \uac80\uc0ac\uc5d0 \ud1b5\uacfc\ud558\uc9c0 \ubabb\ud588\uc5b4\uc694. \uc644\ub8cc\ub85c \ucc98\ub9ac\ud558\uc9c0 \uc54a\uc558\uc5b4\uc694."
	case "processing":
		return "\uc785\ub825\ud55c \uad6c\uac04\ub9cc \ubc1b\uc544 MP4\ub85c \uc800\uc7a5\ud558\uace0 \uc788\uc5b4\uc694."
	case "done":
		return "\uc9c0\uc815 \uad6c\uac04 MP4 \uc800\uc7a5 \uc644\ub8cc"
	}
	return "\uad6c\uac04 \uc800\uc7a5\uc744 \uc644\ub8cc\ud558\uc9c0 \ubabb\ud588\uc5b4\uc694. \uc804\uccb4 \uc601\uc0c1\uc73c\ub85c \uc804\ud658\ud558\uc9c0 \uc54a\uc558\uc5b4\uc694."
}
func clipOwner(a *App, r *http.Request) string {
	if strings.HasPrefix(r.URL.Path, "/bridge/") {
		a.mu.Lock()
		defer a.mu.Unlock()
		return a.browser.Link.Digest
	}
	return "web"
}
func clipResolution(f clipFormat) int {
	if f.Width > 0 && f.Height > 0 {
		return min(f.Width, f.Height)
	}
	return f.Height
}
func clipEligible(f clipFormat) bool {
	return f.URL != "" && !f.DRM && f.VCodec != "none" && clipResolution(f) > 0 && clipIDRE.MatchString(f.ID)
}
func clipQualities(m clipMetadata) []int {
	seen := map[int]bool{}
	for _, f := range m.Formats {
		if clipEligible(f) {
			seen[clipResolution(f)] = true
		}
	}
	out := []int{}
	for n := range seen {
		out = append(out, n)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(out)))
	return out
}
func clipSelect(m clipMetadata, q int) (clipFormat, *clipFormat, error) {
	fs := []clipFormat{}
	for _, f := range m.Formats {
		if clipEligible(f) && (q == 0 || clipResolution(f) == q) {
			fs = append(fs, f)
		}
	}
	sort.SliceStable(fs, func(i, j int) bool {
		a, b := fs[i], fs[j]
		if clipResolution(a) != clipResolution(b) {
			return clipResolution(a) > clipResolution(b)
		}
		if a.FPS != b.FPS {
			return a.FPS > b.FPS
		}
		return a.Bitrate > b.Bitrate
	})
	if len(fs) == 0 {
		return clipFormat{}, nil, errClipRange
	}
	v := fs[0]
	if v.ACodec != "none" && v.ACodec != "" {
		return v, nil, nil
	}
	aud := []clipFormat{}
	for _, f := range m.Formats {
		if f.VCodec == "none" && f.ACodec != "none" && f.ACodec != "" && !f.DRM && clipIDRE.MatchString(f.ID) {
			aud = append(aud, f)
		}
	}
	sort.SliceStable(aud, func(i, j int) bool { return aud[i].Bitrate > aud[j].Bitrate })
	if len(aud) > 0 {
		return v, &aud[0], nil
	}
	return v, nil, nil
}
func (a *App) clipExtract(ctx context.Context, raw string) (clipMetadata, error) {
	var m clipMetadata
	a.mu.Lock()
	tool := a.ytDlp
	a.mu.Unlock()
	if tool == "" {
		return m, errors.New("clip-tools")
	}
	out := &cappedBuffer{Limit: 4 << 20}
	errout := &cappedBuffer{Limit: 8192}
	cmd := hiddenCommandContext(ctx, tool, "--ignore-config", "--skip-download", "--dump-single-json", "--no-playlist", "--playlist-end", "2", "--no-warnings", "--socket-timeout", "5", "--retries", "0", "--extractor-retries", "0", raw)
	cmd.Stdout = out
	cmd.Stderr = errout
	if e := cmd.Run(); e != nil {
		return m, e
	}
	if len(out.Data) >= out.Limit || json.Unmarshal(out.Data, &m) != nil {
		return m, errors.New("clip-metadata")
	}
	if len(m.Entries) == 1 {
		var one clipMetadata
		if json.Unmarshal(m.Entries[0], &one) == nil {
			m = one
		}
	}
	if len(m.Formats) == 0 && m.URL != "" {
		m.Formats = []clipFormat{{ID: "direct", URL: m.URL, Ext: m.Ext, Width: m.Width, Height: m.Height, VCodec: "unknown", ACodec: "unknown", Protocol: "https"}}
	}
	for i := range m.Formats {
		if len(m.Formats[i].Headers) == 0 {
			m.Formats[i].Headers = m.Headers
		}
	}
	return m, nil
}
func (a *App) clipPreview(w http.ResponseWriter, r *http.Request) {
	var in clipPreviewRequest
	if decodeBrowserJSON(w, r, &in) != nil || in.Quality < 0 || in.Quality > 8192 {
		writeError(w, 400, clipNotice("unknown"))
		return
	}
	clean, host, e := cleanURL(in.URL)
	if e != nil {
		writeError(w, 400, clipNotice("unknown"))
		return
	}
	reply := clipPreviewResponse{Partial: "unknown", Reason: "unconfirmed", Notice: clipNotice("unknown"), Qualities: []int{}}
	raw := clean
	bridge := strings.HasPrefix(r.URL.Path, "/bridge/")
	if in.MediaURL != "" {
		if !bridge {
			writeError(w, 400, "browser source only")
			return
		}
		media, e := validateBrowserMedia(browserMedia{PageURL: clean, MediaURL: in.MediaURL, AudioURL: in.AudioURL, AllowSilent: in.AllowSilent})
		if e != nil {
			writeError(w, 400, clipNotice("unknown"))
			return
		}
		raw = media.MediaURL
	} else if isRedditHost(host) {
		reply.Partial = "unsupported"
		reply.Reason = "reddit_browser"
		reply.Notice = clipNotice("reddit")
		writeJSON(w, 200, reply)
		return
	}
	owner := clipOwner(a, r)
	s := a.clips
	s.mu.Lock()
	p := s.plans[in.Token]
	if p != nil && (p.Owner != owner || p.URL != clean || p.SourceURL != raw || time.Since(p.At) > 3*time.Minute) {
		p = nil
	}
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	case <-ctx.Done():
		writeJSON(w, 200, reply)
		return
	}
	if p == nil {
		var m clipMetadata
		var e error
		if in.MediaURL != "" && strings.HasSuffix(strings.Split(raw, "?")[0], ".mp4") {
			m, e = a.clipExtractBrowserFile(ctx, raw)
		} else {
			m, e = a.clipExtract(ctx, raw)
		}
		if e != nil {
			writeJSON(w, 200, reply)
			return
		}
		if len(m.Entries) > 0 {
			reply.Partial = "unsupported"
			reply.Reason = "multiple_videos"
			reply.Notice = clipNotice("multi")
			writeJSON(w, 200, reply)
			return
		}
		if m.Live || m.Duration <= 0 || math.IsNaN(m.Duration) || math.IsInf(m.Duration, 0) {
			reply.Partial = "unsupported"
			reply.Reason, reply.Notice = clipMetadataFeedback(m.Live)
			writeJSON(w, 200, reply)
			return
		}
		if in.AudioURL != "" {
			m.Formats = append(m.Formats, clipFormat{ID: "browseraudio", URL: in.AudioURL, Ext: "m4a", Protocol: "https", VCodec: "none", ACodec: "aac"})
			for i := 0; i < len(m.Formats)-1; i++ {
				m.Formats[i].ACodec = "none"
			}
		}
		token, e := newBrowserSecret()
		if e != nil {
			writeJSON(w, 200, reply)
			return
		}
		p = &clipPlan{Token: token, URL: clean, SourceURL: raw, Owner: owner, At: time.Now(), Meta: m, Choices: map[int]clipChoice{}, Silent: in.AllowSilent}
		if in.MediaURL != "" {
			p.Browser = &browserMedia{PageURL: clean, Title: m.Title, MediaURL: in.MediaURL, AudioURL: in.AudioURL, AllowSilent: in.AllowSilent, Separate: true}
		}
		s.mu.Lock()
		for k, old := range s.plans {
			if time.Since(old.At) > 3*time.Minute {
				delete(s.plans, k)
			}
		}
		if len(s.plans) >= 24 {
			s.mu.Unlock()
			writeJSON(w, 200, reply)
			return
		}
		s.plans[token] = p
		s.mu.Unlock()
	}
	qs := clipQualities(p.Meta)
	reply.Token = p.Token
	reply.Title = p.Meta.Title
	reply.Duration = p.Meta.Duration
	reply.Qualities = qs
	reply.Expires = p.At.Add(3 * time.Minute).UnixMilli()
	q := in.Quality
	if q == 0 && len(qs) > 0 {
		q = qs[0]
	}
	reply.Quality = q
	s.mu.Lock()
	choice, found := p.Choices[q]
	s.mu.Unlock()
	if !found {
		v, aud, e := clipSelect(p.Meta, q)
		choice = clipChoice{Quality: q, Status: "unsupported", Notice: clipNotice("range")}
		if e == nil {
			choice.Selector = v.ID
			if aud != nil {
				choice.Selector += "+" + aud.ID
			}
			choice.Video, e = clipPrepareInput(ctx, s.client, v)
			if e == nil && aud != nil {
				var ai clipInput
				ai, e = clipPrepareInput(ctx, s.client, *aud)
				choice.Audio = &ai
			}
			if e == nil {
				choice.Status = "supported"
				choice.Notice = clipNotice("ready")
			} else if ctx.Err() != nil || !errors.Is(e, errClipRange) {
				choice.Status = "unknown"
				choice.Notice = clipNotice("unknown")
			}
		}
		s.mu.Lock()
		p.Choices[q] = choice
		s.mu.Unlock()
	}
	reply.Partial = choice.Status
	reply.Reason, reply.Notice = clipChoiceFeedback(choice)
	writeJSON(w, 200, reply)
}
func clipPrepareInput(ctx context.Context, c *http.Client, f clipFormat) (clipInput, error) {
	in := clipInput{Format: f}
	if (f.HDR != "" && f.HDR != "SDR") || f.FPS > 120 {
		return in, errClipRange
	}
	if !clipMediaURL(f.URL) {
		return in, errClipRange
	}
	if f.Protocol == "m3u8" || f.Protocol == "m3u8_native" || strings.Contains(strings.Split(f.URL, "?")[0], ".m3u8") {
		resp, e := clipGet(ctx, c, f.URL, clipHeaders(f.Headers), "")
		if e != nil {
			return in, e
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return in, errClipRange
		}
		b, e := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
		if e != nil || len(b) > 1<<20 {
			return in, errClipRange
		}
		in.List, e = clipParsePlaylist(string(b), resp.Request.URL.String())
		return in, e
	}
	if f.Protocol != "" && f.Protocol != "http" && f.Protocol != "https" {
		return in, errClipRange
	}
	if f.Ext != "mp4" && f.Ext != "m4a" {
		return in, errClipRange
	}
	n, e := clipRangeProbe(ctx, c, f.URL, clipHeaders(f.Headers))
	in.Size = n
	return in, e
}
func clipParsePlaylist(text, base string) (*clipPlaylist, error) {
	if !strings.HasPrefix(strings.TrimSpace(text), "#EXTM3U") || !strings.Contains(text, "#EXT-X-ENDLIST") {
		return nil, errClipRange
	}
	for _, bad := range []string{"#EXT-X-KEY", "#EXT-X-BYTERANGE", "#EXT-X-DISCONTINUITY", "#EXT-X-STREAM-INF", "#EXT-X-I-FRAMES-ONLY", "#EXT-X-GAP"} {
		if strings.Contains(text, bad) {
			return nil, errClipRange
		}
	}
	u, e := url.Parse(base)
	if e != nil {
		return nil, errClipRange
	}
	p := &clipPlaylist{}
	d := 0.0
	resolve := func(raw string) (string, error) {
		r, e := url.Parse(raw)
		if e != nil {
			return "", errClipRange
		}
		abs := u.ResolveReference(r).String()
		if !clipMediaURL(abs) {
			return "", errClipRange
		}
		return abs, nil
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#EXT-X-MAP:") {
			if p.Init != "" || strings.Contains(line, "BYTERANGE") {
				return nil, errClipRange
			}
			at := strings.Index(line, `URI="`)
			if at < 0 {
				return nil, errClipRange
			}
			raw := strings.SplitN(line[at+5:], `"`, 2)[0]
			p.Init, e = resolve(raw)
			if e != nil {
				return nil, e
			}
		}
		if strings.HasPrefix(line, "#EXTINF:") {
			d, e = strconv.ParseFloat(strings.Split(strings.TrimPrefix(line, "#EXTINF:"), ",")[0], 64)
			if e != nil || d <= 0 || math.IsNaN(d) || math.IsInf(d, 0) {
				return nil, errClipRange
			}
		}
		if line != "" && !strings.HasPrefix(line, "#") {
			if d <= 0 || len(p.Segments) >= 20000 {
				return nil, errClipRange
			}
			raw, e := resolve(line)
			if e != nil {
				return nil, e
			}
			p.Segments = append(p.Segments, clipSegment{raw, p.Duration, d})
			p.Duration += d
			d = 0
		}
	}
	if len(p.Segments) < 2 {
		return nil, errClipRange
	}
	return p, nil
}
func clipSubset(p *clipPlaylist, start, end float64) ([]clipSegment, error) {
	out := []clipSegment{}
	for _, s := range p.Segments {
		if s.Start+s.Duration > start && s.Start < end {
			out = append(out, s)
		}
	}
	if len(out) == 0 || len(out) == len(p.Segments) || end > p.Duration+.05 {
		return nil, errClipRange
	}
	return out, nil
}
func clipTime(raw string) (float64, error) {
	if raw == "" || len(raw) > 20 || strings.TrimSpace(raw) != raw {
		return 0, errors.New("time")
	}
	parts := strings.Split(raw, ":")
	if len(parts) > 3 {
		return 0, errors.New("time")
	}
	n := 0.0
	for i, s := range parts {
		if !regexp.MustCompile(`^\d+(?:\.\d{1,3})?$`).MatchString(s) || (i < len(parts)-1 && strings.Contains(s, ".")) {
			return 0, errors.New("time")
		}
		v, e := strconv.ParseFloat(s, 64)
		if e != nil || (i > 0 && v >= 60) {
			return 0, errors.New("time")
		}
		n = n*60 + v
	}
	if n > 24*3600 {
		return 0, errors.New("time")
	}
	return n, nil
}
func (a *App) clipQueue(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token           string `json:"token"`
		Quality         int    `json:"quality"`
		Start           string `json:"start"`
		End             string `json:"end"`
		Partial         bool   `json:"partial"`
		OutputMode      string `json:"output_mode"`
		AllowDuplicates bool   `json:"allow_duplicates"`
	}
	if decodeBrowserJSON(w, r, &in) != nil {
		writeError(w, 400, clipNotice("times"))
		return
	}
	owner := clipOwner(a, r)
	s := a.clips
	s.mu.Lock()
	p := s.plans[in.Token]
	var ch clipChoice
	var ok bool
	if p != nil {
		ch, ok = p.Choices[in.Quality]
	}
	s.mu.Unlock()
	if p == nil || !ok || p.Owner != owner || time.Since(p.At) > 3*time.Minute {
		writeError(w, 409, clipNotice("expired"))
		return
	}
	if !clipSelectorValid(ch.Selector) {
		writeError(w, 409, clipNotice("range"))
		return
	}
	var spec *clipSpec
	if in.Partial {
		start, e1 := clipTime(in.Start)
		end, e2 := clipTime(in.End)
		if e1 != nil || e2 != nil || end <= start || end > p.Meta.Duration+.001 || end-start >= p.Meta.Duration-.001 {
			writeError(w, 400, clipNotice("times"))
			return
		}
		if ch.Status != "supported" {
			writeError(w, 409, ch.Notice)
			return
		}
		for _, i := range []*clipInput{&ch.Video, ch.Audio} {
			if i != nil && i.List != nil {
				if _, e := clipSubset(i.List, start, end); e != nil {
					writeError(w, 409, clipNotice("range"))
					return
				}
			}
		}
		spec = &clipSpec{Start: start, End: end, Quality: in.Quality, Token: p.Token}
	}
	// Explicitly selected full-source downloads still use the existing engine.
	// Partial retries never enter that path.
	mode := in.OutputMode
	if mode != "original" && mode != "premiere" {
		mode = "premiere"
	}
	a.mu.Lock()
	if a.newWorkBlockedLocked() || a.state.Phase != "ready" {
		a.mu.Unlock()
		writeError(w, 409, clipNotice("unknown"))
		return
	}
	pending := 0
	for _, j := range a.state.Jobs {
		if j.Status == "queued" || j.Status == "downloading" {
			pending++
		}
	}
	if pending >= 100 {
		a.mu.Unlock()
		writeError(w, 409, "queue full")
		return
	}
	a.created++
	id := fmt.Sprintf("clip-%d-%d", time.Now().UnixNano(), a.created)
	j := &Job{ID: id, URL: p.URL, Title: p.Meta.Title, Status: "queued", Time: time.Now().Format("2006-01-02 15:04"), OutputMode: mode, FormatSelector: ch.Selector, Clip: spec, Detail: clipNotice("processing")}
	_, j.Host, _ = cleanURL(p.URL)
	if owner != "web" {
		j.BrowserOwner = owner
	}
	if spec != nil {
		j.Task = "section"
		j.OutputMode = "premiere"
		j.Title = fmt.Sprintf("%s | %.3f - %.3f | %dp", p.Meta.Title, spec.Start, spec.End, spec.Quality)
	}
	if spec == nil && p.Browser != nil {
		j.BrowserSource = true
		media := *p.Browser

		if strings.HasSuffix(strings.Split(media.MediaURL, "?")[0], ".mp4") {
			j.FormatSelector = "browser-video"
			if media.AudioURL != "" {
				j.FormatSelector += "+browser-audio"
			}
		}
	}
	key := downloadJobKey(j)
	if a.hasQueuedDownloadLocked(key) {
		a.mu.Unlock()
		writeError(w, 409, "already queued")
		return
	}
	if d, exists := a.existingDownloadLocked(key); exists && !in.AllowDuplicates {
		a.mu.Unlock()
		writeJSON(w, 409, map[string]any{"error": "already downloaded", "duplicates": []duplicateInfo{d}})
		return
	}
	a.trimHistoryLocked()
	a.state.Jobs = append([]*Job{j}, a.state.Jobs...)
	if spec == nil && p.Browser != nil {
		a.browser.Sources[id] = *p.Browser
	}
	a.mu.Unlock()
	a.queue <- id
	writeJSON(w, 200, map[string]any{"ok": true, "count": 1, "job_ids": []string{id}})
}
func clipInputURL(g *clipGate, in clipInput, start, end float64) (string, float64, error) {
	if in.List == nil {
		// Leave a hard margin: even a pathological demuxer may not fetch the full
		// object, including re-requests. Small sources may therefore be refused.
		limit := in.Size * 8 / 10
		if limit < 4096 {
			return "", 0, errClipBudget
		}
		u := g.add(&clipResource{URL: in.Format.URL, Headers: clipHeaders(in.Format.Headers), Size: in.Size, limit: limit}, ".mp4")
		return u, start, nil
	}
	segs, e := clipSubset(in.List, start, end)
	if e != nil {
		return "", 0, e
	}
	var b strings.Builder
	b.WriteString("#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-PLAYLIST-TYPE:VOD\n#EXT-X-MEDIA-SEQUENCE:0\n")
	target := 1
	for _, s := range segs {
		target = max(target, int(math.Ceil(s.Duration)))
	}
	fmt.Fprintf(&b, "#EXT-X-TARGETDURATION:%d\n", target)
	if in.List.Init != "" {
		u := g.add(&clipResource{URL: in.List.Init, Headers: clipHeaders(in.Format.Headers), limit: 8 << 20}, ".mp4")
		fmt.Fprintf(&b, "#EXT-X-MAP:URI=\"%s\"\n", u)
	}
	for _, s := range segs {
		u := g.add(&clipResource{URL: s.URL, Headers: clipHeaders(in.Format.Headers), limit: 64 << 20}, ".ts")
		fmt.Fprintf(&b, "#EXTINF:%.6f,\n%s\n", s.Duration, u)
	}
	b.WriteString("#EXT-X-ENDLIST\n")
	return g.add(&clipResource{Body: []byte(b.String())}, ".m3u8"), start - segs[0].Start, nil
}
func (a *App) runClipJob(ctx context.Context, id string, j *Job) {
	fail := func(code string) {
		if !a.isJobCancelled(id) {
			a.mutateJob(id, func(j *Job) {
				j.Status = "error"
				j.ErrorCode = "CC-SECTION-" + strings.ToUpper(code)
				j.ErrorTitle = clipNotice(code)
				j.Detail = j.ErrorTitle
				j.ErrorAdvice = clipNotice("expired")
			})
		}
	}
	if j.Clip == nil {
		fail("times")
		return
	}
	s := a.clips
	s.mu.Lock()
	p := s.plans[j.Clip.Token]
	var ch clipChoice
	var ok bool
	if p != nil {
		ch, ok = p.Choices[j.Clip.Quality]
	}
	s.mu.Unlock()
	if p == nil || !ok || time.Since(p.At) > 3*time.Minute || ch.Status != "supported" {
		fail("expired")
		return
	}
	g, e := newClipGate(ctx, s.client)
	if e != nil {
		fail("unknown")
		return
	}
	defer g.close()
	inputs := []clipInput{ch.Video}
	if ch.Audio != nil {
		inputs = append(inputs, *ch.Audio)
	}
	args := []string{"-hide_banner", "-nostdin", "-v", "error", "-y"}
	var firstURL string
	for _, in := range inputs {
		u, seek, e := clipInputURL(g, in, j.Clip.Start, j.Clip.End)
		if e != nil {
			fail("range")
			return
		}
		if firstURL == "" {
			firstURL = u
		}
		args = append(args, "-rw_timeout", "8000000", "-protocol_whitelist", "http,tcp", "-accurate_seek", "-ss", strconv.FormatFloat(seek, 'f', 3, 64), "-i", u)
	}
	// Reject HDR, malformed or unsupported video headers before conversion.
	header, e := clipProbeRemote(ctx, a.ffmpeg, firstURL)
	if e != nil {
		fail("verify")
		return
	}
	if _, e = planEdit(header); e != nil {
		fail("verify")
		return
	}
	args = append(args, "-xerror", "-t", strconv.FormatFloat(j.Clip.End-j.Clip.Start, 'f', 3, 64), "-map", "0:v:0")
	if len(inputs) > 1 {
		args = append(args, "-map", "1:a:0")
	} else {
		args = append(args, "-map", "0:a?")
	}
	args = append(args, "-sn", "-dn", "-map_metadata", "-1", "-c:v", "libx264", "-preset", "veryfast", "-crf", "18", "-pix_fmt", "yuv420p", "-vf", "pad=ceil(iw/2)*2:ceil(ih/2)*2", "-c:a", "aac", "-b:a", "192k", "-movflags", "+faststart", "-progress", "pipe:1")
	root := filepath.Join(a.dataDir, "section-work")
	if os.MkdirAll(root, 0700) != nil {
		fail("unknown")
		return
	}
	dir, e := os.MkdirTemp(root, "clip-")
	if e != nil {
		fail("unknown")
		return
	}
	defer os.RemoveAll(dir)
	out := filepath.Join(dir, "section.mp4")
	args = append(args, out)
	cmd := hiddenCommandContext(ctx, a.ffmpeg, args...)
	cmd.Stderr = &cappedBuffer{Limit: 8192}
	pipe, e := cmd.StdoutPipe()
	if e != nil {
		fail("unknown")
		return
	}
	a.mutateJob(id, func(j *Job) { j.Detail = clipNotice("processing"); j.Stage = "encoding" })
	if e = cmd.Start(); e != nil {
		fail("unknown")
		return
	}
	a.setJobProcess(id, cmd)
	scanner := bufio.NewScanner(pipe)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "out_time_us=") {
			n, _ := strconv.ParseFloat(strings.TrimPrefix(line, "out_time_us="), 64)
			a.mutateJob(id, func(j *Job) { j.Percent = math.Max(0, math.Min(98, n/1e6/(j.Clip.End-j.Clip.Start)*98)) })
		}
	}
	e = cmd.Wait()
	a.clearJobProcess(id, cmd)
	if ctx.Err() != nil || a.isJobCancelled(id) {
		return
	}
	if ge := g.failure(); ge != nil {
		if errors.Is(ge, errClipBudget) {
			fail("budget")
		} else {
			fail("range")
		}
		return
	}
	if e != nil || scanner.Err() != nil {
		fail("verify")
		return
	}
	a.mutateJob(id, func(j *Job) { j.Stage = "verifying"; j.Percent = 99 })
	info, e := inspectEditMedia(ctx, a.ffmpeg, out)
	if e != nil {
		fail("verify")
		return
	}
	dur, _ := strconv.ParseFloat(info.Format.Duration, 64)
	expected := j.Clip.End - j.Clip.Start
	tolerance := .03
	if ch.Video.Format.FPS > 0 {
		tolerance += 1 / ch.Video.Format.FPS
	} else {
		tolerance = .08
	}
	if math.Abs(dur-expected) > tolerance {
		fail("verify")
		return
	}
	video, audio := false, false
	for _, st := range info.Streams {
		if st.Type == "video" && (st.Width < ch.Video.Format.Width || st.Height < ch.Video.Format.Height) {
			fail("verify")
			return
		}
		video = video || st.Type == "video"
		audio = audio || st.Type == "audio"
	}
	if !video || (!audio && !p.Silent && (ch.Audio != nil || ch.Video.Format.ACodec != "none")) {
		fail("verify")
		return
	}
	if verifyEditDecode(ctx, a.ffmpeg, out, func(cmd *exec.Cmd) {
		if cmd != nil {
			a.setJobProcess(id, cmd)
		}
	}) != nil {
		if ctx.Err() == nil {
			fail("verify")
		}
		return
	}
	if ctx.Err() != nil {
		return
	}
	name := fmt.Sprintf("Clip_%s_%0.3f-%0.3f_%dp.mp4", clipSafeName(p.Meta.Title), j.Clip.Start, j.Clip.End, j.Clip.Quality)
	dest, e := publishEditCopy(ctx, out, a.saveDir, name)
	if e != nil {
		fail("unknown")
		return
	}
	a.mutateJob(id, func(j *Job) {
		if j.Status != "cancelled" {
			j.File = dest
			j.Status = "done"
			j.Stage = "ready"
			j.Percent = 100
			j.EditReady = true
			j.Detail = clipNotice("done")
			j.ErrorCode = ""
		}
	})
}
func clipSafeName(raw string) string {
	var b strings.Builder
	for _, r := range raw {
		if r < ' ' || strings.ContainsRune(`<>:"/\|?*`, r) {
			continue
		}
		b.WriteRune(r)
		if b.Len() > 80 {
			break
		}
	}
	s := strings.Trim(b.String(), " .")
	if s == "" {
		s = "video"
	}
	return s
}

// Probe only through the bounded local gateway, never a raw signed URL.
func clipProbeRemote(ctx context.Context, ff, local string) (editMedia, error) {
	var info editMedia
	if ff == "" {
		return info, errors.New("clip-tools")
	}
	probeCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	out := &cappedBuffer{Limit: 65536}
	errout := &cappedBuffer{Limit: 4096}
	cmd := hiddenCommandContext(probeCtx, editProbePath(ff), "-v", "error", "-rw_timeout", "5000000", "-protocol_whitelist", "http,tcp", "-probesize", "262144", "-analyzeduration", "1000000", "-show_entries", "stream=codec_type,codec_name,width,height,pix_fmt,color_transfer,color_space,color_primaries,color_range,field_order,r_frame_rate,avg_frame_rate:format=duration", "-of", "json", local)
	cmd.Stdout = out
	cmd.Stderr = errout
	if e := cmd.Run(); e != nil {
		return info, e
	}
	if len(out.Data) >= out.Limit || json.Unmarshal(out.Data, &info) != nil {
		return info, errors.New("clip-probe")
	}
	return info, nil
}
func (a *App) clipExtractBrowserFile(ctx context.Context, raw string) (clipMetadata, error) {
	var m clipMetadata
	f := clipFormat{ID: "direct", URL: raw, Ext: "mp4", Protocol: "https"}
	in, e := clipPrepareInput(ctx, a.clips.client, f)
	if e != nil {
		return m, e
	}
	g, e := newClipGate(ctx, a.clips.client)
	if e != nil {
		return m, e
	}
	defer g.close()
	u := g.add(&clipResource{URL: raw, Headers: clipHeaders(nil), Size: in.Size, limit: min(in.Size/20, 1<<20)}, ".mp4")
	info, e := clipProbeRemote(ctx, a.ffmpeg, u)
	if e != nil || g.failure() != nil {
		return m, errClipRange
	}
	plan, e := planEdit(info)
	if e != nil {
		return m, errClipRange
	}
	f.Width, f.Height, f.VCodec = plan.Video.Width, plan.Video.Height, plan.Video.Codec
	f.FPS = frameRate(plan.Video.FrameRate)
	f.ACodec = "none"
	if len(plan.Audio) > 0 {
		f.ACodec = plan.Audio[0].Codec
	}
	m.Duration, _ = strconv.ParseFloat(info.Format.Duration, 64)
	m.Title = "Reddit video"
	m.Formats = []clipFormat{f}
	return m, nil
}
