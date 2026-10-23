package main

import (
 "context"
 "fmt"
 "io"
 "net/http"
 "regexp"
 "strings"
 "testing"
 "time"
)

func TestRedditPublicPageLive(t *testing.T) {
 ctx,cancel:=context.WithTimeout(context.Background(),15*time.Second)
 defer cancel()
 url:="https://www.reddit.com/r/IllegallySmolCats/comments/1x1rb7t/buckle_up_buckaroo/"
 req,err:=http.NewRequestWithContext(ctx,http.MethodGet,url,nil)
 if err!=nil {t.Fatal(err)}
 req.Header.Set("User-Agent","Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
 req.Header.Set("Accept","text/html,application/xhtml+xml")
 resp,err:=(&http.Client{Timeout:15*time.Second}).Do(req)
 if err!=nil {t.Logf("live public HTML fetch unavailable: %v",err);return}
 defer resp.Body.Close()
 b,err:=io.ReadAll(io.LimitReader(resp.Body,4<<20))
 if err!=nil {t.Logf("live public HTML read unavailable: %v",err);return}
 data:=string(b)
 ids:=regexp.MustCompile(`https?:\\?/\\?/v\\?\\.redd\\?\\.it/[a-z0-9]+`).FindAllString(data,5)
 t.Logf("Public Reddit HTML probe: HTTP=%d contentType=%s bytes=%d shredditPlayer=%v playbackMediaJSON=%v redditVideo=%v dashMPD=%v videoLinks=%v",resp.StatusCode,resp.Header.Get("Content-Type"),len(b),strings.Contains(data,"shreddit-player"),strings.Contains(data,"playback-media-json"),strings.Contains(data,"v.redd.it"),strings.Contains(data,"DASHPlaylist"),ids)
 fmt.Println("REDDIT_LIVE_PROBE",resp.StatusCode,len(b),strings.Contains(data,"shreddit-player"),strings.Contains(data,"playback-media-json"),strings.Contains(data,"v.redd.it"),strings.Contains(data,"DASHPlaylist"))
}
