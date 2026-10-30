package main

import (
 "os"
 "path/filepath"
 "runtime"
 "strings"
 "testing"
)

func TestRedditAPIRestrictionIsNotLoginRequired(t *testing.T) {
 for _, raw := range []string{
  "ERROR: [Reddit] 1x0k17b: Unable to access the Reddit API",
  "ERROR: [Reddit] 1x0k17b: Blocked by network security",
 } {
  if !isRedditAPIRestricted(raw) || !shouldRetryReddit(raw) {
   t.Fatalf("Reddit API access error missed: %q", raw)
  }
  code, title, advice := classifyDownloadError(raw, "www.reddit.com")
  if code != "CC-104" || title == "" || !strings.Contains(advice, "ClipCatch Browser") {
   t.Fatalf("wrong classification: %q => %s %s %s", raw, code, title, advice)
  }
  if needsAuth(raw) { t.Fatalf("API block falsely requires credentials: %q", raw) }
  code, _, _ = classifyDownloadError(raw, "youtube.com")
  if code == "CC-104" { t.Fatalf("Reddit classification leaked to YouTube: %q", raw) }
 }
 if shouldRetryReddit("HTTP 429 Too Many Requests") {
  t.Fatal("rate limit must not trigger immediate retry")
 }
}

func TestRedditAPIBlockAfterLongOutputRetainsCause(t *testing.T) {
 if runtime.GOOS == "windows" { t.Skip("Unix shell stub") }
 tmp := t.TempDir()
 stub := filepath.Join(tmp, "fake-dlp")
 script := `#!/bin/sh
case " $* " in
 *" --impersonate chrome "*) ;;
 *) ;;
esac
printf 'ERROR: [Reddit] 1x0k17b: Unable to access the Reddit API\n' >&2
i=0
while [ "$i" -lt 20 ]; do
 printf 'WARNING: additional context %s\n' "$i" >&2
 i=$((i+1))
done
exit 1
`
 if err := os.WriteFile(stub, []byte(script), 0700); err != nil { t.Fatal(err) }
 a := newApp(tmp, filepath.Join(tmp, "downloads"))
 a.ytDlp = stub
 a.state.Jobs = []*Job{{ID:"reddit-api", Host:"www.reddit.com",URL:"https://www.reddit.com/r/catsistanbul/comments/1x0k17b/the_new_employee_has_an_idea/",Status:"queued"}}
 a.runDownload("reddit-api")
 j := a.getJob("reddit-api")
 if j == nil || j.Status != "error" || j.ErrorCode != "CC-104" || j.AuthRequired {
  t.Fatalf("incorrect error result: %+v", j)
 }
 if !strings.Contains(j.ErrorAdvice,"ClipCatch Browser") {t.Fatal("missing available fallback guidance")}
 if strings.Contains(j.Detail,"ERROR:") {t.Fatal("raw downloader errors must not be persisted")}
}
