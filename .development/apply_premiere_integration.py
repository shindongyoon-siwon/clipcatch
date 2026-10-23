"""One-time integration of locally validated changes; deleted before PR review."""
from pathlib import Path
import subprocess
root=Path.cwd()
def change(path,pairs):
    p=root/path;s=p.read_text()
    for before,after in pairs:
        if s.count(before)!=1:
            raise RuntimeError('Ambiguous or missing integration anchor: '+path+' '+repr(before[:60]))
        s=s.replace(before,after,1)
    p.write_text(s)
change('src/main.go',[
 ('appDefaultVersion = "1.7.1"','appDefaultVersion = "1.8.0"'),
 ('type Job struct {','type Job struct {\n OutputMode string `json:"output_mode,omitempty"`\n Task string `json:"task,omitempty"`\n OriginalFile string `json:"original_file,omitempty"`\n EditReady bool `json:"edit_ready,omitempty"`\n Stage string `json:"stage,omitempty"`'),
 ('type ClientState struct {','type ClientState struct {\n OutputMode string `json:"output_mode"`'),
 ('type App struct {','type App struct {\n premiereSelecting bool'),
 ('\ta.initBrowserBridge()','\ta.initPremiere()\n\ta.initBrowserBridge()'),
 ('strings.ReplaceAll(string(b), "__APP_VERSION__", appVersion)','strings.ReplaceAll(strings.Replace(string(b), "</body>", "<script src=\\\"/premiere.js\\\"></script></body>", 1), "__APP_VERSION__", appVersion)'),
 ('\ta.mux = mux','\ta.premiereRoutes(mux)\n\ta.mux = mux'),
 ('ID: id, URL: it.u, Host: it.h, Title:','OutputMode: a.state.OutputMode,\n\t\t\tID: id, URL: it.u, Host: it.h, Title:'),
 ('\tif j.BrowserSource {\n\t\ta.runBrowserJob','\tif j.Task == "convert" || (j.OutputMode == premiereMode && nonempty(j.OriginalFile)) {\n\t\ta.finishPremiere(id, ctx)\n\t\treturn\n\t}\n\tif j.BrowserSource {\n\t\ta.runBrowserJob'),
 ('\ta.mutateJob(id, func(j *Job) {\n\t\tif j.Status == "cancelled" {\n\t\t\treturn\n\t\t}\n\t\tj.Status, j.Percent, j.Speed, j.ETA = "done", 100, "", ""','\tif j.OutputMode == premiereMode {\n\t\ta.finishPremiere(id, ctx)\n\t\treturn\n\t}\n\ta.mutateJob(id, func(j *Job) {\n\t\tif j.Status == "cancelled" {\n\t\t\treturn\n\t\t}\n\t\tj.Status, j.Percent, j.Speed, j.ETA = "done", 100, "", ""'),
 ('\treturn append(args, j.URL)\n}','\tif j.OutputMode == premiereMode {\n\t\t// Resolution and frame rate outrank codec preference: never silently take a smaller AVC stream.\n\t\targs = append(args, "--format-sort-force", "-S", "hasvid,res,fps,hdr:12,lang,+vcodec:avc,+acodec:m4a")\n\t}\n\treturn append(args, j.URL)\n}')])
change('src/browser_bridge.go',[('ID:id,URL:in.PageURL','OutputMode:a.state.OutputMode,ID:id,URL:in.PageURL')])
change('src/browser_download.go',[('\ta.mutateJob(id, func(job *Job) {\n\t\tif job.Status == "cancelled" {\n\t\t\treturn\n\t\t}\n\t\tjob.Status = "done"','\tif j.OutputMode == premiereMode {\n\t\ta.finishPremiere(id, ctx)\n\t\treturn\n\t}\n\ta.mutateJob(id, func(job *Job) {\n\t\tif job.Status == "cancelled" {\n\t\t\treturn\n\t\t}\n\t\tjob.Status = "done"')])
change('src/browser_feed.go',[('func downloadJobKey(j *Job) string {','func downloadJobKey(j *Job) string {\n if j.Task == "convert" {return "local-convert:" + j.OriginalFile}')])
change('src/queue_controls.go',[('if j.Status=="done"&&nonempty(j.File){key:=downloadJobKey(j);','if j.Task!="convert"&&j.Status=="done"&&nonempty(j.File){key:=downloadJobKey(j);')])
change('src/browser_version.go',[('if _, err := parseSemver(manifest.Version); err == nil { appVersion = manifest.Version }','if _, err := parseSemver(manifest.Version); err == nil && compareSemver(manifest.Version, appDefaultVersion) > 0 { appVersion = manifest.Version }')])
change('.github/workflows/browser-bridge-check.yml',[('-X main.appVersion=1.7.2','-X main.appVersion=1.8.0')])
change('VERSION',[('1.7.2\n','1.8.0\n')])
subprocess.run(['gofmt','-w','src/main.go','src/edit_media.go','src/premiere.go','src/edit_media_test.go','src/premiere_test.go'],check=True)
print('Integrated only the approved Premiere-mode source files.')
