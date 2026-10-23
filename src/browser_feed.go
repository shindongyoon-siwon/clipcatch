package main

import (
 "regexp"
 "strings"
)

// Media identity in opaque browser job IDs survives history reload without
// persisting signed URLs. Legacy jobs retain their original identity.
var separateBrowserJobRE = regexp.MustCompile(`^bm_([a-z0-9]{5,40})_[0-9]+_[0-9]+$`)
func selectedRedditKey(page, media string) string { return canonicalJobKey(page) + "|reddit-media:" + media }
func downloadJobKey(j *Job) string {
 if j.BrowserSource { if m := separateBrowserJobRE.FindStringSubmatch(j.ID); m != nil { return selectedRedditKey(j.URL, m[1]) } }
 return canonicalJobKey(j.URL)
}
func numberedXKey(rawPath, postKey string) string {
 m := regexp.MustCompile(`(?i)/status/[0-9]+/video/([1-9][0-9]?)/?$`).FindStringSubmatch(strings.TrimRight(rawPath, "/"))
 if m != nil { return postKey + ":video:" + m[1] }
 return postKey
}
