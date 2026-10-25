package main

import (
	"context"
	"testing"
)

func TestQualityDeadlineReleasesBusyState(t *testing.T) {
	a := newApp(t.TempDir(), t.TempDir())
	a.state.Jobs = []*Job{{ID: "slow", Task: "quality", Status: "downloading"}, {ID: "cancelled", Task: "quality", Status: "cancelled"}, {ID: "done", Task: "quality", Status: "done"}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, id := range []string{"slow", "cancelled", "done"} {
		a.finishQualityDeadline(ctx, id)
	}
	if a.getJob("slow").Status != "error" || a.getJob("slow").ErrorCode != "CC-QUALITY-TIMEOUT" {
		t.Fatal("timed-out work retained the busy state")
	}
	if a.getJob("cancelled").Status != "cancelled" || a.getJob("done").Status != "done" {
		t.Fatal("terminal work status was overwritten")
	}
}
