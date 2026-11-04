# ClipCatch section/quality candidate

Version: **2.1.0-test.1**. Baseline: production **2.0.0**, commit `161817a3d15d4d66029fcd35ac21953c3255f94d`.
Branch: `test/section-quality-preflight`. Do not merge into main or publish a stable release without approval.

## User contract

- Paste one link: metadata and selected-quality partial support are inspected automatically, bounded by a 12-second request deadline. Success, unsupported, and inconclusive are separate results. A timeout is not a claim that the source does not support partial access.
- Choose an actual source resolution. Changing quality rechecks that format. No silent resolution downgrade and no automatic upscaling.
- Optional section-only checkbox with explicit start/end input (seconds, mm:ss, hh:mm:ss; up to milliseconds). No artificial lead-in or tail-out. Exact cuts are re-encoded once to H.264/AAC MP4 and checked at frame/sample precision.
- No full-download fallback, including temporary full-source files. Failed/unsupported section jobs stop. A manual full-video choice is a separate action.
- MP4 inputs must answer an actual byte-range probe. A tokenized loopback gateway checks every subsequent range response. Responses ignoring Range are rejected before their body is copied. Per-input cumulative request budgets leave at least a 20% safety margin, so very short/full-adjacent clips or expensive indexes may intentionally be refused.
- HLS VOD inputs use a generated manifest containing only intersecting segments. Any range requiring every source segment is refused. Encryption, live streams, discontinuities, byte-range manifests and currently unsupported DASH layouts are refused rather than guessed.
- A little header/keyframe/segment data outside the output interval is necessary for decoding; this is NOT extra output padding. A positive preflight is protocol evidence, not a guarantee against later URL expiry or network failures.
- Multi-video posts need an individual video selection/link. The existing normal full-download batch workflow remains available.

## Isolation and installation

Download `ClipCatch-SectionTest.exe` from this branch's Actions artifact, then run it directly. It is a portable, separate candidate, not a stable installer and not the legacy TEST2 executable.

- App ID: `clipcatch-section-test`; channel: `section-test`.
- Local UI: `http://127.0.0.1:18767` (production 8765 and legacy TEST2 18766 are untouched).
- Data/tools/helper: `%LOCALAPPDATA%\ClipCatch-SectionTest`.
- Videos: `%USERPROFILE%\Downloads\ClipCatch-SectionTest` by default.
- No auto-update feed, production share link, autostart, existing shortcut replacement or credential/profile extraction.
- The included extension is generated into the candidate's separate folder as **ClipCatch Browser SECTION TEST**. Load this separate unpacked folder and pair with this candidate only. Do not replace/reload the production helper as the test helper.
- Reddit remains an explicitly user-operated extension path. No automatic web-app-to-browser recovery was introduced. In the helper, select one video (or use the playing-video scan), then inspect quality/section settings.

## Verification and limits

Automated checks cover the real local byte-range gate, actual FFmpeg MP4/HLS clipping, total transfer being less than the source, no unselected HLS segments, exact supplied times, owner-bound/expired plans, failed range responses, no implicit full download, isolated identities, existing regression tests and actual Chromium controls.

A generated fixture is not a live Instagram/YouTube/X/Reddit success test. Cloud-runner access restrictions, user-login requirements, site changes, expiring media URLs, source type, FFmpeg versions and GPU/CPU performance still vary. No four-platform zero-error or performance guarantee is claimed. Premiere import on a design-team workstation has not been verified.

If a section fails, preserve its request/time inputs and report the fixed section error code. Never reuse a different video from a multi-video post and never fill an error by downloading the whole source.
