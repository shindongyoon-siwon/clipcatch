package main

import "net/http"

// A failed network check is not evidence of a newer release. Only an already
// verified release can gate new work; existing queued/running work is untouched.
func (a *App) updateRequiredLocked() bool {
 return a.latestRelease.BinaryURL!="" && compareRuntimeVersions(a.latestRelease.Version,appVersion)>0
}
func (a *App) newWorkBlockedLocked() bool {
 return updateInProgress(a.state.AppUpdate.Status) || a.updateRequiredLocked()
}
func (a *App) updateGateRequest(w http.ResponseWriter,r *http.Request) bool {
 if r.Method!="POST" {return false}
 switch r.URL.Path {
 case "/api/download","/api/retry-failed","/api/premiere/convert","/api/premiere/choose","/api/quality/choose","/api/quality/start","/api/quality/install","/bridge/page","/bridge/download":
 default:return false
 }
 a.mu.Lock();blocked:=a.newWorkBlockedLocked();latest:=a.latestRelease.Version;a.mu.Unlock()
 if !blocked{return false}
 writeJSON(w,409,map[string]any{"code":"CC-UPDATE-REQUIRED","error":"\uc0c8 \ubc84\uc804 \uc5c5\ub370\uc774\ud2b8 \ud6c4 \uc774\uc6a9\ud574 \uc8fc\uc138\uc694. \uc9c4\ud589 \uc911\uc778 \uc791\uc5c5\uc740 \uacc4\uc18d\ub429\ub2c8\ub2e4.","latest":latest})
 return true
}
