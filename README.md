# ClipCatch v1.6.0

ClipCatch is a local Windows 64-bit web-interface downloader for approved public video platforms, including Reddit, YouTube and X. It uses the official yt-dlp project and FFmpeg and preserves the highest available source quality without re-encoding by default.

## Core functions

- Accept up to 10 links per batch, retain the last 50 job results (8 expanded in the main screen), and allow an individual job to be cancelled or all failed jobs to be retried.
- Detect duplicate videos across equivalent Reddit/X/YouTube links and a durable local download index; confirm before downloading an additional copy without overwriting the original.
- Handle Reddit connection drops with retries and an alternate network route. Users may explicitly provide **Reddit-only** cookies for videos that require access; ClipCatch does not extract Chrome, Edge or other browser login information.
- Check for ClipCatch application updates and install them only after user confirmation, with file SHA-256 validation, health/version verification and automatic rollback on failure.

## Stability and diagnostics (v1.6.0)

- Check the installed yt-dlp engine against the official SHA-256 checksum **in the background on launch**. This is a check only: no engine file is installed without an explicit button click.
- Offer stable engine update, optional upstream **nightly** engine recovery (official yt-dlp nightly-builds repository with SHA-256 validation), and one-click rollback to the previous engine.
- Preserve one previous engine and recover interrupted engine replacement during the next program startup where possible. Previously downloaded video files and history are untouched.
- Classify common download failures with human-readable titles, recommended next steps and local error codes (`CC-101`, etc.). Raw stderr is not saved for new failures.
- Provide a **Problem solving** panel that diagnoses tool executability, FFmpeg, download-folder write access, GitHub update reachability and Reddit reachability. This diagnosis runs only when requested.
- Copy a **redacted diagnostic report** containing versions, predefined statuses and error codes, but never video URLs, Windows file paths, cookies, credentials or raw command output. Clipboard failures fall back to a selectable text field.

## Scope and limitations

- Site login restrictions, deleted/private videos, service rate limits, network outages and platform extractor changes can still prevent downloads. A nightly yt-dlp engine may solve newly changed sites but is less stable than the release version; manual opt-in is required.
- The program does not bypass site access restrictions, does not edit videos or publish to social media, and does not transmit downloaded video files to a remote server.
- The application is not Authenticode-signed. Windows Smart App Control may block running unsigned binaries. Do not disable system security protections to use ClipCatch.

## Paths

- Program install: `%LOCALAPPDATA%\Programs\ClipCatch\ClipCatch.exe` (Windows user-local installation)
- Tools, engine backup and local history: `%LOCALAPPDATA%\ClipCatch`
- Videos: `%USERPROFILE%\Downloads\ClipCatch`

## Build and release

```sh
cd src
go test ./...
go vet ./...
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags='-s -w -H windowsgui -X main.appVersion=1.6.0' -o ../ClipCatch.exe .
```

Publish by changing the root `VERSION` to `1.6.0` and committing all matching source code to `main`. GitHub Actions compiles a Windows binary, runs tests and publishes a new release with SHA-256 sums. Never commit login cookies or raw client logs.
