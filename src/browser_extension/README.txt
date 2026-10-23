ClipCatch Browser v1.7.2 - bundled feed/video picker

Use:
1. Open a supported video page or a loaded Reddit/YouTube/X/Instagram feed.
2. Click ClipCatch Browser. It reads video cards already loaded in this tab.
3. Tick one or more videos, or Select All (maximum 10 per send).
4. Click Download selected. The popup reports queue acceptance, not completion.
5. Check actual download completion in ClipCatch.

If nothing is found, play the desired video or scroll to load it, then click Refresh.
No automatic feed scrolling or downloads are performed. Unsupported page layouts
may still require opening the individual post.

Multiple videos:
- Separate post cards are separate choices.
- Reddit media IDs actually observed inside one post are separate choices.
- Multiple resolutions of the same Reddit media ID are not duplicate videos.
- X numbered video links are separate only when those links are actually present.
- Instagram and other posts without individually identifiable URLs remain
  post-level choices; hidden carousel entries are not claimed as discovered.
- Audio is paired only from the same Reddit media ID. Video-only candidates need
  explicit silent-original consent; no audio URLs or streams are invented.

Update from v1.7.1:
Update the ClipCatch application, then reload the existing ClipCatch Browser card
in chrome://extensions (Edge: edge://extensions). The installed folder path and
pairing are retained. No separate extension ZIP is required.

No cookies, passwords or browser history are read. Signed media URLs stay only in
session memory and temporary job files. Use only permitted media. Authentication,
DRM, deleted videos or network restrictions may still cause download failures.
