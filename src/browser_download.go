package main

import (
	"context"
	"encoding/json"
	"errors"
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
				job.ErrorTitle = "브라우저 영상 주소를 다시 받아야 해"
				job.ErrorAdvice = "원본 게시물을 열어 재생하고 확장 프로그램에서 다시 보내 줘."
			} else if job.ErrorCode == "CC-104" || job.ErrorCode == "CC-103" {
				job.ErrorCode = "CC-314"
				job.ErrorTitle = "영상 서버가 직접 저장을 거부했어"
				job.ErrorAdvice = "브라우저 재생과 별개로 영상 주소가 만료됐거나 접근이 제한될 수 있어. 다시 감지해도 안 되면 이 경로로는 저장할 수 없어."
			}
			job.Detail = job.ErrorTitle
		})
		return
	}
	got := a.getJob(id)
	if got == nil || !nonempty(got.File) {
		a.browserJobError(id, "CC-315", "저장된 영상 파일을 확인하지 못했어")
		return
	}
	a.mu.Lock()
	media := a.browser.Sources[id]
	a.mu.Unlock()
	ctx := a.jobContext(id)
	video, audio, err := a.inspectBrowserOutput(ctx, got.File)
	if a.isJobCancelled(id) {
		return
	}
	if err != nil || !video {
		a.browserJobError(id, "CC-315", "저장 파일의 영상·음성 검사에 실패했어")
		return
	}
	if !audio && !media.AllowSilent {
		a.browserJobError(id, "CC-313", "저장된 영상에 음성이 없어")
		return
	}
	if j.OutputMode == premiereMode {
		a.finishPremiere(id, ctx)
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
		job.Detail = "브라우저 영상 저장 완료"
		if audio {
			job.Detail += " · 영상·음성 확인"
		} else {
			job.Detail += " · 무음 저장 허용됨"
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
		j.ErrorAdvice = "원본이 무음 영상인지 확인해 줘. 무음이 맞으면 확장의 [무음 저장 허용]을 체크해 다시 보내 줘. 기존 파일은 삭제하지 않았어."
		if code != "CC-313" {
			j.ErrorAdvice = "문제 해결에서 영상·음성 처리 도구와 저장 폴더를 점검해 줘. 확장에서 영상을 다시 찾아 보내 줘. 인증 파일을 다시 등록할 필요는 없어."
		}
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
	b, err := hiddenCommandContext(ctx, probe, "-v", "error", "-show_entries", "stream=codec_type", "-of", "json", path).Output()
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
