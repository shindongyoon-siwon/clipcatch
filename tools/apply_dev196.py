"""Reviewable source edits based on v1.9.5; never changes production refs or releases."""
from pathlib import Path
R=Path(__file__).resolve().parents[1]
def edit(path, before, after):
 p=R/path;s=p.read_text(encoding='utf-8')
 if after in s:return
 if before not in s:raise RuntimeError('Baseline differs: '+path+' / '+before[:70])
 p.write_text(s.replace(before,after,1),encoding='utf-8')
for fn,val in [('runtimeAppID','clipcatch-dev196'),('runtimeDataName','ClipCatch-Dev196'),('runtimePort','18796')]:
 edit('src/trial_runtime.go',f'func {fn}() string {{',f'func {fn}() string {{\n\tif isolatedDevelopment {{ return "{val}" }}')
edit('src/trial_runtime.go','func runtimeSaveDir(home string) string {','func runtimeSaveDir(home string) string {\n\tif isolatedDevelopment { return filepath.Join(home,"Downloads","ClipCatch-Dev196") }')
edit('src/build_identity.go','return buildIdentity{runtimeAppID(), appChannel, appVersion, stableShareURL}','share := stableShareURL\n\tif isolatedDevelopment { share = "" }\n\treturn buildIdentity{runtimeAppID(), appChannel, appVersion, share}')
edit('src/build_identity.go','func validateBuildIdentity(b buildIdentity) error {','func validateBuildIdentity(b buildIdentity) error {\n\tif isolatedDevelopment {\n\t\tif b.App != "clipcatch-dev196" || b.Channel != "test" || b.ShareURL != "" { return errors.New("isolated development identity mismatch") }\n\t\t_, err := parseTrialVersion(b.Version); return err\n\t}')
edit('src/build_identity.go','func buildLabel() string {','func buildLabel() string {\n\tif isolatedDevelopment { return "DEV196" }')
for header,guard in [
 ('func installedExePath() string {','if isolatedDevelopment { return filepath.Join(os.Getenv("LOCALAPPDATA"),"Programs","ClipCatch-Dev196","ClipCatch-Dev196.exe") }'),
 ('func installAndRelaunch() bool {','if isolatedDevelopment { return false } // No installation or migration.'),
 ('func makeClipCatchShortcuts(target string) {','if isolatedDevelopment { return }'),
 ('func applyClipCatchUpdate(args []string) {','if isolatedDevelopment { return }'),
 ('func launchManualRepair(source, target string) error {','if isolatedDevelopment { return errors.New("development installation disabled") }'),
 ('func (a *App) downloadAndInstall(rel verifiedRelease, target string) {','if isolatedDevelopment { a.setUpdate(UpdateInfo{Status:"current",Detail:"DEV196: automatic installation disabled"}); return }'),
 ('func (a *App) installAppUpdateHandler(w http.ResponseWriter, r *http.Request) {','if isolatedDevelopment { writeError(w,403,"DEV196: automatic installation disabled"); return }')]:edit('src/updater.go',header,header+'\n\t'+guard)
for header,guard in [
 ('func (a *App) beginAppUpdateCheck(automatic, initial bool) (*appUpdateCheck, string) {','if isolatedDevelopment { return nil,"development" }'),
 ('func (a *App) loadRequiredUpdate() {','if isolatedDevelopment { return }'),
 ('func (a *App) watchAppUpdates() {','if isolatedDevelopment { return }')]:edit('src/update_policy.go',header,header+'\n\t'+guard)
edit('src/browser_bridge.go','origin := r.Header.Get("Origin")','if isolatedDevelopment { writeError(w,403,"DEV196: existing browser extensions are not paired with this test build"); return true }\n\torigin := r.Header.Get("Origin")')
edit('src/browser_bridge.go','func (a *App) installBrowserExtension() (string, error) {','func (a *App) installBrowserExtension() (string, error) {\n\tif isolatedDevelopment { return "",errors.New("DEV196: production extension remains unchanged") }')
edit('src/main.go','type App struct {','type App struct {\n\tworkspaceClients map[string]workspaceClient\n\tworkspaceSequence uint64\n\tworkspaceOpening time.Time\n\tqualityUploads int\n')
edit('src/main.go','type ClientState struct {','type ClientState struct {\n\tBuildLabel string `json:"build_label"`\n\tIsolated bool `json:"isolated_development"`\n')
edit('src/main.go','\t\tVersion:         appVersion,','\t\tVersion:         appVersion,\n\t\tBuildLabel: buildLabel(),\n\t\tIsolated: isolatedDevelopment,')
edit('src/main.go','\ta.loadRequiredUpdate()','\tif isolatedDevelopment { a.state.AppUpdate = UpdateInfo{Status:"current",Detail:"DEV196: no automatic updates"}; a.state.DistributionURL = "" }\n\ta.workspaceClients = make(map[string]workspaceClient)\n\ta.loadRequiredUpdate()')
old='''\tmux.HandleFunc("POST /api/stop", func(w http.ResponseWriter, r *http.Request) {
\t\twriteJSON(w, 200, map[string]bool{"ok": true})
\t\tgo func() {
\t\t\ttime.Sleep(250 * time.Millisecond)
\t\t\ta.stopWork()
\t\t\tif a.server != nil {
\t\t\t\t_ = a.server.Close()
\t\t\t}
\t\t}()
\t})'''
edit('src/main.go',old,'\tmux.HandleFunc("POST /api/stop", a.stopHandler)\n\ta.workspaceRoutes(mux)')
edit('src/main.go','<script src=\\"/startup-update.js\\"></script></body>','<script src=\\"/startup-update.js\\"></script><script src=\\"/lifecycle.js\\"></script></body>')
edit('src/main.go','if len(os.Args) == 4 && os.Args[1] == "--clipcatch-apply-update" {','if isolatedDevelopment && len(os.Args)>1 { log.Println("Development installer commands refused"); return }\n\tif len(os.Args) == 4 && os.Args[1] == "--clipcatch-apply-update" {')
edit('src/main.go','openBrowser(baseURL + "/")\n\t\t\treturn','reopenWorkspace(baseURL)\n\t\t\treturn')
edit('src/main.go','if err != nil && isTrialBuild() {','if err != nil && waitForWorkspace(baseURL,3*time.Second) { reopenWorkspace(baseURL); return }\n\tif err != nil && (isTrialBuild() || isolatedDevelopment) {')
edit('src/main.go','showInstallError("\ud14c\uc2a4\ud2b8\ud310 \uc5f0\uacb0 \ud3ec\ud2b8(18766)\uac00 \uc0ac\uc6a9 \uc911\uc785\ub2c8\ub2e4. \uc774\uc804 \ud14c\uc2a4\ud2b8\ud310\uc744 \uc885\ub8cc\ud574 \uc8fc\uc138\uc694. \uc815\uc2dd\ud310\uc740 \ubcc0\uacbd\ud558\uc9c0 \uc54a\uc2b5\ub2c8\ub2e4.")','showInstallError("ClipCatch "+runtimePort()+": address in use. No other installation changed.")')
edit('src/main.go','a.requireStartupCheck = true','a.requireStartupCheck = !isolatedDevelopment')
edit('src/main.go','go func() { time.Sleep(300 * time.Millisecond); openBrowser(addr) }()','go func() { if waitForWorkspace(strings.TrimSuffix(addr,"/"),10*time.Second) { reopenWorkspace(strings.TrimSuffix(addr,"/")) } else { log.Println("Workspace readiness timed out") } }()')
edit('src/main.go',"img-src 'self' data:;","img-src 'self' data: blob:;")
edit('src/main.go','func (a *App) bootstrap() {','func (a *App) bootstrap() {\n\tif isolatedDevelopment && os.Getenv("CLIPCATCH_DEV196_FIXTURE")=="1" {\n\t\ta.ytDlp=os.Getenv("CLIPCATCH_DEV196_YTDLP"); a.ffmpeg=os.Getenv("CLIPCATCH_DEV196_FFMPEG"); a.setSetup("ready","DEV196 fixture tools","",100); return\n\t}')
p=R/'src/web/index.html';s=p.read_text(encoding='utf-8');start=s.find("    $('quitBtn').addEventListener('click'")
if start>=0:
 end=s.index('    function renderJob',start);s=s[:start]+s[end:]
s=s.replace('poll();setInterval(poll,1000);','poll();window.clipcatchPollTimer=setInterval(poll,1000);');p.write_text(s,encoding='utf-8')
edit('src/web/workspace-tabs.js',"identity.textContent=trial?'TEST2 \ud14c\uc2a4\ud2b8\ud310':'';","identity.textContent=trial?(st.build_label||'TEST2 \ud14c\uc2a4\ud2b8\ud310'):'';")
edit('src/quality_media.go','type qualitySource struct {','type qualitySource struct {\n\tOwned bool `json:"-"`\n')
edit('src/quality_media.go','type qualityTask struct {','type qualityTask struct {\n\tReusePreview string `json:"reuse_preview,omitempty"`\n')
edit('src/quality.go','a.qualitySources = make(map[string]qualitySource)','a.qualitySources = make(map[string]qualitySource)\n\ta.cleanQualityInputs()')
edit('src/quality.go','mux.HandleFunc("POST /api/quality/choose", a.chooseQualitySource)','mux.HandleFunc("POST /api/quality/upload", a.uploadQualitySource)\n\tmux.HandleFunc("GET /api/quality/sources", a.listQualitySources)\n\tmux.HandleFunc("POST /api/quality/release", a.releaseQualitySource)\n\tmux.HandleFunc("POST /api/quality/choose", a.chooseQualitySource)')
edit('src/quality.go','if len(a.qualitySources) >= 20 {\n\t\ta.qualitySources = make(map[string]qualitySource)\n\t}','if len(a.qualitySources)>=qualitySourceLimit { a.mu.Unlock(); writeError(w,429,"Remove an unused item first"); return }')
edit('src/quality.go','\t\t\t\tapproved = true','\t\t\t\tapproved = true\n\t\t\t\tif q.Kind=="image" { task.ReusePreview=j.ID }')
old='''\tactive := 0
\tfor _, j := range a.state.Jobs {
\t\tif j.Status == "queued" || j.Status == "downloading" {
\t\t\tactive++
\t\t\tif j.Task == "quality" || j.Task == "quality-engine" {
\t\t\t\twriteError(w, 409, qualityMessage(errors.New("quality-busy")))
\t\t\t\treturn
\t\t\t}
\t\t}
\t}'''
new='''\tactive := 0
\tfor _, j := range a.state.Jobs {
\t\tif j.Quality!=nil && j.Task=="quality" && j.Quality.Preview==task.Preview && qualityFingerprint(*j.Quality)==qualityFingerprint(*task) {
\t\t\tif j.Status=="queued" || j.Status=="downloading" || (!task.Preview && task.ReusePreview!="" && j.Quality.ReusePreview==task.ReusePreview && j.Status=="done" && nonempty(j.File)) { writeJSON(w,202,map[string]string{"id":j.ID}); return }
\t\t}
\t\tif j.Status=="queued" || j.Status=="downloading" {
\t\t\tactive++
\t\t\tif j.Task=="quality-engine" { writeError(w,409,qualityMessage(errors.New("quality-busy"))); return }
\t\t}
\t}'''
edit('src/quality.go',old,new)
edit('src/quality.go','engine := ""\n\tif o.Mode == "ai" {','if q.Kind=="image" && !task.Preview && task.ReusePreview!="" { a.saveQualityImagePreview(ctx,id,task,fail); return }\n\tengine := ""\n\tif o.Mode == "ai" {')
edit('src/update_gate.go','"/api/quality/choose",','"/api/quality/choose", "/api/quality/upload",')
edit('src/queue_controls.go','j.Detail = "\uc0ac\uc6a9\uc790\uac00 \ub2e4\uc6b4\ub85c\ub4dc\ub97c \ucde8\uc18c\ud588\uc5b4."','j.Detail = "\uc0ac\uc6a9\uc790\uac00 \uc791\uc5c5\uc744 \ucde8\uc18c\ud588\uc2b5\ub2c8\ub2e4."')
# Keep current workspace results separate from the bounded recent-history list.
edit('src/queue_controls.go','func (a *App) trimHistoryLocked() {\n\tkept :=','func (a *App) trimHistoryLocked() {\n\tprotected := a.qualityWorkspaceJobsLocked()\n\tkept :=')
edit('src/queue_controls.go','\t\tif finished {\n','\t\tif finished && !protected[job.ID] {\n')
edit('src/quality.go','time.Now().Unix()-j.Quality.Created < 3600 && qualityFingerprint','a.qualityPreviewAvailableLocked(j) && qualityFingerprint')
edit('src/quality.go','\ta.trimHistoryLocked()\n\ta.state.Jobs = append([]*Job{j}, a.state.Jobs...)','\ta.state.Jobs = append([]*Job{j}, a.state.Jobs...)\n\t// Pin the accepted save and its preview before pruning recent history.\n\ta.trimHistoryLocked()')
edit('src/quality.go','time.Now().Unix()-j.Quality.Created > 3600','!a.qualityPreviewAvailable(j)')

print('Applied readable DEV196 changes to development working copy only')
