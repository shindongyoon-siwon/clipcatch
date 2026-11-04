package main

// The section worker never gives FFmpeg an upstream URL. The loopback gate
// checks EVERY media response, including after the preflight probe. A 200 in
// response to a byte range is rejected without copying the body. VOD manifests
// are rebuilt with only the selected segments; there is no full-source fallback.
import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const clipChunk = int64(256 * 1024)

var errClipRange = errors.New("clip-range")
var errClipBudget = errors.New("clip-budget")

func clipPublicIP(ip net.IP) bool {
	return ip != nil && ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() && !ip.IsUnspecified()
}
func clipMediaURL(raw string) bool {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.User != nil || u.Fragment != "" || (u.Port() != "" && u.Port() != "443") {
		return false
	}
	h := strings.ToLower(u.Hostname())
	for _, d := range []string{"googlevideo.com", "cdninstagram.com", "fbcdn.net", "twimg.com", "v.redd.it"} {
		if h == d || strings.HasSuffix(h, "."+d) {
			return true
		}
	}
	return false
}
func clipHTTPClient() *http.Client {
	tr := &http.Transport{Proxy: nil, ForceAttemptHTTP2: true, DisableCompression: true, MaxIdleConns: 12, IdleConnTimeout: 30 * time.Second, ResponseHeaderTimeout: 5 * time.Second,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, e := net.SplitHostPort(addr)
			if e != nil || port != "443" {
				return nil, errors.New("clip-host")
			}
			ips, e := net.DefaultResolver.LookupIPAddr(ctx, host)
			if e != nil {
				return nil, e
			}
			for _, ip := range ips {
				if !clipPublicIP(ip.IP) {
					return nil, errors.New("clip-address")
				}
			}
			d := net.Dialer{Timeout: 5 * time.Second}
			for _, ip := range ips {
				c, e := d.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
				if e == nil {
					return c, nil
				}
			}
			return nil, errors.New("clip-connect")
		}}
	return &http.Client{Transport: tr, Timeout: 20 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) > 3 || !clipMediaURL(r.URL.String()) {
			return errors.New("clip-redirect")
		}
		return nil
	}}
}
func clipHeaders(h map[string]string) http.Header {
	out := http.Header{}
	for k, v := range h {
		if strings.ContainsAny(v, "\r\n") || len(v) > 4096 {
			continue
		}
		switch strings.ToLower(k) {
		case "user-agent", "referer", "origin", "accept", "accept-language":
			out.Set(k, v)
		}
	}
	out.Set("Accept-Encoding", "identity")
	return out
}
func clipGet(ctx context.Context, c *http.Client, raw string, h http.Header, byteRange string) (*http.Response, error) {
	req, e := http.NewRequestWithContext(ctx, "GET", raw, nil)
	if e != nil {
		return nil, e
	}
	req.Header = h.Clone()
	if byteRange != "" {
		req.Header.Set("Range", byteRange)
	}
	return c.Do(req)
}
func clipContentRange(s string) (int64, int64, int64, error) {
	var a, b, n int64
	if _, e := fmt.Sscanf(s, "bytes %d-%d/%d", &a, &b, &n); e != nil || a < 0 || b < a || n <= b || s != fmt.Sprintf("bytes %d-%d/%d", a, b, n) {
		return 0, 0, 0, errClipRange
	}
	return a, b, n, nil
}
func clipRangeProbe(ctx context.Context, c *http.Client, raw string, h http.Header) (int64, error) {
	// A real, nonzero-offset byte request; Accept-Ranges/HEAD alone is not proof.
	resp, e := clipGet(ctx, c, raw, h, "bytes=1-1")
	if e != nil {
		return 0, e
	}
	defer resp.Body.Close()
	if resp.StatusCode == 403 || resp.StatusCode == 401 || resp.StatusCode == 429 || resp.StatusCode >= 500 {
		return 0, errors.New("clip-access")
	}
	a, b, n, e := clipContentRange(resp.Header.Get("Content-Range"))
	if resp.StatusCode != 206 || e != nil || a != 1 || b != 1 || n < 4096 || (resp.ContentLength != -1 && resp.ContentLength != 1) {
		return 0, errClipRange
	}
	data, e := io.ReadAll(io.LimitReader(resp.Body, 2))
	if e != nil || len(data) != 1 {
		return 0, errClipRange
	}
	return n, nil
}

type clipResource struct {
	URL      string
	Headers  http.Header
	Size     int64  // >0 means verified ranged object; 0 means a selected VOD segment
	Body     []byte // a generated, bounded local manifest
	mu       sync.Mutex
	received int64
	limit    int64
}
type clipGate struct {
	client    *http.Client
	ctx       context.Context
	mu        sync.Mutex
	resources map[string]*clipResource
	prefix    string
	err       error
	server    *http.Server
	listener  net.Listener
}

type clipLocalListener struct{ net.Listener }

func (l clipLocalListener) Accept() (net.Conn, error) {
	c, e := l.Listener.Accept()
	if e == nil {
		if tcp, ok := c.(*net.TCPConn); ok {
			_ = tcp.SetWriteBuffer(16384)
		}
	}
	return c, e
}

func newClipGate(ctx context.Context, c *http.Client) (*clipGate, error) {
	token, e := newBrowserSecret()
	if e != nil {
		return nil, e
	}
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		return nil, e
	}
	g := &clipGate{client: c, ctx: ctx, resources: map[string]*clipResource{}, prefix: "/" + token + "/", listener: l}
	g.server = &http.Server{Handler: g, ReadHeaderTimeout: 5 * time.Second}
	go g.server.Serve(clipLocalListener{l})
	return g, nil
}
func (g *clipGate) close()         { g.server.Close() }
func (g *clipGate) failure() error { g.mu.Lock(); defer g.mu.Unlock(); return g.err }
func (g *clipGate) fail(e error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.err == nil {
		g.err = e
	}
}
func (g *clipGate) add(r *clipResource, ext string) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	p := g.prefix + strconv.Itoa(len(g.resources)) + ext
	g.resources[p] = r
	return "http://" + g.listener.Addr().String() + p
}
func (g *clipGate) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Host != g.listener.Addr().String() || r.URL.RawQuery != "" || (r.Method != "GET" && r.Method != "HEAD") {
		http.Error(w, "denied", 403)
		return
	}
	g.mu.Lock()
	v := g.resources[r.URL.Path]
	g.mu.Unlock()
	if v == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if v.Body != nil {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		if r.Method == "GET" {
			w.Write(v.Body)
		}
		return
	}
	ctx, cancel := context.WithCancel(g.ctx)
	defer cancel()
	stop := context.AfterFunc(r.Context(), cancel)
	defer stop()
	if v.Size == 0 {
		g.segment(w, r, ctx, v)
		return
	}
	if r.Method == "HEAD" {
		w.Header().Set("Content-Length", strconv.FormatInt(v.Size, 10))
		w.Header().Set("Accept-Ranges", "bytes")
		return
	}
	start, end := int64(0), v.Size-1
	if raw := r.Header.Get("Range"); raw != "" {
		if !strings.HasPrefix(raw, "bytes=") || strings.Contains(raw, ",") {
			http.Error(w, "range", 416)
			return
		}
		parts := strings.Split(strings.TrimPrefix(raw, "bytes="), "-")
		if len(parts) != 2 || parts[0] == "" {
			http.Error(w, "range", 416)
			return
		}
		var e error
		start, e = strconv.ParseInt(parts[0], 10, 64)
		if e != nil {
			http.Error(w, "range", 416)
			return
		}
		if parts[1] != "" {
			end, e = strconv.ParseInt(parts[1], 10, 64)
			if e != nil {
				http.Error(w, "range", 416)
				return
			}
		}
	}
	if start < 0 || end < start || start >= v.Size || end >= v.Size {
		http.Error(w, "range", 416)
		return
	}
	// Reserve per chunk; a demuxer may open a seek request before closing its old connection.
	first := true
	for at := start; at <= end; {
		v.mu.Lock()
		n := min(clipChunk, end-at+1, v.limit-v.received)
		if n <= 0 {
			v.mu.Unlock()
			g.fail(errClipBudget)
			if first {
				http.Error(w, "partial budget", 409)
			}
			return
		}
		v.received += n
		v.mu.Unlock()
		resp, e := clipGet(ctx, g.client, v.URL, v.Headers, fmt.Sprintf("bytes=%d-%d", at, at+n-1))
		if e != nil {
			if r.Context().Err() != nil {
				return
			}
			g.fail(e)
			if first {
				http.Error(w, "source", 502)
			}
			return
		}
		a, b, total, ce := clipContentRange(resp.Header.Get("Content-Range"))
		if resp.StatusCode != 206 || ce != nil || a != at || b != at+n-1 || total != v.Size || (resp.ContentLength != -1 && resp.ContentLength != n) {
			resp.Body.Close()
			g.fail(errClipRange)
			if first {
				http.Error(w, "partial unavailable", 409)
			}
			return
		}
		// At most one small chunk is buffered. Never cache the entire source.
		data, e := io.ReadAll(io.LimitReader(resp.Body, n+1))
		resp.Body.Close()
		if e != nil || int64(len(data)) != n {
			if r.Context().Err() != nil {
				return
			}
			g.fail(errClipRange)
			if first {
				http.Error(w, "range body", 502)
			}
			return
		}
		if first {
			w.Header().Set("Content-Type", "video/mp4")
			w.Header().Set("Accept-Ranges", "bytes")
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, v.Size))
			w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
			w.WriteHeader(206)
			first = false
		}
		if _, e = w.Write(data); e != nil {
			return
		}
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		at += n
	}
}
func (g *clipGate) segment(w http.ResponseWriter, r *http.Request, ctx context.Context, v *clipResource) {
	if r.Method == "HEAD" {
		http.Error(w, "GET only", 405)
		return
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.received >= v.limit {
		g.fail(errClipBudget)
		http.Error(w, "segment budget", 409)
		return
	}
	resp, e := clipGet(ctx, g.client, v.URL, v.Headers, "")
	if e != nil {
		g.fail(e)
		http.Error(w, "source", 502)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || resp.ContentLength > v.limit-v.received {
		g.fail(errClipRange)
		http.Error(w, "segment", 409)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	n, e := io.Copy(w, io.LimitReader(resp.Body, v.limit-v.received))
	v.received += n
	if e != nil && r.Context().Err() != nil {
		return
	}
	if e != nil {
		g.fail(errClipRange)
	}
	if n == v.limit {
		g.fail(errClipBudget)
	}
}
