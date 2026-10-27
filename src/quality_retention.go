package main

import "time"

// qualityWorkspaceJobsLocked returns the bounded, live workspace references.
// Jobs are newest-first. Each selected source keeps its current preview and save
// status, including failures/cancellations so the user can retry them. A save
// also keeps the exact preview it uses. These are NOT ordinary recent history.
// Caller holds a.mu. No user files or generated results are deleted here.
func (a *App) qualityWorkspaceJobsLocked() map[string]bool {
	protected := make(map[string]bool)
	type role struct {
		source  string
		preview bool
	}
	seen := make(map[role]bool)
	for _, j := range a.state.Jobs {
		if j.Task != "quality" || j.Quality == nil {
			continue
		}
		q := j.Quality.Source
		current, exists := a.qualitySources[q.ID]
		if !exists || !qualitySourceStillSame(current, q) {
			continue
		}
		key := role{q.ID, j.Quality.Preview}
		if !seen[key] {
			seen[key] = true
			protected[j.ID] = true
		}
	}
	for _, j := range a.state.Jobs {
		if j.Task != "quality" || j.Quality == nil || j.Quality.ReusePreview == "" {
			continue
		}
		if protected[j.ID] || j.Status == "queued" || j.Status == "downloading" || a.activeRuns[j.ID] != nil {
			protected[j.Quality.ReusePreview] = true
		}
	}
	return protected
}

// A current preview remains available while its source is in the workspace.
// Long batches must not silently expire before the user can save them. Older,
// unreferenced history keeps the previous one-hour lifetime. This is session
// retention, not recovery after program exit: existing startup cleanup remains.
func (a *App) qualityPreviewAvailableLocked(j *Job) bool {
	if j == nil || j.Task != "quality" || j.Quality == nil || !j.Quality.Preview || j.Status != "done" {
		return false
	}
	if time.Now().Unix()-j.Quality.Created < 3600 {
		return true
	}
	q := j.Quality.Source
	current, exists := a.qualitySources[q.ID]
	return exists && qualitySourceStillSame(current, q) && a.qualityWorkspaceJobsLocked()[j.ID]
}

func (a *App) qualityPreviewAvailable(j *Job) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.qualityPreviewAvailableLocked(j)
}
