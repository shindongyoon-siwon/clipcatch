# ClipCatch isolated development policy

Approved 2026-10-27. Production baseline: v1.9.5 / 1113a822aadb89c5882e461dbdd3b83b1139b0f1.

## Required workflow for every candidate

1. Start a separate branch and working directory from the exact production baseline. Do not reuse an old TEST2 working copy.
2. Keep production files, settings, cookies, history, download locations, browser extension and updater untouched.
3. Develop and test individual changes, then assemble one candidate. A development approval is NOT a production publication approval.
4. Validate the candidate on Windows, distinguish automated fixtures from actual user-PC and GPU testing, and retain evidence.
5. After user acceptance, review the expanded source diff. Rebuild with production configuration in a separate environment and repeat installation/update tests. Only an explicit final approval permits main merge or production release.
6. Never rename a test EXE to ClipCatch.exe or copy its settings into production.

## DEV196 identity

- Executable: ClipCatch-Dev196.exe; build version: 1.9.6-test.1.
- App identity: clipcatch-dev196; port: 18796.
- Data: LOCALAPPDATA/ClipCatch-Dev196; output: USERPROFILE/Downloads/ClipCatch-Dev196.
- The existing production (8765) and TEST2 (18766) identities are not used.
- Portable explicit launch; no self-install, no startup registration, no automatic app update, no production extension pairing.
- Normal download dependencies are fetched into its own data folder when missing. AI activation still requires the existing explicit confirmation.

## This branch is a reviewable development workbench

The existing v1.9.5 files are retained alongside tools/apply_dev196.py, a plain-text list of exact source replacements, and the new source files. The read-only CI job applies these replacements to its disposable checkout. This prevents partially staged edits from being executed and makes the changes reviewable. It does not modify GitHub source, main, releases or the user's PC.

The workflow runs only after dev/BUILD_READY changes. Its artifact includes the exact expanded, tested source ZIP and file hashes. That expanded source is already patched: build it directly, do not run apply_dev196.py again. Before promotion, materialize and review that source diff on a separate candidate branch. Do NOT merge this workbench wholesale into production.

## Candidate scope

- Existing lime icon and ClipCatch product metadata.
- Honest shutdown confirmation and active-job protection.
- HTTP/UI readiness before browser launch; existing-tab heartbeat and rapid-launch deduplication.
- Multiple image selection, user-initiated paste and file drop using the browser picker.
- Sequential quality queue, item status/cancellation/retry, partial-failure isolation and selected-result saving.
- Verified full-image preview reuse; video drafts remain separate from final processing.

## Limits and truthful test reporting

- Up to 50 images, 32 MiB per image; one video at a time, up to 512 MiB. Existing AI dimension/profile/transparency safeguards remain.
- Ordinary browser security may prevent bringing a background tab to the front; tab reuse is best effort, not guaranteed forced focus.
- Closing only the tab does not stop the app; the program shutdown button does. No automatic launch or silent startup is added.
- DOM paste/drop fixtures do not prove the actual Windows clipboard or desktop drag source. Native Windows lifecycle checks do not prove GPU inference quality.
- Rejected: introductory popups, startup registration, cloud hosting, redesigning the icon.
