# ClipCatch development and release policy

User-approved workflow (2026-10-27):

1. Copy the exact current production baseline into a separate development branch and working directory. Do not continue an unrelated old TEST2 source tree.
2. Isolate executable identity, installation/data/output paths, browser extension pairing, local port and update feed. Never register autostart without explicit opt-in.
3. Preserve the running production installation and existing user data. Test features separately and keep evidence, including skips and environment limits.
4. Review and materialize only approved functional changes onto a clean production-based release branch. Do not merge a development workbench wholesale or rename its executable.
5. Rebuild with stable identity and repeat native Windows installation, upgrade, rollback and data-preservation checks. Code tests are not proof of real user GPU or clipboard behavior.
6. Merge/publish only after the user explicitly authorizes deployment. The user authorized this v1.9.6 deployment on 2026-10-27.
7. Keep earlier release artifacts and verification hashes. Do not claim automatic recovery covers every runtime defect.

v1.9.6 baseline: 1113a822aadb89c5882e461dbdd3b83b1139b0f1 (v1.9.5).
Approved development source: db3ca0446e5a13776f2f12e4368d4eea15c0c31e.
The DEV196 branch remains independent; production keeps its existing identity, installer, updater and browser integration.

Rejected scope: first-run explanatory popup, file-open/location buttons, save-folder preferences, automatic startup, cloud hosting and icon redesign.

## v2.0.0

User authorized development and release on 2026-10-27. Baseline: v1.9.9 / 3c001a3681294beac6b3e1b3f8adc2c6f793bc52.
Approved scope: activity-only stable update discovery, shared/persisted one-hour cooldown, last-tab shutdown with a 15-second grace and active-work protection. Existing updater verification, transactions and release identity remain unchanged.
Native candidate checks now include running 1.9.9, locked backup/concurrent helpers and real Chrome lifecycle. Published official 1.9.9 button/manual paths are checked separately after release. This is not a claim of testing the design team workstation.
