package main

import (
	"context"
	"os/exec"
	"sync"
)

const downloadWorkers = 2
const qualityWorkers = 1

type jobExecution struct {
	ctx    context.Context
	cancel context.CancelFunc
	cmd    *exec.Cmd
}

// Task lanes are bounded: two downloads/preparations and one AI worker.
// A long AI job cannot monopolize the download queue. Cancellation belongs to
// one job, never a process/context shared by different workers.
func (a *App) dispatchJobs(execute func(string)) {
	network := make(chan string, 150)
	quality := make(chan string, 150)
	var workers sync.WaitGroup
	consume := func(queue <-chan string) {
		defer workers.Done()
		for {
			select {
			case <-a.workerStop:
				return
			case id, ok := <-queue:
				if !ok {
					return
				}
				select {
				case <-a.workerStop:
					return
				default:
				}
				if !a.isJobCancelled(id) {
					execute(id)
				}
			}
		}
	}
	for i := 0; i < downloadWorkers; i++ {
		workers.Add(1)
		go consume(network)
	}
	for i := 0; i < qualityWorkers; i++ {
		workers.Add(1)
		go consume(quality)
	}
	defer func() { close(network); close(quality); workers.Wait() }()
	for {
		select {
		case <-a.workerStop:
			return
		case id, ok := <-a.queue:
			if !ok {
				return
			}
			job := a.getJob(id)
			if job == nil || job.Status != "queued" {
				continue
			}
			lane := network
			if job.Task == "quality" || job.Task == "quality-engine" {
				lane = quality
			}
			select {
			case lane <- id:
			case <-a.workerStop:
				return
			}
		}
	}
}
func (a *App) jobContext(id string) context.Context {
	a.mu.Lock()
	defer a.mu.Unlock()
	if run := a.activeRuns[id]; run != nil {
		return run.ctx
	}
	// Direct engine fixture tests have no worker context; actual jobs always do.
	return context.Background()
}
func (a *App) setJobProcess(id string, cmd *exec.Cmd) {
	a.mu.Lock()
	run := a.activeRuns[id]
	cancelled := false
	if run != nil {
		run.cmd = cmd
		cancelled = run.ctx.Err() != nil
	}
	a.mu.Unlock()
	if cancelled && cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
func (a *App) clearJobProcess(id string, cmd *exec.Cmd) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if run := a.activeRuns[id]; run != nil && run.cmd == cmd {
		run.cmd = nil
	}
}
func (a *App) stopWork() {
	a.stopOnce.Do(func() {
		a.mu.Lock()
		a.stopping = true
		a.cancelWorkspaceIdleLocked()
		if a.updateCheck.cancel != nil {
			a.updateCheck.cancel()
		}
		cancels := make([]context.CancelFunc, 0, len(a.activeRuns))
		for _, run := range a.activeRuns {
			cancels = append(cancels, run.cancel)
		}
		for _, j := range a.state.Jobs {
			if j.Status == "queued" || j.Status == "downloading" {
				j.Status = "cancelled"
				j.Detail = "프로그램 종료로 중단됨. 원본은 보존돼."
			}
		}
		close(a.workerStop)
		a.mu.Unlock()
		for _, cancel := range cancels {
			cancel()
		}
		a.saveHistory()
	})
}
