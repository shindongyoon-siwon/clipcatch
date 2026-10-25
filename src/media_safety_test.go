package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSafetyProfileAndCFR(t *testing.T) {
	m := sampleEditInfo("h264", "aac")
	m.Streams[0].Profile = "High 10"
	m.Streams[0].Pixel = "yuv420p10le"
	m.Streams[0].Rate = "30000/1001"
	m.Streams[1].Profile = "HE-AAC"
	p, e := planEdit(m)
	if e != nil || p.CopyVideo || p.CopyAudio[0] {
		t.Fatal(p, e)
	}
	p, e = forceEditPlan(p)
	if e != nil || p.FPS != "30000/1001" || !p.Force {
		t.Fatal(p, e)
	}
	args := strings.Join(editFFmpegArgs("a.mkv", "b.mp4", p), " ")
	if !strings.Contains(args, "fps=30000/1001") || !strings.Contains(args, "-fps_mode cfr") {
		t.Fatal(args)
	}
	m.Streams[0].Primaries = "bt2020"
	if _, e = planEdit(m); e == nil {
		t.Fatal("wide gamut silently changed")
	}
}
func TestSafetyRealCFRAndCorrupt(t *testing.T) {
	ff := testQualityFF(t)
	dir := t.TempDir()
	p := qualityFixtureVideo(t, ff, dir)
	out, e := makeEditCopy(context.Background(), ff, p, dir, filepath.Join(dir, "work"), nil, nil, true)
	if e != nil {
		t.Fatal(e)
	}
	if out.File == p || out.Strategy != "safe" {
		t.Fatal(out)
	}
	if e = verifyConstantFrames(context.Background(), ff, out.File, "10"); e != nil {
		t.Fatal(e)
	}
	// Existing H.264/AAC MP4 must be actually decoded, not accepted by header only.
	bytes, e := os.ReadFile(out.File)
	if e != nil {
		t.Fatal(e)
	}
	offset := strings.Index(string(bytes), "mdat")
	if offset < 0 {
		t.Fatal("missing mdat")
	}
	for i := offset + 4; i < len(bytes) && i < offset+800; i++ {
		bytes[i] = 0xff
	}
	broken := filepath.Join(dir, "broken.mp4")
	os.WriteFile(broken, bytes, 0600)
	if _, e = makeEditCopy(context.Background(), ff, broken, dir, filepath.Join(dir, "work2"), nil, nil); e == nil {
		t.Fatal("corrupt MP4 accepted")
	}
}
func TestSafetyRealRotation(t *testing.T) {
	ff := testQualityFF(t)
	dir := t.TempDir()
	p := qualityFixtureVideo(t, ff, dir)
	rot := filepath.Join(dir, "rotated.mp4")
	b, e := exec.Command(ff, "-v", "error", "-display_rotation", "90", "-i", p, "-c", "copy", rot).CombinedOutput()
	if e != nil {
		t.Fatal(e, string(b))
	}
	out, e := makeEditCopy(context.Background(), ff, rot, dir, filepath.Join(dir, "work"), nil, nil, true)
	if e != nil {
		t.Fatal(e)
	}
	info, e := inspectEditMedia(context.Background(), ff, out.File)
	if e != nil {
		t.Fatal(e)
	}
	p2, e := planEdit(info)
	if e != nil || p2.Width != 24 || p2.Height != 32 {
		t.Fatal(p2, e)
	}
}

func TestSafetyRealVFR(t *testing.T) {
	ff := testQualityFF(t)
	dir := t.TempDir()
	source := filepath.Join(dir, "variable.mp4")
	b, e := exec.Command(ff, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=96x64:rate=30", "-f", "lavfi", "-i", "sine=frequency=500:sample_rate=48000", "-t", "0.8", "-vf", "select='not(mod(n,2))+not(mod(n,5))'", "-fps_mode", "vfr", "-c:v", "libx264", "-threads", "1", "-c:a", "aac", source).CombinedOutput()
	if e != nil {
		t.Fatal(e, string(b))
	}
	if e = verifyConstantFrames(context.Background(), ff, source, "30"); e == nil {
		t.Fatal("fixture not variable")
	}
	r, e := makeEditCopy(context.Background(), ff, source, dir, filepath.Join(dir, "work"), nil, nil, true)
	if e != nil {
		t.Fatal(e)
	}
	info, e := inspectEditMedia(context.Background(), ff, r.File)
	if e != nil {
		t.Fatal(e)
	}
	if e = verifyConstantFrames(context.Background(), ff, r.File, info.Streams[0].FrameRate); e != nil {
		t.Fatal(e)
	}
}
func TestSafetyRealTenBitAndMultipleAudio(t *testing.T) {
	ff := testQualityFF(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "tenbit.mkv")
	b, e := exec.Command(ff, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=96x64:rate=30000/1001", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000", "-f", "lavfi", "-i", "sine=frequency=660:sample_rate=48000", "-map", "0:v", "-map", "1:a", "-map", "2:a", "-t", "0.6", "-c:v", "libx264", "-pix_fmt", "yuv420p10le", "-threads", "1", "-c:a", "libopus", p).CombinedOutput()
	if e != nil {
		t.Fatal(e, string(b))
	}
	r, e := makeEditCopy(context.Background(), ff, p, dir, filepath.Join(dir, "work"), nil, nil, true)
	if e != nil {
		t.Fatal(e)
	}
	info, e := inspectEditMedia(context.Background(), ff, r.File)
	if e != nil {
		t.Fatal(e)
	}
	n := 0
	for _, s := range info.Streams {
		if s.Type == "audio" {
			n++
			if s.Codec != "aac" || s.Profile != "LC" || s.SampleRate != "48000" {
				t.Fatal(s)
			}
		} else if s.Type == "video" && s.Pixel != "yuv420p" {
			t.Fatal(s)
		}
	}
	if n != 2 {
		t.Fatal("audio tracks dropped", n)
	}
}
