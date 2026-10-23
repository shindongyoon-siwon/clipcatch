# Feed picker 1.7.2

Fixes the v1.7.1 UX regression shown on a Reddit subreddit listing: the old popup
accepted only a canonical individual-post URL and disabled the action on feeds.

New behavior: on-demand, bounded DOM discovery; checkbox selection of up to 10
loaded video candidates; independent outcome for each queued item; selected
Reddit media have separate persistent duplicate keys, even within one post.

Original post URLs, downloaded files, previous history, Reddit-only authentication
and the one-EXE application updater are preserved. No automatic feed crawling,
cookie extraction, system security changes or new broad host permissions.

Known limits: only loaded/identifiable cards; no claim that every site's hidden
carousel media is individually selectable. Generic platforms still use the
existing yt-dlp post extractor. No live login/CDN download or design-team Windows
security acceptance has been performed by these synthetic tests.

QA consists of Go unit/race/vet checks, Windows unit/build checks, JS core/worker
tests, existing engine/audio fixtures and a new actual Chromium extension +
synthetic Reddit feed/local bridge test. CI run results, not this document, are
the source of truth for whether a given commit passed.
