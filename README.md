# ClipCatch - video link downloader

ClipCatch is a local Windows web app with a dedicated, versioned auto-update channel.

## Install / update

- Download `ClipCatch.exe` from [Releases](../../releases/latest).
- Run it once to install into `%LOCALAPPDATA%\Programs\ClipCatch\ClipCatch.exe` and create a Start Menu shortcut (plus Desktop shortcut, where available).
- Keep using that shortcut; the app checks GitHub Releases when it launches.
- When an update is available, use **업데이트 설치** in ClipCatch. The EXE is downloaded, SHA-256 verified and replaced with restart/rollback.
- Downloaded videos and history are kept in the same locations as earlier versions.

## Publishing new versions

1. Update `VERSION` to the next SemVer, such as `1.3.1`.
2. Commit the code changes to `main`.
3. GitHub Actions runs `go test`, cross-compiles the Windows binary, generates `SHA256SUMS` and creates `vX.Y.Z` Release automatically.
4. Existing ClipCatch clients detect the new version on next launch. **No user-side ZIP extraction.**

Do not republish an existing version: the release workflow fails if the version already exists. No GitHub token is required on end-user machines.

## Privacy and limitations

- Local app server binds to loopback; downloaded files remain on the user's computer.
- Cookies, if selected by the user, are used locally and must never be included in GitHub commits, logs or support tickets.
- GitHub public repository & public Release are needed for login-free update checks.
- Public downloading does **not** imply the right to republish or reuse other people's videos.
- Some platforms restrict automated access and may return HTTP 403 / require login.
- Neither the application nor its release binaries are code-signed; security software may display a warning. Update integrity is checked using GitHub's release hash and `SHA256SUMS`. This is not a substitute for code-signing.
- Deploy through the public repository `shindongyoon-siwon/clipcatch`. GitHub Actions publishes versioned Releases from the main branch when maintainers commit a new version.

## Development

Requires Go 1.23+.

```sh
cd src
go test ./...
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags='-s -w -H windowsgui' -o ../ClipCatch.exe .
```