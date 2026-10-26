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
	ErrorCode string `json:"error_code,omitempty"`
}

// These fixed messages are also the only per-file details allowed in a
// shareable diagnostic report. Never store a subprocess message or a path here.
var postMediaErrorMessages = map[string]string{
	"CC-421-SOURCE":      "저장 파일의 경로나 형식을 확인하지 못했어.",
	"CC-421-PATH":        "저장 파일이 작업 폴더 안에 있는지 확인하지 못했어.",
	"CC-421-MISSING":     "저장된 영상 파일을 찾지 못했어.",
	"CC-421-EMPTY":       "저장된 영상 파일이 비어 있어.",
	"CC-421-READ":        "저장된 영상 파일을 읽을 수 없어.",
	"CC-421-FORMAT":      "지원하지 않는 영상 파일 형식이야.",
	"CC-421-PROBE":       "영상 처리 도구나 저장된 영상의 형식을 확인하지 못했어.",
	"CC-421-TOOLS":       "영상 처리 도구를 실행할 수 없어.",
	"CC-421-NO-VIDEO":    "파일에서 영상 트랙을 찾지 못했어.",
	"CC-421-HDR":         "HDR·특수 색역 영상은 색상 변화를 막기 위해 자동 변환하지 않았어.",
	"CC-421-INTERLACED":  "인터레이스 영상은 자동 변환하지 않았어.",
	"CC-421-ROTATION":    "영상의 회전 정보를 처리하지 못했어.",
	"CC-421-RATE":        "영상의 프레임 간격을 확인하지 못했어.",
	"CC-421-STRUCTURE":   "영상의 프레임·회전·인터레이스 형식을 확인하지 못했어.",
	"CC-421-MULTI-TRACK": "영상 트랙이 여러 개인 파일은 자동 변환하지 않았어.",
	"CC-421-LIMIT":       "영상 크기나 트랙 수가 변환 범위를 넘었어.",
	"CC-421-STORAGE":     "MP4 저장에 실패했어. 저장 공간과 폴더 권한을 확인해 줘.",
	"CC-421-ENCODE":      "편집용 MP4로 변환하지 못했어.",
	"CC-421-VALIDATE":    "변환된 영상의 형식·크기·소리를 확인하지 못했어.",
	"CC-421-DURATION":    "원본과 변환된 영상의 길이가 맞지 않아.",
	"CC-421-DECODE":      "영상이나 소리의 전체 재생 검사에 실패했어.",
	"CC-421-TIMEOUT":     "영상 검사 시간이 길어져 작업을 멈췄어.",
	"CC-421-MP4":         "MP4 변환·검증에 실패했어.",
	"CC-421-UNKNOWN":     "파일을 완료하지 못했어. 세부 원인을 다시 확인해 줘.",
}

func postMediaEditErrorCode(err error) string {
	if err == nil {
		return ""
	}
	switch err.Error() {
	case "edit-input":
		return "CC-421-READ"
	case "edit-tools":
		return "CC-421-TOOLS"
	case "edit-probe":
		return "CC-421-PROBE"
	case "edit-no-video":
		return "CC-421-NO-VIDEO"
	case "edit-hdr":
		return "CC-421-HDR"
	case "edit-interlaced":
		return "CC-421-INTERLACED"
	case "edit-rotation":
		return "CC-421-ROTATION"
	case "edit-rate":
		return "CC-421-RATE"
	case "edit-multiple-video":
		return "CC-421-MULTI-TRACK"
	case "edit-limit":
		return "CC-421-LIMIT"
	case "edit-storage":
		return "CC-421-STORAGE"
	case "edit-encode":
		return "CC-421-ENCODE"
	case "edit-validation":
		return "CC-421-VALIDATE"
	case "edit-duration":
		return "CC-421-DURATION"
	case "edit-decode":
		return "CC-421-DECODE"
	case context.DeadlineExceeded.Error():
		return "CC-421-TIMEOUT"
	default:
		return "CC-421-MP4"
	}
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
			if m.Error == "" && m.ErrorCode == "" && m.File != "" {
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
			if m.Error != "" || m.ErrorCode != "" || !nonempty(m.File) {
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
	return postSourceErrorCode(root, path) == ""
}
func postSourceErrorCode(root, path string) string {
	if !filepath.IsAbs(path) {
		return "CC-421-PATH"
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp4", ".mkv", ".webm", ".mov", ".m4v", ".avi", ".ts", ".m2ts", ".flv", ".ogv":
	default:
		return "CC-421-FORMAT"
	}
	st, err := os.Stat(path)
	if os.IsNotExist(err) {
		return "CC-421-MISSING"
	}
	if err != nil {
		return "CC-421-READ"
	}
	if !st.Mode().IsRegular() {
		return "CC-421-FORMAT"
	}
	if st.Size() == 0 {
		return "CC-421-EMPTY"
	}
	// Resolve symlinks; a fake stdout marker cannot make conversion inspect a
	// file outside the chosen download folder. Source files are never deleted.
	base, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "CC-421-PATH"
	}
	actual, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "CC-421-PATH"
	}
	rel, err := filepath.Rel(base, actual)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "CC-421-PATH"
	}
	return ""
}
func (a *App) finishPostMedia(id string, ctx context.Context, convert bool) {
	j := a.getJob(id)
	if j == nil {
		return
	}
	count := len(j.Media)
	// The downloader may have failed after saving one of several videos. Keep
	// its classified cause; per-file checks and the completion summary supplement it.
	keepDownloadError := j.DownloadIncomplete && isDownloadErrorCode(j.ErrorCode)
	if count == 0 {
		a.mutateJob(id, func(j *Job) {
			if j.Status != "cancelled" {
				j.Status = "error"
				j.Detail = "완료된 영상 0개 · 저장된 파일을 확인하지 못했어."
				if !keepDownloadError {
					j.ErrorCode = "CC-421"
					j.ErrorTitle = j.Detail
					j.ErrorAdvice = "남아 있는 파일은 삭제하지 않았어. 문제 해결에서 진단 결과를 확인해 줘."
				}
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
		item.ErrorCode = ""
		if code := postSourceErrorCode(a.downloadRoot(j), item.Original); code != "" {
			item.ErrorCode = code
			item.EditReady = false
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
				}, func(cmd *exec.Cmd) { a.setJobProcess(id, cmd) })
				if ctx.Err() != nil || a.isJobCancelled(id) {
					return
				}
				if err != nil {
					item.ErrorCode = postMediaEditErrorCode(err)
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
			if err != nil {
				item.ErrorCode = postMediaEditErrorCode(err)
			} else if !hasVideo {
				item.ErrorCode = "CC-421-NO-VIDEO"
			} else {
				item.File = item.Original
			}
		}
		if item.ErrorCode != "" {
			item.Error = postMediaErrorMessages[item.ErrorCode]
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
		if !keepDownloadError {
			item.AuthRequired = false
		}
		if successes < count || item.DownloadIncomplete || item.MediaOverflow {
			item.Status = "error"
			item.Detail = fmt.Sprintf("%d/%d개 준비 완료 · 일부 영상 확인 필요", successes, count)
			if successes == 0 {
				item.Detail = "완료된 영상 0개 · 파일별 오류를 확인해 줘."
			}
			if !keepDownloadError {
				item.ErrorCode = "CC-421"
				item.ErrorTitle = item.Detail
				item.ErrorAdvice = "완료된 파일은 그대로 두었어. 문제가 있는 파일의 오류를 확인해 줘."
				if successes == 0 {
					item.ErrorAdvice = "남아 있는 파일은 삭제하지 않았어. 아래 파일별 오류를 확인해 줘."
				}
			}
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
