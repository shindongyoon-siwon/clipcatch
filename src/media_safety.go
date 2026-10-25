package main

import (
	"bufio"
	"context"
	"errors"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func editRotation(s editStream) (int, error) {
	rotation := 0.0
	for _, data := range s.SideData {
		if data.Type == "Display Matrix" {
			rotation = data.Rotation
		}
	}
	n := int(math.Round(rotation))
	if math.Abs(rotation-float64(n)) > .01 || n%90 != 0 {
		return 0, errors.New("edit-rotation")
	}
	return ((n % 360) + 360) % 360, nil
}
func frameRate(raw string) float64 {
	parts := strings.Split(raw, "/")
	a, _ := strconv.ParseFloat(parts[0], 64)
	if len(parts) == 2 {
		b, _ := strconv.ParseFloat(parts[1], 64)
		if b == 0 {
			return 0
		}
		a /= b
	}
	if math.IsNaN(a) || math.IsInf(a, 0) {
		return 0
	}
	return a
}
func repairFPS(s editStream) (string, error) {
	// Preserve exact NTSC fractions and a plausible original nominal cadence.
	rate := frameRate(s.Rate)
	if rate >= 1 && rate <= 120 {
		return s.Rate, nil
	}
	rate = frameRate(s.FrameRate)
	if rate < 1 || rate > 120 {
		return "", errors.New("edit-rate")
	}
	for _, common := range []string{"24000/1001", "24", "25", "30000/1001", "30", "48", "50", "60000/1001", "60", "120"} {
		if math.Abs(frameRate(common)-rate) < 0.01 {
			return common, nil
		}
	}
	return s.FrameRate, nil
}
func forceEditPlan(p editPlan) (editPlan, error) {
	var err error
	p.FPS, err = repairFPS(p.Video)
	if err != nil {
		return p, err
	}
	p.Force = true
	p.CopyVideo = false
	p.Strategy = "safe"
	p.Width += (p.Width % 2)
	p.Height += (p.Height % 2)
	for i := range p.CopyAudio {
		p.CopyAudio[i] = false
	}
	return p, nil
}
func verifyEditDecode(parent context.Context, ff, file string, process func(*exec.Cmd)) error {
	// Checks real video AND every audio stream, not just the file header.
	ctx, cancel := context.WithTimeout(parent, 2*time.Hour)
	defer cancel()
	cmd := exec.CommandContext(ctx, ff, "-hide_banner", "-nostdin", "-v", "error", "-xerror", "-err_detect", "explode", "-threads", "2", "-protocol_whitelist", "file,pipe", "-i", file, "-map", "0:v:0", "-map", "0:a?", "-sn", "-dn", "-f", "null", "-")
	cmd.Stderr = &cappedBuffer{Limit: 8192}
	if err := cmd.Start(); err != nil {
		return errors.New("edit-tools")
	}
	if process != nil {
		process(cmd)
		defer process(nil)
	}
	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("edit-decode")
	}
	return nil
}
func verifyConstantFrames(parent context.Context, ff, file, fps string) error {
	// Stream packet times: bound memory even for long recordings.
	ctx, cancel := context.WithTimeout(parent, 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, editProbePath(ff), "-v", "error", "-protocol_whitelist", "file,pipe", "-select_streams", "v:0", "-show_entries", "packet=dts_time", "-of", "csv=p=0", file)
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = &cappedBuffer{Limit: 4096}
	if err = cmd.Start(); err != nil {
		return err
	}
	scan := bufio.NewScanner(pipe)
	last := 0.0
	count := 0
	bad := false
	step := 1 / frameRate(fps)
	for scan.Scan() {
		raw := strings.TrimSpace(strings.Split(scan.Text(), ",")[0])
		n, e := strconv.ParseFloat(raw, 64)
		if e != nil {
			continue
		}
		if count > 0 && math.Abs((n-last)-step) > .0001 {
			bad = true
		}
		last = n
		count++
	}
	err = cmd.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil || scan.Err() != nil || count < 1 || bad {
		return errors.New("edit-rate")
	}
	return nil
}
