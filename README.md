# ClipCatch v1.4.1

Reddit-first, local Windows video downloader with opt-in, integrity-checked GitHub Releases updates.

- URL allowlist, strict same-origin POST validation, localhost-only API.
- High-quality yt-dlp downloads and ffmpeg merge; no re-encoding by default.
- Reddit post normalization; connection-drop recovery with capped yt-dlp retries and an IPv4 fallback; browser impersonation for access blocks only.
- When Reddit requests authentication, **only user-imported Reddit cookies** are supported. `--cookies-from-browser` is disabled. Cookies are filtered to exact `reddit.com` domains, used only for Reddit requests and handled via temporary files deleted after each attempt.
- Optional *per-user* Windows DPAPI encryption for remembering Reddit login; off by default.
- Existing `%LOCALAPPDATA%\ClipCatch` and `Downloads\ClipCatch` are retained.
- App update checks GitHub Releases, validates SHA-256, requires an explicit Install click, backs up/restarts/rolls back. Not silently executed.
- The app is **not Authenticode-signed** and cannot substitute for corporate IT approval.

## Build and test

```sh
cd src
go test ./...
go vet ./...
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags='-s -w -H windowsgui -X main.appVersion=1.4.1' -o ../ClipCatch.exe .
```

Publish by updating `VERSION`, committing source to `main`, and letting GitHub Actions release a new version. No credentials shipped in the application. **Never commit cookies.**
