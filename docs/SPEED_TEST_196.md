# DEV196 test.2: image grouping

Approved scope: reduce repeated AI program/model initialization for multiple images. Keep the existing model, tile size 128, processing threads 1:1:1, strength, output sizes, and video pipeline.

Up to four queued full-image previews with identical options share a single inference call. Total input pixels are capped at 3,000,000; larger groups split naturally. No AI process persists after the group. A single image is unchanged. Images are normalized and results validated individually. The full PNG pipeline after inference is unchanged.

Cancellation: cancelling one member discards its output without terminating siblings. Cancelling all members or exiting the program terminates the shared inference. During a shared call a cancelled image may finish internally, but its result is not returned. One group is not a promise of simultaneous kernel execution or fixed speedup. A first preview can be delayed until the small group completes.

Validation: unit/regression tests include engine-call counts, pixel/count limits, full-job reuse, partial failure, transparent inputs, source preservation, cancellation and cache reference lifetimes. A separate official-engine fixture test checks byte-identical final PNG output. Tiny synthetic fixture timings do not predict real user GPU performance.

Unapproved findings are NOT included as feature changes: completion history limit vs. 50-image input; per-file open/location shortcuts. Production remains v1.9.5. No autostart, extension pairing, installation, or production updater changes.
