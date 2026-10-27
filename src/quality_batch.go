package main

import (
	"context"
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"time"
)

// Bounded groups amortize model startup without parallel inference.
const qualityBatchMaxItems = 4
const qualityBatchMaxPixels int64 = 3000000

type qualityBatchGroup struct {
	dir       string
	remaining int
}
type qualityBatchResult struct {
	group       *qualityBatchGroup
	fingerprint string
	path        string
	err         error
}
type qualityBatchMember struct {
	id   string
	task qualityTask
}
type qualityBatchHooks struct {
	infer func(context.Context, string, string, string) error
}

func batchImagePreview(j *Job) bool {
	return j != nil && j.Task == "quality" && j.Quality != nil && j.Quality.Preview && j.Quality.Source.Kind == "image" && j.Quality.Options.Mode == "ai" && j.Quality.ReusePreview == ""
}
func (a *App) qualityBatchMembers(id string, task qualityTask) []qualityBatchMember {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.stopping {
		return nil
	}
	members := []qualityBatchMember{{id, task}}
	pixels := int64(task.Source.Width) * int64(task.Source.Height)
	// History is newest first; preserve oldest-first quality work.
	for i := len(a.state.Jobs) - 1; i >= 0; i-- {
		j := a.state.Jobs[i]
		if j.ID == id || j.Status != "queued" {
			continue
		}
		if j.Task != "quality" && j.Task != "quality-engine" {
			continue
		}
		// Never jump across video/save/engine installation or a different setting.
		if !batchImagePreview(j) || j.Quality.Options != task.Options || a.qualityBatchResults[j.ID] != nil {
			break
		}
		n := int64(j.Quality.Source.Width) * int64(j.Quality.Source.Height)
		if len(members) >= qualityBatchMaxItems || pixels+n > qualityBatchMaxPixels {
			break
		}
		members = append(members, qualityBatchMember{j.ID, *j.Quality})
		pixels += n
	}
	return members
}
func (a *App) qualityBatchWanted(id string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.stopping {
		return false
	}
	for _, j := range a.state.Jobs {
		if j.ID == id {
			return j.Status == "queued" || j.Status == "downloading"
		}
	}
	return false
}

// A taken entry retains its reference until its reader finishes. Cancellation
// of a sibling must not delete the output currently being consumed.
func (a *App) takeQualityBatchResult(id string, task qualityTask) (*qualityBatchResult, func()) {
	a.mu.Lock()
	r := a.qualityBatchResults[id]
	delete(a.qualityBatchResults, id)
	a.mu.Unlock()
	if r == nil {
		return nil, func() {}
	}
	release := func() {
		a.mu.Lock()
		r.group.remaining--
		remove := r.group.remaining == 0
		a.mu.Unlock()
		if remove {
			_ = os.RemoveAll(r.group.dir)
		}
	}
	if r.fingerprint != qualityFingerprint(task) {
		r.err = errors.New("quality-source-changed")
	}
	return r, release
}
func (a *App) discardQualityBatchResult(id string) {
	a.mu.Lock()
	r := a.qualityBatchResults[id]
	delete(a.qualityBatchResults, id)
	remove := false
	if r != nil {
		r.group.remaining--
		remove = r.group.remaining == 0
	}
	a.mu.Unlock()
	if remove {
		_ = os.RemoveAll(r.group.dir)
	}
}

// The group owns inference, not its first member. One cancellation discards
// only that output; all cancellations or app shutdown terminate the process.
func (a *App) qualityBatchContext(members []qualityBatchMember) (context.Context, func()) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(len(members))*10*time.Minute)
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		tick := time.NewTicker(50 * time.Millisecond)
		defer tick.Stop()
		for {
			wanted := false
			for _, m := range members {
				if a.qualityBatchWanted(m.id) {
					wanted = true
					break
				}
			}
			if !wanted {
				cancel()
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-a.workerStop:
				cancel()
				return
			case <-tick.C:
			}
		}
	}()
	return ctx, func() { cancel(); <-stopped }
}

// Only the single quality worker calls this. No resident AI process: a
// bounded group starts one engine, which exits after that group completes.
func (a *App) prepareQualityImageBatch(id string, task qualityTask, engine string) {
	members := a.qualityBatchMembers(id, task)
	if len(members) < 2 {
		return
	}
	root := filepath.Join(a.dataDir, "quality-batches")
	if os.MkdirAll(root, 0700) != nil {
		return
	}
	work, e := os.MkdirTemp(root, "batch-")
	if e != nil {
		return
	}
	group := &qualityBatchGroup{dir: work}
	defer func() {
		a.mu.Lock()
		empty := group.remaining == 0
		a.mu.Unlock()
		if empty {
			_ = os.RemoveAll(work)
		}
	}()
	input, output := filepath.Join(work, "input"), filepath.Join(work, "output")
	if os.Mkdir(input, 0700) != nil || os.Mkdir(output, 0700) != nil {
		return
	}
	ctx, finish := a.qualityBatchContext(members)
	defer finish()
	results := make(map[string]*qualityBatchResult, len(members))
	prepared := 0
	for index, m := range members {
		if !a.qualityBatchWanted(m.id) {
			continue
		}
		r := &qualityBatchResult{group: group, fingerprint: qualityFingerprint(m.task)}
		results[m.id] = r
		q := m.task.Source
		if !sameQualitySource(q) {
			r.err = errors.New("quality-source-changed")
			continue
		}
		if e = validateQualityOptions(q, m.task.Options); e != nil {
			r.err = e
			continue
		}
		name := fmt.Sprintf("%03d.png", index)
		path := filepath.Join(input, name)
		if e = qualityPNG(ctx, a.ffmpeg, q.Path, path, q.Width, q.Height); e != nil {
			r.err = e
			_ = os.Remove(path)
			continue
		}
		if pngHasTransparency(path) {
			r.err = errors.New("quality-alpha")
			_ = os.Remove(path)
			continue
		}
		if !sameQualitySource(q) {
			r.err = errors.New("quality-source-changed")
			_ = os.Remove(path)
			continue
		}
		if !a.qualityBatchWanted(m.id) {
			delete(results, m.id)
			_ = os.Remove(path)
			continue
		}
		r.path = filepath.Join(output, name)
		prepared++
		a.mutateJob(m.id, func(j *Job) {
			if j.Status == "queued" || j.Status == "downloading" {
				j.Detail = "AI 묶음 처리 준비 중"
			}
		})
	}
	if prepared > 0 && ctx.Err() == nil {
		infer := a.qualityBatchHooks.infer
		if infer == nil {
			infer = func(c context.Context, e, i, o string) error {
				return qualityAIWithin(c, e, i, o, time.Duration(prepared)*10*time.Minute)
			}
		}
		e = infer(ctx, engine, input, output)
		// Validate individual outputs even when the process reports partial failure.
		for _, m := range members {
			r := results[m.id]
			if r == nil || r.err != nil || r.path == "" {
				continue
			}
			if check := validQualityBatchPNG(r.path, m.task.Source); check != nil {
				r.err = check
				if e != nil {
					r.err = e
				}
			}
		}
	} else {
		for _, r := range results {
			if r.err == nil {
				r.err = errors.New("quality-process")
			}
		}
	}
	a.mu.Lock()
	if a.qualityBatchResults == nil {
		a.qualityBatchResults = make(map[string]*qualityBatchResult)
	}
	for _, m := range members {
		r := results[m.id]
		if r == nil {
			continue
		}
		wanted := false
		for _, j := range a.state.Jobs {
			if j.ID == m.id {
				wanted = !a.stopping && (j.Status == "queued" || j.Status == "downloading")
				break
			}
		}
		if wanted {
			a.qualityBatchResults[m.id] = r
			group.remaining++
		}
	}
	a.mu.Unlock()
}
func validQualityBatchPNG(path string, q qualitySource) error {
	f, e := os.Open(path)
	if e != nil {
		return errors.New("quality-validation")
	}
	defer f.Close()
	c, e := png.DecodeConfig(f)
	if e != nil || c.Width != q.Width*4 || c.Height != q.Height*4 {
		return errors.New("quality-validation")
	}
	return nil
}
