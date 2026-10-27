# Update replacement repair

Baseline: production v1.9.6, commit 5e8a1a73f7445c2c87fd2fc18a5afbe10b2445a9.
User requested a repair and design-team recovery instructions on 2026-10-27.
Changes are developed on a separate baseline copy and candidate branch. No old DEV196 or TEST2 workbench is merged.

Safety properties:
- File-handle transaction lock survives neither crash nor process exit. File existence is never treated as a live lock.
- Discover only exact installed-image process paths; enumerate loopback listeners by their owner PID. Same-name apps, other installations and test channels are not force-stopped.
- Check all owned endpoint identities/work states before graceful shutdown; wait for process termination, not merely HTTP success or port closure.
- Stage and hash-verify the incoming file before moving the working one. Use a unique backup. Locked legacy backups cannot block the new transaction.
- Verify the new app version AND listener PID before declaring success. Rollback kills only the process HANDLE started by this transaction.
- Launch gate permits only the updater's exact-path, hash-bound, short-lived token. It adds no Windows autostart entry or silent-update consent.
- The original two-argument updater command remains compatible with old released parents.

Required evidence before publication:
- Windows tests reproduce the old locked-backup rename failure and show repaired success.
- Delayed process exit, alternate loopback port, same-name unrelated process and cross-process lock tests.
- Native update of actual 1.9.5 and 1.9.6 executables while running, including a locked legacy backup and concurrent helpers.
- Existing rollback, browser, extension and media regression checks.

Limits: no direct access to the affected design-team PC. Antivirus or ACL denial can still block file replacement; these must be reported accurately, not treated as a running app or bypassed.
