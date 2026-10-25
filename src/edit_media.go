package main

// Non-destructive local media conversion. No network, shell expansion, or
// source overwrite is allowed here. All process arguments are separate tokens.
import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type editStream struct {
	Profile    string `json:"profile"`
	Rate       string `json:"r_frame_rate"`
	SAR        string `json:"sample_aspect_ratio"`
	Primaries  string `json:"color_primaries"`
	Range      string `json:"color_range"`
	SampleRate string `json:"sample_rate"`
	Channels   int    `json:"channels"`
	FieldOrder string `json:"field_order"`
	SideData   []struct {
		Rotation float64 `json:"rotation"`
		Type     string  `json:"side_data_type"`
	} `json:"side_data_list"`
	Index       int    `json:"index"`
	Codec       string `json:"codec_name"`
	Type        string `json:"codec_type"`
	Pixel       string `json:"pix_fmt"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	Transfer    string `json:"color_transfer"`
	ColorSpace  string `json:"color_space"`
	FrameRate   string `json:"avg_frame_rate"`
	Disposition struct {
		Attached int `json:"attached_pic"`
	} `json:"disposition"`
}
type editMedia struct {
	Streams []editStream `json:"streams"`
	Format  struct {
		Name     string `json:"format_name"`
		Duration string `json:"duration"`
	} `json:"format"`
}
type editPlan struct {
	Force         bool
	FPS           string
	Rotated       bool
	Video         editStream
	Audio         []editStream
	CopyVideo     bool
	CopyAudio     []bool
	Strategy      string
	Width, Height int
}
type editResult struct {
	File, Original, Strategy string
	Width, Height            int
}

// Bound subprocess output rather than retaining arbitrary raw stderr in logs.
type cappedBuffer struct {
	Data  []byte
	Limit int
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	space := b.Limit - len(b.Data)
	if space > 0 {
		if space > n {
			space = n
		}
		b.Data = append(b.Data, p[:space]...)
	}
	return n, nil
}
func editProbePath(ffmpeg string) string {
	name := "ffprobe"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(filepath.Dir(ffmpeg), name)
}
func inspectEditMedia(parent context.Context, ffmpeg, path string) (editMedia, error) {
	var info editMedia
	if ffmpeg == "" {
		return info, errors.New("edit-tools")
	}
	stat, e := os.Stat(path)
	if e != nil || !stat.Mode().IsRegular() || stat.Size() == 0 {
		return info, errors.New("edit-input")
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	out := &cappedBuffer{Limit: 1 << 20}
	errout := &cappedBuffer{Limit: 4096}
	cmd := exec.CommandContext(ctx, editProbePath(ffmpeg), "-v", "error", "-protocol_whitelist", "file,pipe", "-show_entries", "stream=index,codec_type,codec_name,pix_fmt,width,height,color_transfer,color_space,color_primaries,color_range,avg_frame_rate,r_frame_rate,sample_aspect_ratio,field_order,profile,sample_rate,channels:stream_side_data=rotation,side_data_type:stream_disposition=attached_pic:format=format_name,duration", "-of", "json", path)
	cmd.Stdout = out
	cmd.Stderr = errout
	if e = cmd.Run(); e != nil {
		if ctx.Err() != nil {
			return info, ctx.Err()
		}
		return info, errors.New("edit-probe")
	}
	if len(out.Data) >= out.Limit || json.Unmarshal(out.Data, &info) != nil {
		return info, errors.New("edit-probe")
	}
	return info, nil
}
func planEdit(info editMedia) (editPlan, error) {
	p := editPlan{Strategy: "remux"}
	found := false
	for _, s := range info.Streams {
		if s.Type == "video" && s.Disposition.Attached == 0 && found {
			return p, errors.New("edit-multiple-video")
		}
		if s.Type == "video" && s.Disposition.Attached == 0 && !found {
			p.Video = s
			found = true
		}
		if s.Type == "audio" {
			p.Audio = append(p.Audio, s)
			p.CopyAudio = append(p.CopyAudio, s.Codec == "aac" && (s.Profile == "" || s.Profile == "LC"))
		}
	}
	if !found || p.Video.Width <= 0 || p.Video.Height <= 0 {
		return p, errors.New("edit-no-video")
	}
	// Do not silently flatten HDR/Dolby or wide-gamut content to 8-bit SDR.
	if p.Video.Transfer == "smpte2084" || p.Video.Transfer == "arib-std-b67" || strings.HasPrefix(p.Video.ColorSpace, "bt2020") || strings.HasPrefix(p.Video.Primaries, "bt2020") || p.Video.Primaries == "smpte432" || p.Video.Primaries == "smpte431" {
		return p, errors.New("edit-hdr")
	}
	if p.Video.Width > 8192 || p.Video.Height > 8192 || len(p.Audio) > 16 {
		return p, errors.New("edit-limit")
	}
	p.CopyVideo = p.Video.Codec == "h264" && p.Video.Pixel == "yuv420p" && p.Video.Range != "pc" && (p.Video.Profile == "" || p.Video.Profile == "High" || p.Video.Profile == "Main" || p.Video.Profile == "Constrained Baseline" || p.Video.Profile == "Baseline")
	if p.Video.FieldOrder != "" && p.Video.FieldOrder != "unknown" && p.Video.FieldOrder != "progressive" {
		return p, errors.New("edit-interlaced")
	}
	for _, side := range p.Video.SideData {
		if side.Type == "DOVI configuration record" {
			return p, errors.New("edit-hdr")
		}
	}
	rotation, err := editRotation(p.Video)
	if err != nil {
		return p, err
	}
	p.Rotated = rotation != 0
	if p.Rotated {
		p.CopyVideo = false
	}
	p.Width, p.Height = p.Video.Width, p.Video.Height
	if rotation == 90 || rotation == 270 {
		p.Width, p.Height = p.Height, p.Width
	}
	if !p.CopyVideo {
		p.Strategy = "video"
		p.Width += (p.Width % 2)
		p.Height += (p.Height % 2)
	}
	for _, copyAudio := range p.CopyAudio {
		if !copyAudio {
			if p.CopyVideo {
				p.Strategy = "audio"
			} else {
				p.Strategy = "video-audio"
			}
			break
		}
	}
	return p, nil
}
func editIsMP4(info editMedia, path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".mp4") && strings.Contains(","+info.Format.Name+",", ",mp4,")
}
func editFFmpegArgs(source, dest string, p editPlan) []string {
	args := []string{"-hide_banner", "-nostdin", "-loglevel", "error", "-xerror", "-y", "-protocol_whitelist", "file,pipe", "-i", source, "-map", fmt.Sprintf("0:%d", p.Video.Index)}
	for _, s := range p.Audio {
		args = append(args, "-map", fmt.Sprintf("0:%d", s.Index))
	}
	metadata := "0"
	if p.Force {
		metadata = "-1"
	}
	args = append(args, "-map_metadata", metadata, "-map_chapters", "-1", "-sn", "-dn")
	if p.CopyVideo {
		args = append(args, "-c:v", "copy")
	} else {
		threads := runtime.NumCPU()
		if threads > 8 {
			threads = 8
		}
		if threads < 1 {
			threads = 1
		}
		args = append(args, "-c:v", "libx264", "-preset", "veryfast", "-crf", "18", "-pix_fmt", "yuv420p", "-threads", strconv.Itoa(threads), "-fps_mode", "passthrough", "-profile:v", "high", "-tag:v", "avc1", "-metadata:s:v:0", "rotate=0")
		// Padding at most one edge pixel avoids downscaling an odd-sized source.
		filters := []string{fmt.Sprintf("pad=%d:%d:0:0", p.Width, p.Height)}
		if p.Video.Range == "pc" {
			filters = append(filters, "scale=in_range=full:out_range=tv")
		}
		if p.Force {
			filters = append(filters, "fps="+p.FPS)
			args = append(args, "-fps_mode", "cfr", "-video_track_timescale", "120000")
		}
		args = append(args, "-vf", strings.Join(filters, ","))
	}
	for i, copyAudio := range p.CopyAudio {
		stream := strconv.Itoa(i)
		if copyAudio {
			args = append(args, "-c:a:"+stream, "copy")
		} else {
			args = append(args, "-c:a:"+stream, "aac", "-b:a:"+stream, "192k", "-ar:a:"+stream, "48000", "-profile:a:"+stream, "aac_low")
		}
	}
	return append(args, "-avoid_negative_ts", "make_zero", "-movflags", "+faststart", "-progress", "pipe:1", "-nostats", "-f", "mp4", dest)
}
func checkEditResult(before, after editMedia, p editPlan) error {
	if !strings.Contains(","+after.Format.Name+",", ",mp4,") {
		return errors.New("edit-validation")
	}
	var videos []editStream
	var audios []editStream
	for _, s := range after.Streams {
		if s.Type == "video" && s.Disposition.Attached == 0 {
			videos = append(videos, s)
		}
		if s.Type == "audio" {
			audios = append(audios, s)
		}
	}
	if len(videos) != 1 || videos[0].Codec != "h264" || videos[0].Pixel != "yuv420p" || videos[0].Width != p.Width || videos[0].Height != p.Height || len(audios) != len(p.Audio) {
		return errors.New("edit-validation")
	}
	for _, s := range audios {
		if s.Codec != "aac" {
			return errors.New("edit-validation")
		}
	}
	a, _ := strconv.ParseFloat(before.Format.Duration, 64)
	b, _ := strconv.ParseFloat(after.Format.Duration, 64)
	if a > 0 && (b <= 0 || math.Abs(a-b) > 1) {
		return errors.New("edit-duration")
	}
	return nil
}

// Publish only a verified output. O_EXCL prevents overwriting any existing copy.
func publishEditCopy(ctx context.Context, temp, folder, source string) (string, error) {
	stem := strings.TrimSuffix(filepath.Base(source), filepath.Ext(source))
	stem = cut(stem, 100)
	if err := os.MkdirAll(folder, 0755); err != nil {
		return "", errors.New("edit-storage")
	}
	for n := 0; n < 1000; n++ {
		suffix := ""
		if n > 0 {
			suffix = fmt.Sprintf("_%03d", n)
		}
		dest := filepath.Join(folder, stem+"_premiere"+suffix+".mp4")
		dst, e := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if os.IsExist(e) {
			continue
		}
		if e != nil {
			return "", errors.New("edit-storage")
		}
		src, e := os.Open(temp)
		if e != nil {
			dst.Close()
			os.Remove(dest)
			return "", errors.New("edit-storage")
		}
		buffer := make([]byte, 1<<20)
		for e == nil {
			if ctx.Err() != nil {
				e = ctx.Err()
				break
			}
			var read int
			read, e = src.Read(buffer)
			if read > 0 {
				if _, werr := dst.Write(buffer[:read]); werr != nil {
					e = werr
					break
				}
			}
		}
		src.Close()
		syncErr := dst.Sync()
		closeErr := dst.Close()
		if e != io.EOF || syncErr != nil || closeErr != nil {
			os.Remove(dest)
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			return "", errors.New("edit-storage")
		}
		return dest, nil
	}
	return "", errors.New("edit-storage")
}
func makeEditCopy(ctx context.Context, ffmpeg, source, folder, work string, progress func(string, float64), process func(*exec.Cmd), force ...bool) (editResult, error) {
	result := editResult{Original: source}
	emit := func(stage string, pct float64) {
		if progress != nil {
			progress(stage, pct)
		}
	}
	emit("probe", 0)
	before, e := inspectEditMedia(ctx, ffmpeg, source)
	if e != nil {
		return result, e
	}
	plan, e := planEdit(before)
	if e != nil {
		return result, e
	}
	if len(force) > 0 && force[0] {
		plan, e = forceEditPlan(plan)
		if e != nil {
			return result, e
		}
	}
	result.Strategy = plan.Strategy
	result.Width, result.Height = plan.Width, plan.Height
	if plan.Strategy == "remux" && editIsMP4(before, source) {
		emit("decode", 99)
		if e = verifyEditDecode(ctx, ffmpeg, source, process); e != nil {
			return result, e
		}
		result.File = source
		result.Strategy = "already"
		emit("already", 100)
		return result, nil
	}
	if e = os.MkdirAll(work, 0700); e != nil {
		return result, errors.New("edit-storage")
	}
	temp, e := os.CreateTemp(work, "convert-*.part.mp4")
	if e != nil {
		return result, errors.New("edit-storage")
	}
	tempName := temp.Name()
	temp.Close()
	defer os.Remove(tempName)
	duration, _ := strconv.ParseFloat(before.Format.Duration, 64)
	cmd := exec.CommandContext(ctx, ffmpeg, editFFmpegArgs(source, tempName, plan)...)
	out, e := cmd.StdoutPipe()
	if e != nil {
		return result, errors.New("edit-encode")
	}
	errout := &cappedBuffer{Limit: 8192}
	cmd.Stderr = errout
	emit(plan.Strategy, 0)
	if e = cmd.Start(); e != nil {
		return result, errors.New("edit-tools")
	}
	if process != nil {
		process(cmd)
		defer process(nil)
	}
	scan := bufio.NewScanner(out)
	for scan.Scan() {
		line := scan.Text()
		if strings.HasPrefix(line, "out_time_us=") && duration > 0 {
			v, _ := strconv.ParseFloat(strings.TrimPrefix(line, "out_time_us="), 64)
			emit(plan.Strategy, math.Max(0, math.Min(98, v/1e6/duration*98)))
		}
	}
	e = cmd.Wait()
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	if e != nil || scan.Err() != nil {
		return result, errors.New("edit-encode")
	}
	emit("verify", 99)
	after, e := inspectEditMedia(ctx, ffmpeg, tempName)
	if e != nil {
		return result, e
	}
	if e = checkEditResult(before, after, plan); e != nil {
		return result, e
	}
	emit("decode", 99)
	if e = verifyEditDecode(ctx, ffmpeg, tempName, process); e != nil {
		return result, e
	}
	if plan.Force {
		if e = verifyConstantFrames(ctx, ffmpeg, tempName, plan.FPS); e != nil {
			return result, e
		}
	}
	result.File, e = publishEditCopy(ctx, tempName, folder, source)
	return result, e
}
