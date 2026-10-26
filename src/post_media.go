package main

// A post may contain multiple separate videos, not multiple tracks in one file.
// Keep every file, convert each independently, and never claim partial success
// is complete. Nothing in this file opens downloaded media as a program.
import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type savedPostMedia struct {
	Original  string `json:"original"`
	File      string `json:"file"`
	EditReady bool   `json:"edit_ready,omitempty"`
	Error     string `json:"error,omitempty"`
}

var xWholePostRE = regexp.MustCompile(`^/(?:[^/]+/status|i/status)/[0-9]+/?$`)
var igWholePostRE = regexp.MustCompile(`^/(?:p|reel|tv)/[A-Za-z0-9_-]+/?$`)

func isWholeVideoPost(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return false
	}
	h := strings.ToLower(u.Hostname())
	domain := func(d string) bool { return h == d || strings.HasSuffix(h, "."+d) }
	return ((domain("x.com") || domain("twitter.com")) && xWholePostRE.MatchString(u.Path)) || (domain("instagram.com") && igWholePostRE.MatchString(u.Path))
}
func requestedDownloadKey(raw string) string {
	key := canonicalJobKey(raw)
	if isWholeVideoPost(raw) {
		u, _ := url.Parse(raw)
		h := strings.ToLower(u.Hostname())
		if h == "instagram.com" || strings.HasSuffix(h, ".instagram.com") {
			parts := strings.Split(strings.Trim(u.Path, "/"), "/")
			return "instagram:post:" + parts[len(parts)-1] + "|all-post-v1"
		}
		return key + "|all-post-v1"
	}
	return key
}
func cloneDownloadJob(j *Job) Job {
	cp := *j
	if j.Quality != nil {
		q := *j.Quality
		cp.Quality = &q
	}
	cp.Media = append([]savedPostMedia(nil), j.Media...)
	return cp
}
func postDownloadArgs(j *Job, args []string) []string {
	if !j.WholePost || j.BrowserSource || !isWholeVideoPost(j.URL) {
		return args
	}
	out := append([]string(nil), args...)
	for i, s := range out {
		if s == "--no-playlist" {
			out[i] = "--yes-playlist"
		}
		if s == "-o" && i+1 < len(out) {
			out[i+1] = "%(playlist_index&{} - |)s" + out[i+1]
		}
	}
	// No user profiles, search results or YouTube playlists reach this branch.
	// Hitting the limit returns an error, so it cannot be shown as full success.
	return append(out, "--no-overwrites", "--no-abort-on-error", "--max-downloads", "100")
}
func (a *App) recordPostFile(id, path string) {
	a.mutateJob(id, func(j *Job) {
		if j.Status == "cancelled" {
			return
		}
		j.File = path
		for _, f := range j.Media {
			if f.Original == path {
				return
			}
		}
		if len(j.Media) >= 100 {
			j.MediaOverflow = true
			return
		}
		j.Media = append(append([]savedPostMedia(nil), j.Media...), savedPostMedia{Original: path, File: path})
		j.Detail = fmt.Sprintf("\uC601\uC0C1 %d\uAC1C \uC800\uC7A5 \uD655\uC778 \uC911", len(j.Media))
	})
}
func allRecordFiles(files []string) bool {
	for _, f := range files {
		if !nonempty(f) {
			return false
		}
	}
	return true
}
func completedPostFiles(j *Job) []string {
	if len(j.Media) > 1 || j.WholePost {
		files := []string{}
		for _, m := range j.Media {
			if m.Error == "" && m.File != "" {
				files = append(files, m.File)
			}
		}
		return files
	}
	if j.File != "" {
		return []string{j.File}
	}
	return nil
}
func allSavedPostFiles(j *Job) bool {
	if j.DownloadIncomplete || j.MediaOverflow {
		return false
	}
	if len(j.Media) > 1 || j.WholePost {
		if len(j.Media) == 0 {
			return false
		}
		for _, m := range j.Media {
			if m.Error != "" || !nonempty(m.File) {
				return false
			}
		}
		return true
	}
	return nonempty(j.File)
}
func (a *App) setPostMedia(id string, index int, item savedPostMedia) {
	a.mutateJob(id, func(j *Job) {
		if index < 0 || index >= len(j.Media) {
			return
		}
		cp := append([]savedPostMedia(nil), j.Media...)
		cp[index] = item
		j.Media = cp
	})
}
func validPostSource(root, path string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp4", ".mkv", ".webm", ".mov", ".m4v", ".avi", ".ts", ".m2ts", ".flv", ".ogv":
	default:
		return false
	}
	st, err := os.Stat(path)
	if err != nil || !st.Mode().IsRegular() || st.Size() == 0 {
		return false
	}
	// Resolve symlinks; a fake stdout marker cannot make conversion inspect a
	// file outside the chosen download folder. Source files are never deleted.
	base, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}
	actual, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(base, actual)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel)
}
func (a *App) finishPostMedia(id string, ctx context.Context, convert bool) {
	j := a.getJob(id)
	if j == nil {
		return
	}
	count := len(j.Media)
	if count == 0 {
		a.mutateJob(id, func(j *Job) {
			if j.Status != "cancelled" {
				j.Status = "error"
				j.ErrorCode = "CC-421"
				j.Detail = "\uC800\uC7A5\uB41C \uC601\uC0C1\uC744 \uD655\uC778\uD558\uC9C0 \uBABB\uD588\uC5B4."
			}
		})
		return
	}
	a.mu.Lock()
	ff := a.ffmpeg
	a.mu.Unlock()
	successes := 0
	var firstGood string
	for i, item := range j.Media {
		if ctx.Err() != nil || a.isJobCancelled(id) {
			return
		}
		item.Error = ""
		if !validPostSource(a.downloadRoot(j), item.Original) {
			item.Error = "\uC800\uC7A5 \uD30C\uC77C \uACBD\uB85C\u00B7\uD615\uC2DD \uD655\uC778 \uD544\uC694"
		} else if convert {
			if !item.EditReady || !nonempty(item.File) {
				result, err := a.makeDownloadEditCopy(j, ctx, ff, item.Original, a.saveDir, filepath.Join(a.dataDir, "edit-work"), func(stage string, pct float64) {
					a.mutateJob(id, func(j *Job) {
						if j.Status != "cancelled" {
							j.Status = "downloading"
							j.Stage = "converting"
							j.Percent = (float64(i)*100 + pct) / float64(count)
							j.Speed = ""
							j.ETA = ""
							j.Detail = fmt.Sprintf("%d/%d \u00B7 %s", i+1, count, editStageText(stage))
						}
					})
				}, func(cmd *exec.Cmd) {
					a.mu.Lock()
					if a.activeJobID == id {
						a.activeCmd = cmd
					}
					a.mu.Unlock()
				})
				if ctx.Err() != nil || a.isJobCancelled(id) {
					return
				}
				if err != nil {
					item.Error = editErrorMessage(err)
					item.EditReady = false
				} else {
					item.File = result.File
					item.EditReady = true
				}
			}
		} else {
			info, err := inspectEditMedia(ctx, ff, item.Original)
			hasVideo := false
			for _, s := range info.Streams {
				if s.Type == "video" && s.Disposition.Attached == 0 {
					hasVideo = true
				}
			}
			if err != nil || !hasVideo {
				item.Error = "\uC601\uC0C1 \uD615\uC2DD \uAC80\uC0AC \uC2E4\uD328"
			} else {
				item.File = item.Original
			}
		}
		a.setPostMedia(id, i, item)
		if item.Error == "" {
			successes++
			if firstGood == "" {
				firstGood = item.File
			}
		}
	}
	a.mutateJob(id, func(item *Job) {
		if item.Status == "cancelled" {
			return
		}
		item.Stage = ""
		item.Speed = ""
		item.ETA = ""
		item.Percent = 100
		if firstGood != "" {
			item.File = firstGood
		}
		item.OriginalFile = item.Media[0].Original
		item.EditReady = convert && successes == count
		item.AuthRequired = false
		if successes < count || item.DownloadIncomplete || item.MediaOverflow {
			item.Status = "error"
			item.ErrorCode = "CC-421"
			item.Detail = fmt.Sprintf("%d\uAC1C \uC800\uC7A5 \u00B7 \uC77C\uBD80 \uC601\uC0C1 \uD655\uC778 \uD544\uC694", successes)
			item.ErrorTitle = item.Detail
			item.ErrorAdvice = "\uC131\uACF5\uD55C \uD30C\uC77C\uACFC \uC6D0\uBCF8\uC740 \uBCF4\uC874\uD588\uC5B4. \uC804\uCCB4 \uC644\uB8CC\uB85C \uCC98\uB9AC\uD558\uC9C0 \uC54A\uC558\uC5B4."
		} else {
			item.Status = "done"
			item.ErrorCode = ""
			item.ErrorTitle = ""
			item.ErrorAdvice = ""
			item.Detail = fmt.Sprintf("\uC601\uC0C1 %d\uAC1C \uC800\uC7A5 \uC644\uB8CC", successes)
			if convert {
				item.Detail = fmt.Sprintf("\uD3B8\uC9D1\uC6A9 MP4 %d\uAC1C \uC900\uBE44 \uC644\uB8CC \u00B7 \uC6D0\uBCF8 \uBCF4\uC874", successes)
			}
		}
	})
}
