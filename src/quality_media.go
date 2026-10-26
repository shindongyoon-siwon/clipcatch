package main

// Local, opt-in restoration. A bounded batch is kept on disk; originals are
// read-only. AI is never substituted for a missing engine or called silently.
import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type qualityOptions struct {
	Mode     string  `json:"mode"`
	Scale    int     `json:"scale"`
	Strength float64 `json:"strength"`
	Start    float64 `json:"start"`
	Seconds  float64 `json:"seconds"`
}
type qualitySource struct {
	ID               string  `json:"id"`
	Name             string  `json:"name"`
	Kind             string  `json:"kind"`
	Width            int     `json:"width"`
	Height           int     `json:"height"`
	Duration         float64 `json:"duration"`
	GeometryAdjusted bool    `json:"geometry_adjusted,omitempty"`
	PixelAspect      string  `json:"pixel_aspect,omitempty"`
	Path             string  `json:"-"`
	Size             int64   `json:"-"`
	Stamp            int64   `json:"-"`
}
type qualityTask struct {
	Options qualityOptions `json:"options"`
	Source  qualitySource  `json:"source"`
	Preview bool           `json:"preview"`
	Before  string         `json:"-"`
	Created int64          `json:"created"`
}

func describeQualitySource(ctx context.Context, ff, path string) (qualitySource, error) {
	q := qualitySource{Path: path, Name: filepath.Base(path)}
	st, e := os.Stat(path)
	if e != nil || !st.Mode().IsRegular() || st.Size() == 0 {
		return q, errors.New("quality-input")
	}
	q.Size = st.Size()
	q.Stamp = st.ModTime().UnixNano()
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".webp":
		q.Kind = "image"
		if qualityEmbeddedICC(path, ext) {
			return q, errors.New("quality-image-profile")
		}
	case ".mp4", ".mkv", ".webm", ".mov", ".m4v":
		q.Kind = "video"
	default:
		return q, errors.New("quality-input")
	}
	info, e := inspectEditMedia(ctx, ff, path)
	if e != nil {
		return q, e
	}
	plan, e := planEdit(info)
	if e != nil {
		return q, e
	}
	if q.Kind == "image" {
		if plan.Video.Codec != "png" && plan.Video.Codec != "mjpeg" && plan.Video.Codec != "webp" {
			return q, errors.New("quality-image-format")
		}
		if strings.Contains(plan.Video.Pixel, "16") || strings.Contains(plan.Video.Pixel, "48") || strings.Contains(plan.Video.Pixel, "64") {
			return q, errors.New("quality-image-format")
		}
	}
	q.Width, q.Height, q.GeometryAdjusted, e = qualityDisplayGeometry(plan.Video, q.Kind == "video")
	if e != nil {
		return q, e
	}
	q.PixelAspect = plan.Video.SAR
	q.Duration, _ = strconv.ParseFloat(info.Format.Duration, 64)
	if q.Kind == "video" && (q.Duration <= 0 || math.IsInf(q.Duration, 0) || math.IsNaN(q.Duration)) {
		return q, errors.New("quality-duration")
	}
	id, e := newBrowserSecret()
	if e != nil {
		return q, e
	}
	q.ID = id
	return q, nil
}
func validateQualityOptions(q qualitySource, o qualityOptions) error {
	if o.Mode != "resize" && o.Mode != "ai" {
		return errors.New("quality-options")
	}
	if o.Scale != 1 && o.Scale != 2 && o.Scale != 4 {
		return errors.New("quality-options")
	}
	if math.IsNaN(o.Strength) || math.IsInf(o.Strength, 0) || (o.Mode == "ai" && o.Strength != .35 && o.Strength != .65 && o.Strength != 1) {
		return errors.New("quality-options")
	}
	if o.Mode == "ai" && (q.Width > 2048 || q.Height > 2048 || int64(q.Width)*int64(q.Height) > 3000000) {
		return errors.New("quality-ai-source-size")
	}
	if q.Kind == "video" && o.Scale == 1 && (q.Width%2 != 0 || q.Height%2 != 0) {
		return errors.New("quality-size")
	}
	w, h := q.Width*o.Scale, q.Height*o.Scale
	if w < 1 || h < 1 || w > 4096 || h > 4096 || int64(w)*int64(h) > 9000000 {
		return errors.New("quality-size")
	}
	if q.Kind == "video" {
		if math.IsNaN(o.Start) || math.IsNaN(o.Seconds) || math.IsInf(o.Start, 0) || math.IsInf(o.Seconds, 0) || o.Start < 0 || o.Seconds <= 0 || o.Seconds > 120 || o.Start+o.Seconds > q.Duration+.02 {
			return errors.New("quality-duration")
		}
	}
	return nil
}
func sameQualitySource(q qualitySource) bool {
	st, e := os.Stat(q.Path)
	return e == nil && st.Size() == q.Size && st.ModTime().UnixNano() == q.Stamp
}
func qualityRun(ctx context.Context, bin string, args []string, process func(*exec.Cmd)) error {
	cmd := hiddenCommandContext(ctx, bin, args...)
	cmd.Stderr = &cappedBuffer{Limit: 8192}
	cmd.Stdout = &cappedBuffer{Limit: 4096}
	if e := cmd.Start(); e != nil {
		return errors.New("quality-tools")
	}
	if process != nil {
		process(cmd)
		defer process(nil)
	}
	if e := cmd.Wait(); e != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("quality-process")
	}
	return nil
}
func qualityPNG(ctx context.Context, ff, source, dest string, w, h int) error {
	return qualityRun(ctx, ff, []string{"-v", "error", "-nostdin", "-xerror", "-y", "-protocol_whitelist", "file,pipe", "-i", source, "-vf", fmt.Sprintf("scale=%d:%d:flags=lanczos,setsar=1", w, h), "-frames:v", "1", "-threads", "1", "-update", "1", dest}, nil)
}
func readPNG(path string) (image.Image, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	return png.Decode(f)
}
func blendQuality(base, ai, dest string, amount float64) error {
	a, e := readPNG(base)
	if e != nil {
		return e
	}
	b, e := readPNG(ai)
	if e != nil {
		return e
	}
	if a.Bounds() != b.Bounds() {
		return errors.New("quality-validation")
	}
	dst := image.NewNRGBA(a.Bounds())
	for y := a.Bounds().Min.Y; y < a.Bounds().Max.Y; y++ {
		for x := a.Bounds().Min.X; x < a.Bounds().Max.X; x++ {
			ac := color.NRGBAModel.Convert(a.At(x, y)).(color.NRGBA)
			bc := color.NRGBAModel.Convert(b.At(x, y)).(color.NRGBA)
			mix := func(v, u uint8) uint8 { return uint8(math.Round(float64(v)*(1-amount) + float64(u)*amount)) }
			dst.SetNRGBA(x, y, color.NRGBA{R: mix(ac.R, bc.R), G: mix(ac.G, bc.G), B: mix(ac.B, bc.B), A: ac.A})
		}
	}
	f, e := os.Create(dest)
	if e != nil {
		return e
	}
	e = png.Encode(f, dst)
	ce := f.Close()
	if e != nil {
		return e
	}
	return ce
}
func qualityAI(ctx context.Context, engine, indir, outdir string) error {
	for _, name := range []string{"realesrgan-x4plus.bin", "realesrgan-x4plus.param"} {
		if !nonempty(filepath.Join(filepath.Dir(engine), "models", name)) {
			return errors.New("quality-engine")
		}
	}
	limited, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	e := qualityRun(limited, engine, []string{"-i", indir, "-o", outdir, "-n", "realesrgan-x4plus", "-s", "4", "-t", "128", "-m", filepath.Join(filepath.Dir(engine), "models"), "-j", "1:1:1", "-f", "png"}, nil)
	if e != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("quality-gpu")
	}
	return nil
}
func renderQualityImage(ctx context.Context, ff, engine, source, dest, work string, o qualityOptions, w, h int) error {
	reference := filepath.Join(work, "reference.png")
	if e := qualityPNG(ctx, ff, source, reference, w, h); e != nil {
		return e
	}
	if o.Mode == "resize" {
		return copyQualityFile(reference, dest)
	}
	in := filepath.Join(work, "input")
	out := filepath.Join(work, "output")
	os.MkdirAll(in, 0700)
	os.MkdirAll(out, 0700)
	if e := qualityPNG(ctx, ff, source, filepath.Join(in, "frame.png"), w/o.Scale, h/o.Scale); e != nil {
		return e
	}
	if pngHasTransparency(filepath.Join(in, "frame.png")) {
		return errors.New("quality-alpha")
	}
	if e := qualityAI(ctx, engine, in, out); e != nil {
		return e
	}
	scaled := filepath.Join(work, "scaled.png")
	if e := qualityPNG(ctx, ff, filepath.Join(out, "frame.png"), scaled, w, h); e != nil {
		return e
	}
	return blendQuality(reference, scaled, dest, o.Strength)
}
func copyQualityFile(from, to string) error {
	a, e := os.Open(from)
	if e != nil {
		return e
	}
	defer a.Close()
	b, e := os.Create(to)
	if e != nil {
		return e
	}
	_, e = io.Copy(b, a)
	ce := b.Close()
	if e != nil {
		return e
	}
	return ce
}
func writeRGBPNG(raw []byte, w, h int, path string) error {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for p, i := 0, 0; i < len(raw); p, i = p+4, i+3 {
		img.Pix[p] = raw[i]
		img.Pix[p+1] = raw[i+1]
		img.Pix[p+2] = raw[i+2]
		img.Pix[p+3] = 255
	}
	f, e := os.Create(path)
	if e != nil {
		return e
	}
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	e = enc.Encode(f, img)
	ce := f.Close()
	if e != nil {
		return e
	}
	return ce
}
func renderQualityVideo(ctx context.Context, ff, engine string, q qualitySource, o qualityOptions, dest, work string, progress func(float64)) error {
	info, e := inspectEditMedia(ctx, ff, q.Path)
	if e != nil {
		return e
	}
	p, e := planEdit(info)
	if e != nil {
		return e
	}
	fps, e := repairFPS(p.Video)
	if e != nil {
		return e
	}
	w, h := q.Width*o.Scale, q.Height*o.Scale
	duration := strconv.FormatFloat(o.Seconds, 'f', 6, 64)
	start := strconv.FormatFloat(o.Start, 'f', 6, 64)
	if o.Mode == "resize" {
		args := []string{"-v", "error", "-xerror", "-nostdin", "-y", "-protocol_whitelist", "file,pipe", "-ss", start, "-i", q.Path, "-t", duration, "-map", "0:v:0", "-map", "0:a?", "-vf", fmt.Sprintf("scale=%d:%d:flags=lanczos,setsar=1,fps=%s", w, h, fps), "-c:v", "libx264", "-preset", "veryfast", "-crf", "18", "-pix_fmt", "yuv420p", "-profile:v", "high", "-threads", "4", "-fps_mode", "cfr", "-c:a", "aac", "-profile:a", "aac_low", "-ar", "48000", "-b:a", "192k", "-map_metadata", "-1", "-map_chapters", "-1", "-movflags", "+faststart", "-video_track_timescale", "120000", dest}
		return qualityRun(ctx, ff, args, nil)
	}
	// Decode once, process small batches, and stream PNGs to one encoder. No full
	// video frame dump: at most four native and four AI frames are held on disk.
	decode := hiddenCommandContext(ctx, ff, "-v", "error", "-xerror", "-nostdin", "-threads", "2", "-protocol_whitelist", "file,pipe", "-ss", start, "-i", q.Path, "-t", duration, "-vf", fmt.Sprintf("scale=%d:%d:flags=lanczos,setsar=1,fps=%s", q.Width, q.Height, fps), "-an", "-sn", "-dn", "-pix_fmt", "rgb24", "-f", "rawvideo", "pipe:1")
	reader, e := decode.StdoutPipe()
	if e != nil {
		return e
	}
	decode.Stderr = &cappedBuffer{Limit: 8192}
	args := []string{"-v", "error", "-xerror", "-nostdin", "-y", "-threads", "2", "-f", "image2pipe", "-vcodec", "png", "-framerate", fps, "-i", "pipe:0", "-protocol_whitelist", "file,pipe", "-ss", start, "-i", q.Path, "-map", "0:v:0", "-map", "1:a?", "-t", duration, "-c:v", "libx264", "-preset", "veryfast", "-crf", "18", "-pix_fmt", "yuv420p", "-profile:v", "high", "-threads", "4", "-fps_mode", "cfr", "-c:a", "aac", "-profile:a", "aac_low", "-ar", "48000", "-b:a", "192k", "-map_metadata", "-1", "-map_chapters", "-1", "-movflags", "+faststart", "-video_track_timescale", "120000", dest}
	encode := hiddenCommandContext(ctx, ff, args...)
	writer, e := encode.StdinPipe()
	if e != nil {
		return e
	}
	encode.Stderr = &cappedBuffer{Limit: 8192}
	if e = decode.Start(); e != nil {
		return e
	}
	defer func() {
		if decode.ProcessState == nil {
			decode.Process.Kill()
			decode.Wait()
		}
	}()
	if e = encode.Start(); e != nil {
		return e
	}
	defer func() {
		writer.Close()
		if encode.ProcessState == nil {
			encode.Process.Kill()
			encode.Wait()
		}
	}()
	frame := make([]byte, q.Width*q.Height*3)
	count := 0
	expected := math.Ceil(o.Seconds * frameRate(fps))
	done := false
	for !done {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		in := filepath.Join(work, "batch-in")
		out := filepath.Join(work, "batch-out")
		os.MkdirAll(in, 0700)
		os.MkdirAll(out, 0700)
		n := 0
		for ; n < 4; n++ {
			_, e = io.ReadFull(reader, frame)
			if e == io.EOF {
				done = true
				break
			}
			if e != nil {
				return errors.New("quality-decode")
			}
			if e = writeRGBPNG(frame, q.Width, q.Height, filepath.Join(in, fmt.Sprintf("%03d.png", n))); e != nil {
				return e
			}
		}
		if n == 0 {
			break
		}
		if e = qualityAI(ctx, engine, in, out); e != nil {
			return e
		}
		for i := 0; i < n; i++ {
			name := fmt.Sprintf("%03d.png", i)
			reference := filepath.Join(work, "reference.png")
			scaled := filepath.Join(work, "scaled.png")
			mixed := filepath.Join(work, "mixed.png")
			if e = qualityPNG(ctx, ff, filepath.Join(in, name), reference, w, h); e != nil {
				return e
			}
			if e = qualityPNG(ctx, ff, filepath.Join(out, name), scaled, w, h); e != nil {
				return e
			}
			if e = blendQuality(reference, scaled, mixed, o.Strength); e != nil {
				return e
			}
			f, e := os.Open(mixed)
			if e != nil {
				return e
			}
			_, e = io.Copy(writer, f)
			f.Close()
			if e != nil {
				return errors.New("quality-encode")
			}
			count++
			if progress != nil {
				progress(math.Min(95, float64(count)/expected*95))
			}
		}
		os.RemoveAll(in)
		os.RemoveAll(out)
	}
	if e = decode.Wait(); e != nil {
		return errors.New("quality-decode")
	}
	writer.Close()
	if e = encode.Wait(); e != nil {
		return errors.New("quality-encode")
	}
	if count == 0 {
		return errors.New("quality-empty")
	}
	return nil
}
func validateQualityOutput(ctx context.Context, ff, path string, q qualitySource, o qualityOptions) error {
	info, e := inspectEditMedia(ctx, ff, path)
	if e != nil {
		return e
	}
	if len(info.Streams) == 0 {
		return errors.New("quality-validation")
	}
	var v editStream
	for _, s := range info.Streams {
		if s.Type == "video" && s.Disposition.Attached == 0 {
			v = s
			break
		}
	}
	if v.Width != q.Width*o.Scale || v.Height != q.Height*o.Scale {
		return errors.New("quality-validation")
	}
	if q.Kind == "image" {
		if v.Codec != "png" {
			return errors.New("quality-validation")
		}
		f, e := os.Open(path)
		if e != nil {
			return e
		}
		defer f.Close()
		_, e = png.Decode(f)
		return e
	}
	before, e := inspectEditMedia(ctx, ff, q.Path)
	if e != nil {
		return e
	}
	nAudio := func(m editMedia) int {
		n := 0
		for _, s := range m.Streams {
			if s.Type == "audio" {
				n++
			}
		}
		return n
	}
	if v.Codec != "h264" || v.Pixel != "yuv420p" || (v.SAR != "1:1" && v.SAR != "" && v.SAR != "N/A") || nAudio(before) != nAudio(info) {
		return errors.New("quality-validation")
	}
	for _, s := range info.Streams {
		if s.Type == "audio" && s.Codec != "aac" {
			return errors.New("quality-validation")
		}
	}
	dur, _ := strconv.ParseFloat(info.Format.Duration, 64)
	if math.Abs(dur-o.Seconds) > .25 {
		return errors.New("quality-duration")
	}
	if e = verifyEditDecode(ctx, ff, path, nil); e != nil {
		return e
	}
	return verifyConstantFrames(ctx, ff, path, v.FrameRate)
}
func publishQuality(ctx context.Context, temp, dir, stem, ext string) (string, error) {
	if e := os.MkdirAll(dir, 0700); e != nil {
		return "", e
	}
	stem = cut(filepath.Base(stem), 80)
	for n := 0; n < 1000; n++ {
		suffix := ""
		if n > 0 {
			suffix = fmt.Sprintf("_%03d", n)
		}
		dest := filepath.Join(dir, stem+suffix+ext)
		dst, e := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if os.IsExist(e) {
			continue
		}
		if e != nil {
			return "", e
		}
		src, e := os.Open(temp)
		if e != nil {
			dst.Close()
			os.Remove(dest)
			return "", e
		}
		reader := bufio.NewReaderSize(src, 1<<20)
		buffer := make([]byte, 1<<20)
		for {
			if ctx.Err() != nil {
				e = ctx.Err()
				break
			}
			var n int
			n, e = reader.Read(buffer)
			if n > 0 {
				if _, err := dst.Write(buffer[:n]); err != nil {
					e = err
					break
				}
			}
			if e != nil {
				break
			}
		}
		src.Close()
		syncErr := dst.Sync()
		closeErr := dst.Close()
		if e != io.EOF || syncErr != nil || closeErr != nil {
			os.Remove(dest)
			return "", errors.New("quality-storage")
		}
		return dest, nil
	}
	return "", errors.New("quality-storage")
}
func qualityFingerprint(q qualityTask) string {
	b, _ := json.Marshal(struct {
		Options qualityOptions
		Size    int64
		Stamp   int64
		ID      string
	}{q.Options, q.Source.Size, q.Source.Stamp, q.Source.ID})
	return string(b)
}
func pngHasTransparency(path string) bool {
	im, e := readPNG(path)
	if e != nil {
		return true
	}
	if o, ok := im.(interface{ Opaque() bool }); ok {
		return !o.Opaque()
	}
	return true
}
func qualitySourceStillSame(a, b qualitySource) bool {
	return a.Path == b.Path && a.Size == b.Size && a.Stamp == b.Stamp
}

// Preserve the displayed aspect ratio: coded width * SAR, then apply rotation.
// Round video dimensions to the nearest even number for H.264 4:2:0 (<=1px).
// Merely changing the SAR tag would stretch anamorphic video, so both renderers
// resample to these dimensions before resetting SAR to 1:1.
func qualityDisplayGeometry(v editStream, video bool) (int, int, bool, error) {
	if v.Width <= 0 || v.Height <= 0 {
		return 0, 0, false, errors.New("quality-aspect")
	}
	ratio := 1.0
	sar := strings.TrimSpace(v.SAR)
	if sar != "" && sar != "N/A" && sar != "0:1" {
		parts := strings.Split(sar, ":")
		if len(parts) != 2 {
			return 0, 0, false, errors.New("quality-aspect")
		}
		n, e1 := strconv.ParseUint(parts[0], 10, 32)
		d, e2 := strconv.ParseUint(parts[1], 10, 32)
		if e1 != nil || e2 != nil || n == 0 || d == 0 {
			return 0, 0, false, errors.New("quality-aspect")
		}
		ratio = float64(n) / float64(d)
		if ratio < 1.0/16 || ratio > 16 {
			return 0, 0, false, errors.New("quality-aspect")
		}
	}
	fw, fh := float64(v.Width)*ratio, float64(v.Height)
	if fw > 32768 || fh > 32768 {
		return 0, 0, false, errors.New("quality-size")
	}
	rot, e := editRotation(v)
	if e != nil {
		return 0, 0, false, e
	}
	if rot == 90 || rot == 270 {
		fw, fh = fh, fw
	}
	w, h := int(math.Round(fw)), int(math.Round(fh))
	adjusted := math.Abs(ratio-1) > .0000001
	if video {
		w, h = int(math.Round(fw/2))*2, int(math.Round(fh/2))*2
		if w < 2 {
			w = 2
		}
		if h < 2 {
			h = 2
		}
		adjusted = adjusted || math.Abs(float64(w)-fw) > .01 || math.Abs(float64(h)-fh) > .01
	}
	if w < 1 || h < 1 {
		return 0, 0, false, errors.New("quality-size")
	}
	return w, h, adjusted, nil
}
