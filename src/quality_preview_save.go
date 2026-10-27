package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// Only full-image previews qualify. Video drafts never enter here.
func (a *App) saveQualityImagePreview(ctx context.Context, id string, task qualityTask, fail func(error)) {
	q := task.Source
	p := a.getJob(task.ReusePreview)
	if q.Kind != "image" || task.Preview || p == nil || p.Task != "quality" || p.Quality == nil || !p.Quality.Preview || p.Status != "done" || p.Quality.Source.Kind != "image" || qualityFingerprint(*p.Quality) != qualityFingerprint(task) || !a.qualityPreviewAvailable(p) {
		fail(errors.New("quality-preview"))
		return
	}
	expected := filepath.Join(a.dataDir, "quality-preview", p.ID, "after.png")
	if p.File != expected || !nonempty(expected) || !sameQualitySource(q) {
		fail(errors.New("quality-preview"))
		return
	}
	a.mutateJob(id, func(j *Job) {
		j.Detail = "\ud655\uc778\ud55c \uc774\ubbf8\uc9c0 \uc800\uc7a5 \uc911 (AI \uc7ac\ucc98\ub9ac \uc5c6\uc74c)"
		j.Percent = 90
	})
	if e := validateQualityOutput(ctx, a.ffmpeg, expected, q, task.Options); e != nil {
		fail(e)
		return
	}
	if ctx.Err() != nil || a.isJobCancelled(id) {
		return
	}
	suffix := fmt.Sprintf("_resize%dx", task.Options.Scale)
	if task.Options.Mode == "ai" {
		suffix = fmt.Sprintf("_AI%dx", task.Options.Scale)
	}
	final, e := publishQuality(ctx, expected, a.saveDir, strings.TrimSuffix(q.Name, filepath.Ext(q.Name))+suffix, ".png")
	if e != nil {
		fail(e)
		return
	}
	a.mutateJob(id, func(j *Job) {
		if j.Status == "cancelled" {
			return
		}
		j.Status = "done"
		j.Percent = 100
		j.File = final
		j.Detail = "\ud655\uc778\ud55c \uc774\ubbf8\uc9c0 \uc800\uc7a5 \uc644\ub8cc \u00b7 \uc6d0\ubcf8 \ubcf4\uc874 (AI \uc7ac\ucc98\ub9ac \uc5c6\uc74c)"
	})
}
