package main

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// A browser handoff never falls back to the broken Reddit post extractor and
// never passes Reddit cookies to media requests. It uses the observed CDN URLs.
func (a *App) runBrowserJob(id string, j *Job) {
	err, msg := a.runDownloadAttempt(id, j, false)
	if a.isJobCancelled(id) {
		return
	}
	if err != nil {
		if msg == "" {
			msg = err.Error()
		}
		a.mutateJob(id, func(job *Job) {
			if job.Status == "cancelled" {
				return
			}
			job.Status = "error"
			job.AuthRequired = false
			job.ErrorCode, job.ErrorTitle, job.ErrorAdvice = classifyDownloadError(msg, job.Host)
			if strings.Contains(msg, "CC-312") {
				job.ErrorCode = "CC-312"
				job.ErrorTitle = "\ube0c\ub77c\uc6b0\uc800 \uc601\uc0c1 \uc8fc\uc18c\ub97c \ub2e4\uc2dc \ubc1b\uc544\uc57c \ud574"
				job.ErrorAdvice = "\uc6d0\ubcf8 \uac8c\uc2dc\ubb3c\uc744 \uc5f4\uc5b4 \uc7ac\uc0dd\ud558\uace0 \ud655\uc7a5 \ud504\ub85c\uadf8\ub7a8\uc5d0\uc11c \ub2e4\uc2dc \ubcf4\ub0b4 \uc918."
			} else if job.ErrorCode == "CC-104" || job.ErrorCode == "CC-103" {
				job.ErrorCode = "CC-314"
				job.ErrorTitle = "\uc601\uc0c1 \uc11c\ubc84\uac00 \uc9c1\uc811 \uc800\uc7a5\uc744 \uac70\ubd80\ud588\uc5b4"
				job.ErrorAdvice = "\ube0c\ub77c\uc6b0\uc800 \uc7ac\uc0dd\uacfc \ubcc4\uac1c\ub85c \uc601\uc0c1 \uc8fc\uc18c\uac00 \ub9cc\ub8cc\ub410\uac70\ub098 \uc811\uadfc\uc774 \uc81c\ud55c\ub420 \uc218 \uc788\uc5b4. \ub2e4\uc2dc \uac10\uc9c0\ud574\ub3c4 \uc548 \ub418\uba74 \uc774 \uacbd\ub85c\ub85c\ub294 \uc800\uc7a5\ud560 \uc218 \uc5c6\uc5b4."
			}
			job.Detail = job.ErrorTitle
		})
		return
	}
	got := a.getJob(id)
	if got == nil || !nonempty(got.File) {
		a.browserJobError(id, "CC-315", "\uc800\uc7a5\ub41c \uc601\uc0c1 \ud30c\uc77c\uc744 \ud655\uc778\ud558\uc9c0 \ubabb\ud588\uc5b4")
		return
	}
	a.mu.Lock()
	media := a.browser.Sources[id]
	ctx := a.activeContext
	a.mu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	video, audio, err := a.inspectBrowserOutput(ctx, got.File)
	if a.isJobCancelled(id) {
		return
	}
	if err != nil || !video {
		a.browserJobError(id, "CC-315", "\uc800\uc7a5 \ud30c\uc77c\uc758 \uc601\uc0c1\u00b7\uc74c\uc131 \uac80\uc0ac\uc5d0 \uc2e4\ud328\ud588\uc5b4")
		return
	}
	if !audio && !media.AllowSilent {
		a.browserJobError(id, "CC-313", "\uc800\uc7a5\ub41c \uc601\uc0c1\uc5d0 \uc74c\uc131\uc774 \uc5c6\uc5b4")
		return
	}
	a.mutateJob(id, func(job *Job) {
		if job.Status == "cancelled" {
			return
		}
		job.Status = "done"
		job.Percent = 100
		job.Speed = ""
		job.ETA = ""
		job.ErrorCode = ""
		job.ErrorTitle = ""
		job.ErrorAdvice = ""
		job.AuthRequired = false
		job.Detail = "\ube0c\ub77c\uc6b0\uc800 \uc601\uc0c1 \uc800\uc7a5 \uc644\ub8cc"
		if audio {
			job.Detail += " \u00b7 \uc601\uc0c1\u00b7\uc74c\uc131 \ud655\uc778"
		} else {
			job.Detail += " \u00b7 \ubb34\uc74c \uc800\uc7a5 \ud5c8\uc6a9\ub428"
		}
	})
}
func (a *App) browserJobError(id, code, title string) {
	a.mutateJob(id, func(j *Job) {
		if j.Status == "cancelled" {
			return
		}
		j.Status = "error"
		j.AuthRequired = false
		j.ErrorCode = code
		j.ErrorTitle = title
		j.Detail = title
		j.ErrorAdvice = "\uc6d0\ubcf8\uc774 \ubb34\uc74c \uc601\uc0c1\uc778\uc9c0 \ud655\uc778\ud574 \uc918. \ubb34\uc74c\uc774 \ub9de\uc73c\uba74 \ud655\uc7a5\uc758 [\ubb34\uc74c \uc800\uc7a5 \ud5c8\uc6a9]\uc744 \uccb4\ud06c\ud574 \ub2e4\uc2dc \ubcf4\ub0b4 \uc918. \uae30\uc874 \ud30c\uc77c\uc740 \uc0ad\uc81c\ud558\uc9c0 \uc54a\uc558\uc5b4."
	})
}
func (a *App) inspectBrowserOutput(parent context.Context, path string) (bool, bool, error) {
	if a.ffmpeg == "" {
		return false, false, errors.New("ffmpeg not configured")
	}
	probe := filepath.Join(filepath.Dir(a.ffmpeg), "ffprobe")
	if runtime.GOOS == "windows" {
		probe += ".exe"
	}
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()
	b, err := exec.CommandContext(ctx, probe, "-v", "error", "-show_entries", "stream=codec_type", "-of", "json", path).Output()
	if err != nil || len(b) > 16384 {
		return false, false, errors.New("media validation failed")
	}
	var out struct {
		Streams []struct {
			CodecType string `json:"codec_type"`
		} `json:"streams"`
	}
	if json.Unmarshal(b, &out) != nil {
		return false, false, errors.New("invalid probe output")
	}
	video, audio := false, false
	for _, s := range out.Streams {
		video = video || s.CodecType == "video"
		audio = audio || s.CodecType == "audio"
	}
	return video, audio, nil
}
