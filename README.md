# ClipCatch v1.5.0

Reddit-first Windows video downloader with local browser interface and opt-in, integrity-checked GitHub Releases updates.

- Add up to 10 links at a time; highest available source quality using yt-dlp and FFmpeg without re-encoding by default.
- Reddit-specific connection-drop retries, IPv4 fallback and optional **Reddit-only** authentication. No automatic Chrome cookie access.
- Recently downloaded items: 8 displayed by default; older items can be expanded.
- **Duplicate protection:** identify the same Reddit/X/YouTube post across equivalent URLs; compare against active downloads and a persistent local index of successful downloads (up to 5,000 entries), and check that the original file still exists. Ask before downloading a duplicate; an explicit second download receives a distinct filename instead of overwriting the original.
- **Job cancellation:** cancel queued items or active yt-dlp downloads individually (and stop child processes on Windows where possible). Cancellation never marks a job as a failed download.
- **Bulk retry:** retry all failed jobs in the local history, without scheduling the same post more than once.
- **Update verification:** after a signed-off-by-checksum GitHub update, the updater checks the restarted application's version and health before writing a local success receipt shown in the UI; failures trigger rollback. Windows OS reputation / Smart App Control may still block unsigned executables.
- All user data, local history, and the duplicate index live under `%LOCALAPPDATA%\ClipCatch`; downloads go to `Downloads\ClipCatch`.
- No server/API fee and no background remote upload of downloads.

## Build

```sh
cd src
go test ./...
go vet ./...
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags='-s -w -H windowsgui -X main.appVersion=1.5.0' -o ../ClipCatch.exe .
```

To publish, bump the `VERSION` file and commit the matching source to `main`. GitHub Actions builds and publishes the executable and SHA-256 checksums. No account credentials are committed. The application is not Authenticode-signed.
