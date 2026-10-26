package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestRevision3AspectGeometry(t *testing.T) {
	for _, tc := range []struct {
		sar               string
		w, h, rot, ow, oh int
	}{
		{"16:15", 720, 576, 0, 768, 576}, {"8:9", 720, 480, 0, 640, 480},
		{"4:3", 72, 54, 90, 54, 96}, {"1:1", 1920, 1080, 0, 1920, 1080},
		{"0:1", 1920, 1080, 0, 1920, 1080}, {"1:1", 321, 181, 0, 322, 182},
	} {
		b, _ := json.Marshal(map[string]any{"width": tc.w, "height": tc.h, "sample_aspect_ratio": tc.sar, "side_data_list": []map[string]any{{"rotation": tc.rot, "side_data_type": "Display Matrix"}}})
		var v editStream
		json.Unmarshal(b, &v)
		w, h, _, e := qualityDisplayGeometry(v, true)
		if e != nil || w != tc.ow || h != tc.oh {
			t.Fatalf("%+v got %dx%d %v", tc, w, h, e)
		}
	}
	for _, sar := range []string{"bad", "-1:1", "1:0", "NaN:1", "999999:1"} {
		if _, _, _, e := qualityDisplayGeometry(editStream{Width: 72, Height: 54, SAR: sar}, true); e == nil {
			t.Fatal("invalid aspect accepted", sar)
		}
	}
}
func TestRevision3RealAnamorphicVideo(t *testing.T) {
	ff := testQualityFF(t)
	ctx := context.Background()
	dir := t.TempDir()
	source := filepath.Join(dir, "anamorphic.mp4")
	args := []string{"-v", "error", "-f", "lavfi", "-i", "testsrc2=size=72x54:rate=25", "-f", "lavfi", "-i", "sine=frequency=600:sample_rate=48000", "-t", "0.4", "-vf", "setsar=4/3", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-threads", "1", "-c:a", "aac", source}
	if out, e := exec.Command(ff, args...).CombinedOutput(); e != nil {
		t.Fatal(e, string(out))
	}
	before, _ := qualityFileHash(source)
	q, e := describeQualitySource(ctx, ff, source)
	if e != nil || !q.GeometryAdjusted || q.Width != 96 || q.Height != 54 {
		t.Fatalf("%+v %v", q, e)
	}
	o := qualityOptions{Mode: "resize", Scale: 1, Seconds: .4}
	dest := filepath.Join(dir, "square.mp4")
	if e = renderQualityVideo(ctx, ff, "", q, o, dest, dir, nil); e != nil {
		t.Fatal(e)
	}
	if e = validateQualityOutput(ctx, ff, dest, q, o); e != nil {
		t.Fatal(e)
	}
	info, e := inspectEditMedia(ctx, ff, dest)
	if e != nil {
		t.Fatal(e)
	}
	if info.Streams[0].SAR != "1:1" {
		t.Fatal("not square pixels", info.Streams[0])
	}
	if after, _ := qualityFileHash(source); after != before {
		t.Fatal("original modified")
	}
	rotated := filepath.Join(dir, "rotated.mp4")
	if out, e := exec.Command(ff, "-v", "error", "-display_rotation", "90", "-i", source, "-c", "copy", rotated).CombinedOutput(); e != nil {
		t.Fatal(e, string(out))
	}
	q, e = describeQualitySource(ctx, ff, rotated)
	if e != nil || q.Width != 54 || q.Height != 96 {
		t.Fatalf("rotated %+v %v", q, e)
	}
	dest = filepath.Join(dir, "rotated-square.mp4")
	if e = renderQualityVideo(ctx, ff, "", q, o, dest, dir, nil); e != nil {
		t.Fatal(e)
	}
	if e = validateQualityOutput(ctx, ff, dest, q, o); e != nil {
		t.Fatal(e)
	}
}
func TestRevision3StrongBlendAndPreviewGuard(t *testing.T) {
	q := qualitySource{Kind: "image", Width: 31, Height: 23}
	if e := validateQualityOptions(q, qualityOptions{Mode: "ai", Scale: 1, Strength: 1}); e != nil {
		t.Fatal(e)
	}
	if e := validateQualityOptions(q, qualityOptions{Mode: "ai", Scale: 1, Strength: 1.01}); e == nil {
		t.Fatal("invalid blend accepted")
	}
}
func TestRevision3PendingProgressAndMissingFile(t *testing.T) {
	a := newApp(t.TempDir(), t.TempDir())
	a.state.Phase = "ready"
	token := browserTestPair(t, a)
	payload := `{"page_url":"https://www.youtube.com/watch?v=abcdefghijk","output_mode":"premiere"}`
	first := browserRequest(a, "/bridge/page", testExtensionOrigin, token, payload)
	second := browserRequest(a, "/bridge/page", testExtensionOrigin, token, payload)
	var x, y struct {
		IDs []string `json:"job_ids"`
	}
	json.Unmarshal(first.Body.Bytes(), &x)
	json.Unmarshal(second.Body.Bytes(), &y)
	if len(x.IDs) != 1 || len(y.IDs) != 1 || x.IDs[0] != y.IDs[0] {
		t.Fatal("pending job cannot be tracked", first.Body.String(), second.Body.String())
	}
	id := x.IDs[0]
	a.mutateJob(id, func(j *Job) { j.Status = "done"; j.File = filepath.Join(a.saveDir, "missing.mp4") })
	b, _ := json.Marshal(map[string]any{"ids": []string{id}})
	var result struct {
		Jobs []browserJobView `json:"jobs"`
	}
	w := browserRequest(a, "/bridge/status", testExtensionOrigin, token, string(b))
	json.Unmarshal(w.Body.Bytes(), &result)
	if len(result.Jobs) != 1 || result.Jobs[0].FileCount != 0 {
		t.Fatal("missing file counted as stored", w.Body.String())
	}
	os.WriteFile(a.state.Jobs[0].File, []byte("fixture saved"), 0600)
	w = browserRequest(a, "/bridge/status", testExtensionOrigin, token, string(b))
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.Jobs[0].FileCount != 1 {
		t.Fatal("saved file absent from status")
	}
}
