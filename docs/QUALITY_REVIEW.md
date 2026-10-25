# Quality tools / Premiere hardening review

## Current scope
- Keep the existing downloader, saved authentication, original media and app updater.
- Include the previously staged multi-file post fix: every reported file is tracked and converted; incomplete posts are never shown as full success.
- Premiere compatibility: inspect H.264 profile/pixel format, AAC profile, rotation, HDR/primaries, interlace. Decode all video and audio before marking an output ready.
- Explicit repair command: re-encode an original-preserving H.264 High / 8-bit 4:2:0 / AAC-LC 48 kHz / constant-frame-rate MP4. Preserve source frame-rate fraction and all audio tracks. Re-encoding is lossy; constant frame rate may duplicate/drop frames. No universal Premiere guarantee.
- Upscaling is opt-in only. Standard Lanczos enlargement is not AI and cannot restore missing information.
- AI uses the official Real-ESRGAN x4plus local NCNN model, blends 35% or 65% of its output with a standard enlarged original. 2x output uses native 4x inference followed by resizing. No face-replacement or generative face-restoration model.
- Preview before save: images or the first three seconds of the selected video segment. Any option/source change invalidates approval. Preview approval expires after one hour.
- Original-preserving PNG / H.264-AAC MP4 output; maximum output side 4096 pixels, 9 MP; video segments up to 120 seconds. AI frames are processed in batches of four, not an unbounded full video frame dump.
- Transparent images: standard resize is supported; AI declines to avoid damaged edges. Embedded image ICC profiles, HDR, wide-gamut and special interlaced inputs are declined rather than silently retagged/flattened.

## Installation and security
The app remains a single Windows EXE. Optional AI weights/tools are downloaded only after consent, from a fixed official GitHub release URL, checked against a reviewed SHA-256, and unpacked with traversal/symlink/size guards. Windows OpenMP runtime DLLs shipped in the pinned archive are included. No browser cookies, remote file uploads, paid inference service, or security-disable instruction is added. An integrity hash is not a malware scan or Authenticode signature.

Engine provenance: xinntao/Real-ESRGAN release v0.2.5.0, portable archive 20220424.
- Windows SHA256: abc02804e17982a3be33675e4d471e91ea374e65b70167abc09e31acb412802d
- Ubuntu SHA256: e5aa6eb131234b87c0c51f82b89390f5e3e642b7b70f2b9bbe95b6a285a40c96
- Reviewed real tiny-image inference: https://github.com/shindongyoon-siwon/clipcatch/actions/runs/40206885944
- Project/license: https://github.com/xinntao/Real-ESRGAN

## Premiere troubleshooting order
1. Try the produced MP4 in a fresh project; use a local drive with space and normal read access.
2. If the file itself has timing/profile issues, use the explicit compatibility repair command rather than renaming the extension.
3. For playback/conforming issues, Adobe documents clearing Media Cache in Preferences; this is not deletion of source media.
4. For H.264 decoder/GPU issues, compare hardware decoding on/off with restart as a diagnostic, restore the prior setting if it makes no difference; update Premiere and GPU drivers when appropriate.
5. For an unresolved H.264-specific importer issue, an intermediate ProRes/DNxHR copy can be made in an approved editor/transcoder. This is a fallback recommendation, not an implemented ClipCatch output mode.

Official references:
- https://helpx.adobe.com/premiere/desktop/organize-media/import-files/supported-file-formats.html
- https://helpx.adobe.com/premiere/desktop/troubleshooting/playback-issues/troubleshoot-sequence-and-file-interpretation-settings.html
- https://helpx.adobe.com/premiere/desktop/troubleshooting/playback-issues/choppy-playback-and-poor-performance-issue.html
- https://helpx.adobe.com/premiere/desktop/get-started/technical-requirements/enable-hardware-accelerated-decoding-support.html

## Verification boundaries
FFmpeg decode, format checks, synthetic media, Chromium fixtures and Windows cross-compilation are NOT the same as importing into Adobe Premiere. The user's Premiere installation and design-team GPU performance are unverified. Frame-by-frame AI can flicker and alter details; always compare. Private user media or original filenames must never be committed into this public repo. Local sample checks use brief excerpts only.
