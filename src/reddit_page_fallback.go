package main

import (
	"context"
	"errors"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var (
	redditFallbackPostIDRE = regexp.MustCompile(`(?i)(?:^|/)comments/([a-z0-9]{5,10})(?:/|$)`)
	redditFallbackMetaRE   = regexp.MustCompile(`(?is)<meta\b[^>]+>`)
	redditFallbackAttrRE   = regexp.MustCompile(`(?is)\b(property|name|content)\s*=\s*["']([^"']+)["']`)
	redditFallbackMediaRE  = regexp.MustCompile(`(?i)(?:https?:)?//v\.redd\.it/([a-z0-9]{6,20})(?:[/?#&"'\\\s<]|$)`)
	redditFallbackPlayerRE = regexp.MustCompile(`(?is)<shreddit-player\b[^>]+>`)
	redditFallbackRootRE   = regexp.MustCompile(`(?is)<(?:shreddit-post|div)\b[^>]+>`)
)

func redditFallbackPostID(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Scheme, "https") {
		return ""
	}
	h := strings.ToLower(u.Hostname())
	if h != "www.reddit.com" && h != "reddit.com" && h != "old.reddit.com" && h != "sh.reddit.com" {
		return ""
	}
	m := redditFallbackPostIDRE.FindStringSubmatch(strings.TrimPrefix(u.EscapedPath(), "/"))
	if len(m) != 2 {
		return ""
	}
	return strings.ToLower(m[1])
}

func shouldAttemptRedditPageFallback(rawURL, host, msg string) bool {
	if !isRedditHost(host) || redditFallbackPostID(rawURL) == "" {
		return false
	}
	s := strings.ToLower(msg)
	if strings.Contains(s, "429") || strings.Contains(s, "too many requests") {
		return false
	}
	// CC-199 is the original user-reported symptom; when no media was saved,
	// a public-page fallback can still help even if yt-dlp's stderr is unknown.
	code, _, _ := classifyDownloadError(msg, host)
	switch code {
	case "CC-103", "CC-104", "CC-106", "CC-199":
		return true
	}
	return false
}

func redditFallbackJobHasNoMedia(job *Job) bool {
	return job != nil && job.File == "" && len(job.Media) == 0
}

func unescapeRedditPageMedia(raw string) string {
	s := html.UnescapeString(raw)
	s = strings.ReplaceAll(s, `\/`, "/")
	s = strings.ReplaceAll(s, `\u002F`, "/")
	s = strings.ReplaceAll(s, `\u002f`, "/")
	s = strings.ReplaceAll(s, `\u002E`, ".")
	s = strings.ReplaceAll(s, `\u002e`, ".")
	return s
}

// extractRedditPublicVideoID only accepts first-party media attached to the
// requested post. A video found in comments or recommendations is not enough.
func extractRedditPublicVideoID(document []byte, postID string) (string, error) {
	if postID == "" || len(document) == 0 {
		return "", errors.New("invalid public post")
	}
	page := unescapeRedditPageMedia(string(document))
	var ids []string
	for _, tag := range redditFallbackMetaRE.FindAllString(page, 64) {
		attrs := make(map[string]string)
		for _, m := range redditFallbackAttrRE.FindAllStringSubmatch(tag, 12) {
			attrs[strings.ToLower(m[1])] = m[2]
		}
		prop := strings.ToLower(attrs["property"])
		if prop == "" {
			prop = strings.ToLower(attrs["name"])
		}
		if prop != "og:video" && prop != "og:video:url" && prop != "og:video:secure_url" && prop != "twitter:player:stream" {
			continue
		}
		if m := redditFallbackMediaRE.FindStringSubmatch(attrs["content"]); len(m) > 1 {
			ids = append(ids, m[1])
		}
	}
	if id := singleRedditFallbackID(ids); id != "" {
		return id, nil
	}
	if len(ids) > 0 {
		return "", errors.New("ambiguous post video metadata")
	}
	// Modern Reddit can expose the player inside a <shreddit-post> associated
	// with the exact post ID; old Reddit sometimes keeps data-url on the post.
	marker := "t3_" + strings.ToLower(postID)
	for _, tag := range redditFallbackRootRE.FindAllString(page, 300) {
		if !strings.Contains(strings.ToLower(tag), marker) {
			continue
		}
		// Only a media URL on the root element itself is unambiguous.
		if m := redditFallbackMediaRE.FindStringSubmatch(tag); len(m) > 1 {
			return m[1], nil
		}
		start := strings.Index(page, tag)
		if start < 0 {
			continue
		}
		limit := start + 70000
		if limit > len(page) {
			limit = len(page)
		}
		section := page[start:limit]
		if end := strings.Index(strings.ToLower(section), "</shreddit-post>"); end >= 0 {
			section = section[:end]
		}
		var observed []string
		for _, player := range redditFallbackPlayerRE.FindAllString(section, 12) {
			if m := redditFallbackMediaRE.FindStringSubmatch(player); len(m) > 1 {
				observed = append(observed, m[1])
			}
		}
		if id := singleRedditFallbackID(observed); id != "" {
			return id, nil
		}
	}
	return "", errors.New("no unambiguous public video metadata")
}

func singleRedditFallbackID(ids []string) string {
	seen := make(map[string]string)
	for _, id := range ids {
		if len(id) < 6 || len(id) > 20 {
			return ""
		}
		seen[strings.ToLower(id)] = id
		if len(seen) > 1 {
			return ""
		}
	}
	for id := range seen {
		return id
	}
	return ""
}

func redditFallbackClient() *http.Client {
	return &http.Client{
		Timeout: 9 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > 2 || req.URL.Scheme != "https" {
				return http.ErrUseLastResponse
			}
			switch strings.ToLower(req.URL.Hostname()) {
			case "reddit.com", "www.reddit.com", "old.reddit.com", "sh.reddit.com":
				if redditFallbackPostID(req.URL.String()) != "" {
					return nil
				}
			}
			return http.ErrUseLastResponse
		},
	}
}

// No cookie database, no Reddit API calls, no automatic CAPTCHA or access
// restriction bypass. Only public HTML reachable at a post's canonical URL.
func resolveRedditPublicVideo(ctx context.Context, rawPost string, client *http.Client) (string, error) {
	postID := redditFallbackPostID(rawPost)
	if postID == "" {
		return "", errors.New("not a Reddit post URL")
	}
	parsed, _ := url.Parse(rawPost)
	pagePath := parsed.EscapedPath()
	if !strings.HasSuffix(pagePath, "/") {
		pagePath += "/"
	}
	if client == nil {
		client = redditFallbackClient()
	}
	for _, host := range []string{"www.reddit.com", "old.reddit.com"} {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		req, err := http.NewRequestWithContext(ctx, "GET", "https://"+host+pagePath, nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("Accept", "text/html")
		req.Header.Set("User-Agent", "ClipCatch/"+appVersion)
		response, err := client.Do(req)
		if err != nil {
			continue
		}
		if response.StatusCode == http.StatusTooManyRequests {
			response.Body.Close()
			return "", errors.New("Reddit rate limit")
		}
		if response.StatusCode != http.StatusOK || !strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/html") ||
			!redditFallbackResponseMatches(response, req, postID) {
			response.Body.Close()
			continue
		}
		document, readErr := io.ReadAll(io.LimitReader(response.Body, 6<<20))
		response.Body.Close()
		if readErr != nil {
			continue
		}
		mediaID, err := extractRedditPublicVideoID(document, postID)
		if err == nil {
			return "https://v.redd.it/" + mediaID + "/DASHPlaylist.mpd", nil
		}
	}
	return "", errors.New("public post media unavailable")
}

func redditFallbackResponseMatches(response *http.Response, request *http.Request, postID string) bool {
	if response == nil || request == nil {
		return false
	}
	u := request.URL
	if response.Request != nil && response.Request.URL != nil {
		u = response.Request.URL
	}
	return u != nil && redditFallbackPostID(u.String()) == postID
}
