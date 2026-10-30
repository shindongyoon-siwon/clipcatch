package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type redditFallbackRoundTripper func(*http.Request) (*http.Response, error)

func (fn redditFallbackRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestRedditPublicHTMLSourceIsOnlyExpectedPost(t *testing.T) {
	const postID = "1x0k17b"
	const video = "vidmediaabcd"
	for name, source := range map[string]string{
		"og video": `<html><head><meta property="og:video" content="https://v.redd.it/vidmediaabcd/DASHPlaylist.mpd"></head></html>`,
		"html escaped": `<meta content="https://v.redd.it/vidmediaabcd/DASHPlaylist.mpd?x=1&amp;y=2" property="og:video:url">`,
		"player in requested post": `<shreddit-post post-id="t3_1x0k17b"><shreddit-player src="https:\/\/v.redd.it\/vidmediaabcd\/DASHPlaylist.mpd"></shreddit-player></shreddit-post>`,
		"old post data url": `<div data-fullname="t3_1x0k17b" data-url="https://v.redd.it/vidmediaabcd"></div>`,
	} {
		got, err := extractRedditPublicVideoID([]byte(source), postID)
		if err != nil || got != video {
			t.Errorf("%s: %q, %v", name, got, err)
		}
	}
	for name, source := range map[string]string{
		"wrong post": `<shreddit-post post-id="t3_notthis"><shreddit-player src="https://v.redd.it/vidmediaabcd/DASHPlaylist.mpd"></shreddit-player></shreddit-post>`,
		"related video only": `<a href="https://v.redd.it/vidmediaabcd">recommended video</a>`,
		"image meta": `<meta property="og:image" content="https://v.redd.it/vidmediaabcd/DASHPlaylist.mpd">`,
		"ambiguous media": `<meta property="og:video" content="https://v.redd.it/vidmediaabcd/DASHPlaylist.mpd"><meta property="og:video:url" content="https://v.redd.it/othermedia99/DASHPlaylist.mpd">`,
	} {
		if got, err := extractRedditPublicVideoID([]byte(source), postID); err == nil || got != "" {
			t.Errorf("%s: unrelated media accepted %q (%v)", name, got, err)
		}
	}
}

func TestRedditPublicPageResolverUsesOnlyKnownHosts(t *testing.T) {
	const source = "https://www.reddit.com/r/catsistanbul/comments/1x0k17b/the_new_employee_has_an_idea/"
	requests := []string{}
	client := &http.Client{Transport: redditFallbackRoundTripper(func(req *http.Request) (*http.Response, error) {
		requests = append(requests, req.URL.String())
		body := ""
		status := 403
		if req.URL.Host == "old.reddit.com" {
			status = 200
			body = `<meta name="twitter:player:stream" content="https://v.redd.it/fallback0001/DASHPlaylist.mpd">`
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewBufferString(body)), Header: http.Header{"Content-Type": []string{"text/html"}}}, nil
	})}
	media, err := resolveRedditPublicVideo(context.Background(), source, client)
	if err != nil || media != "https://v.redd.it/fallback0001/DASHPlaylist.mpd" || len(requests) != 2 {
		t.Fatalf("media=%q requests=%v err=%v", media, requests, err)
	}
	for _, req := range requests {
		if !strings.Contains(req, "/comments/1x0k17b/") || strings.Contains(req, "?") {
			t.Fatalf("unsafe request: %s", req)
		}
	}
	for _, raw := range []string{"http://www.reddit.com/comments/1x0k17b/", "https://x.com/comments/1x0k17b/", "https://v.redd.it/1234567", "https://www.reddit.com/r/aww/"} {
		if _, err := resolveRedditPublicVideo(context.Background(), raw, client); err == nil {
			t.Fatalf("non-canonical post accepted: %q", raw)
		}
	}
	if len(requests) != 2 {
		t.Fatal("invalid URLs triggered a network request")
	}
}

func TestRedditPublicPageRateLimitIsNotRetried(t *testing.T) {
	tries := 0
	client := &http.Client{Transport: redditFallbackRoundTripper(func(req *http.Request) (*http.Response, error) {
		tries++
		return &http.Response{StatusCode: 429, Body: io.NopCloser(strings.NewReader("rate limited")), Header: make(http.Header)}, nil
	})}
	_, err := resolveRedditPublicVideo(context.Background(), "https://www.reddit.com/comments/1x0k17b/", client)
	if err == nil || tries != 1 {
		t.Fatalf("429 must stop retry: %v, tries=%d", err, tries)
	}
	for _, msg := range []string{"ERROR: Account authentication is required", "ERROR: [Reddit] Failed to parse JSON", "ERROR: HTTP 403 Forbidden"} {
		if !shouldAttemptRedditPageFallback("https://www.reddit.com/comments/1x0k17b/", "www.reddit.com", msg) {
			t.Fatalf("fallback skipped after public API failure: %s", msg)
		}
	}
	if shouldAttemptRedditPageFallback("https://www.reddit.com/comments/1x0k17b/", "www.reddit.com", "HTTP 429 Too Many Requests") {
		t.Fatal("fallback should not retry on 429")
	}
}
