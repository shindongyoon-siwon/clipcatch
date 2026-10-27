# Unsaved quality results: development-only correction

Approved scope (2026-10-27): fix unsaved results only. File-open/location buttons
and save-folder preferences were rejected and are NOT implemented. No extra user
acceptance build is requested. Production merge/release still needs approval.

## Root cause

The live quality workspace accepts 50 sources, but it shared a list pruned to
30 completed history entries. Each completed task and new save pruned that list,
so an earlier approved preview could disappear before its save worker read it.
A one-hour preview age check also invalidated results in a long-running batch.

## Correction

- Keep the latest preview and save status for each source still in the workspace,
  including failures and cancellations so retry remains available.
- Keep the exact preview referenced by a retained or active save. Register a new
  save before trimming history, not after.
- These bounded workspace references do not consume the 30 ordinary history slots.
  Repeated attempts do not retain an unlimited history per source.
- Referenced current previews do not expire after one hour in the active program
  session. Old, unreferenced previews keep the former one-hour lifetime.
- Explicit source removal releases history protection. It does not delete user
  originals or saved outputs. Existing owned-input removal is unchanged.
- Existing preview fingerprint, source-change, path and output validation remains.
- No changes to model, tile size, threading, scaling, browser UI, download engine,
  auto-update cadence, installer, startup behavior, or production identity.

## Limits

This is retention during the running session, not crash/reboot recovery. Existing
startup cleanup and 30-entry on-disk recent history are unchanged. Programs should
still be closed only after saving desired outputs. Image files are kept on disk;
this change does not load all 50 full images into RAM.

## Verification

Six regression tests cover: 50 real PNG uploads/previews and exact saved outputs;
50-item mixed failures/cancellations/retries; bounded history and explicit removal;
queued saves of superseded previews; unchanged source/settings verification; and
concurrent history operations under the Go race detector. Resize fixtures isolate
retention from GPU/model differences. Existing batch/preview/video tests also run;
GPU inference is not required or claimed by these retention-specific checks.

Development builds use apply_dev196.py, gofmt, then apply_speed196.py and gofmt.
The exact expanded source must be reviewed and revalidated before production.
This change does not update VERSION or distribute a new executable to end users.
