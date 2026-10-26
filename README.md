# ClipCatch v1.9.1

## ClipCatch v1.9.1 (stable)

- Promoted verified TEST2 1.9.1-test.5 behavior into stable ClipCatch.
- New-version update dialog preserves active jobs; new jobs wait for update. Offline check failure does not lock use.
- Private temporary workspaces publish only verified MP4 and clean only on success; failed sources and existing user originals remain preserved.
- Compatible MP4 files are byte-copied, otherwise lossy transcoding may be needed.
- Browser extension identifies build mismatch and offers reload; stable channel pairing and Chrome identity retained.
- TEST2 remains an independent prerelease; no server data modified.


## Premiere safety and optional local quality tools

Read [scope, limitations and evidence](docs/QUALITY_REVIEW.md). The main app remains one Windows EXE. Optional AI assets download only after consent. Image/video upscaling is not enabled on normal downloads. Preview before saving; keep originals.

Premiere repair explicitly produces a CFR H.264/AAC copy. An FFmpeg test is not a Premiere import test.


## Multiple videos in a single post

- Supported X/Twitter status posts and Instagram post/reel URLs request all video attachments returned by yt-dlp. Profiles, feeds and YouTube playlists are not expanded by this feature.
- The browser companion groups numbered X links into the original post and labels choices as all videos in that post. Explicit numbered X URLs pasted into the desktop app still target that numbered video.
- Every completed media file is retained in history. Premiere mode prepares each file, not only the last one, and preserves every original.
- Successful files survive partial failures. A post is marked complete only if all returned files pass preparation and the downloader did not report a failure.
- Old one-file history does not suppress a new whole-post request. A missing saved attachment makes the post eligible for retry.
- Downloading/decoding is not antivirus scanning; do not disable company Windows security or share cookie files.
- Desktop Windows only. No mobile server or mobile app has been added.

# ClipCatch v1.7.1

ClipCatch is a single-file Windows 64-bit downloader for approved public video platforms.

## v1.7.1: One ClipCatch installer, multi-platform Chrome helper

**No extra extension ZIP is required.** Updating ClipCatch through its existing
in-app updater also bundles the ClipCatch Browser v1.7.1 extension. The main
Windows executable is still the only downloadable installer.

- YouTube videos/Shorts, Instagram posts/Reels, X/Twitter posts, Reddit posts,
  TikTok, Facebook, Vimeo, Twitch, Streamable and Imgur: open a supported public
  video-post page, click the ClipCatch Browser icon, then **Current video download**.
- For Reddit only, the optional playing-video fallback detects selected
  `v.redd.it` streams when the post-link download fails.
- The extension sends the chosen post URL through the locally paired browser
  bridge. The same downloader queue, history, duplicate confirmation, and
  downloaded-file folder remain in use. It never reads site cookies or passwords.
- New users still need to **register the unpacked extension once** in
  `chrome://extensions`. Chrome will not silently install an unpacked extension.
- Existing extension users: after the app update/restart, open
  `chrome://extensions` and click **Reload** on ClipCatch Browser. The app
  refreshes the embedded extension files automatically when it detects a
  previously installed extension folder.
- Browser installation can be restricted by organization policy. Do not disable
  security protections; consult IT for approved deployment.
- Downloading login-restricted, private or DRM-protected media is not guaranteed
  and access restrictions are not bypassed. Use permitted videos only.

The original Reddit-only cookie registration, download logs, and app updater
are unchanged.

## Previous version: v1.7.0

ClipCatch is a local Windows 64-bit web-interface downloader for approved public video platforms, including Reddit, YouTube and X. It uses the official yt-dlp project and FFmpeg and preserves the highest available source quality without re-encoding by default.

## v1.7.0: Reddit browser handoff

When Reddit post extraction fails, open the public post in Chrome and use the included **ClipCatch Browser** extension. This is a separate path: the extension sends observed HTTPS `v.redd.it` media URLs to the app; the app does not request Reddit post JSON for these jobs.

### One-time setup
1. Update ClipCatch using the existing in-app updater.
2. Expand the browser handoff panel and click the extension-folder button. The bundled extension is written to `%LOCALAPPDATA%\ClipCatch\browser-extension`.
3. In `chrome://extensions`, use Developer mode and **Load unpacked**, selecting that folder. Respect company extension policies; do not disable system security controls.
4. Create a pairing code in ClipCatch and paste the entire code in the extension popup. The code is single-use and expires in five minutes.

### Daily use
Open the exact Reddit post, play its video, click **Find current post video**, and explicitly send the matching candidate to ClipCatch. If nothing is detected, refresh the post and play it with capture armed, then scan again. Completion is shown in ClipCatch, not inferred from the extension's handoff acknowledgement.

The extension prefers a media manifest actually observed on the current post. It may combine separately observed video/audio URLs only from the same media ID. It never invents a media URL. If only video is available, silent saving requires explicit consent. The app verifies both video and audio with FFprobe before reporting normal completion. A truly silent original is supported with the silent-saving checkbox.

### Permissions and limitations
- No cookies, passwords, browser-history databases, remote servers, or cloud account connections are used by the extension.
- Capture is limited by code to one explicitly armed Reddit tab for ten minutes; only CDN URLs are observed, not request headers. Candidate data stays in browser session memory. The link key stays in local extension storage and can be revoked in ClipCatch.
- Only a pairing-key digest is persisted by the app. Signed media URLs are not written to history or diagnostic reports. Temporary paired-stream request files are removed after use.
- The first version supports public Reddit-hosted MPD/HLS/MP4 streams, not every external video embed. DRM, CAPTCHA, paywalls and access-control bypass are not implemented.
- A playable page does not guarantee that its CDN permits direct downloads. The user's exact Reddit clip and their Windows browser environment require a live acceptance check; synthetic test success must not be presented as live Reddit success.
- Existing 30-result history, duplicate prevention, downloaded files, and Reddit-only saved authentication are preserved.
- This unpacked extension is not published to the Chrome Web Store. After a later app update changes the extension, prepare the folder again and reload its card in Chrome.

## v1.6.1 improvements
- Download history URLs open the original post in a new tab.
- Strip `utm_*` tracking keys from newly entered URLs and old history without removing required video parameters.
- Keep at most 30 completed/failed/cancelled jobs in history, while preserving active downloads, video files and the separate duplicate-detection index.
- Do not label generic Reddit 403/JSON parsing failures as login-required. Explicit authentication errors still request the user's Reddit-only session.
- The application still checks GitHub for releases on startup and offers a verified, user-confirmed in-app update.

## Core functions

- Accept up to 10 links per batch, retain the last 30 completed/failed/cancelled job results (8 expanded in the main screen), and allow an individual job to be cancelled or all failed jobs to be retried.
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
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags='-s -w -H windowsgui -X main.appVersion=1.7.0' -o ../ClipCatch.exe .
```

Publish by changing the root `VERSION` to `1.7.0` and committing all matching source code to `main`. GitHub Actions compiles a Windows binary, runs tests and publishes a new release with SHA-256 sums. Never commit login cookies or raw client logs.
