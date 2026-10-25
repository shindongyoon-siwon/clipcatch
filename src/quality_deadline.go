package main

import "context"

// A subprocess deadline is a failure, not a user cancellation. Always release
// the visible busy state even when a child process returns through an early exit.
func (a *App) finishQualityDeadline(ctx context.Context, id string) {
	if ctx.Err() == nil {
		return
	}
	a.mutateJob(id, func(j *Job) {
		if j.Status != "queued" && j.Status != "downloading" {
			return
		}
		j.Status = "error"
		j.ErrorCode = "CC-QUALITY-TIMEOUT"
		j.Detail = "처리 시간이 너무 길어 중단했어. 원본은 유지돼. 더 짧은 구간이나 비AI 확대를 선택해 줘."
	})
}
