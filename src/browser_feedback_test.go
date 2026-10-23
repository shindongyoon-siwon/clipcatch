package main

import (
	"strings"
	"testing"
)

func TestBrowserOutputFailureAdviceIsSpecific(t *testing.T) {
	a := newApp(t.TempDir(), t.TempDir())
	a.state.Jobs = []*Job{{ID: "failed", Status: "downloading", BrowserSource: true}}
	a.browserJobError("failed", "CC-315", "validation failed")
	if j := a.getJob("failed"); strings.Contains(j.ErrorAdvice, "무음") || j.AuthRequired {
		t.Fatal("generic output failure was mislabeled as silent or authentication")
	}
	a.browserJobError("failed", "CC-313", "missing audio")
	if j := a.getJob("failed"); !strings.Contains(j.ErrorAdvice, "무음") {
		t.Fatal("missing silent-video advice")
	}
}
