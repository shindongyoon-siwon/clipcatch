package main

import (
 "encoding/json"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "testing"
)

func TestPremiereDefaultsPersistsAndRejectsForeignOrigin(t *testing.T) {
 dir:=t.TempDir();a:=newApp(dir,t.TempDir())
 if a.state.OutputMode!=premiereMode {t.Fatal("missing recommended default")}
 r:=browserRequest(a,"/api/premiere/settings","https://other.example","",`{"mode":"original"}`)
 if r.Code!=403 {t.Fatal("foreign origin allowed")}
 r=browserRequest(a,"/api/premiere/settings","http://127.0.0.1:8765","",`{"mode":"original"}`)
 if r.Code!=200 {t.Fatal(r.Body.String())}
 b:=newApp(dir,t.TempDir());if b.state.OutputMode!=originalMode {t.Fatal("preference not persisted")}
 r=browserRequest(a,"/api/premiere/settings","http://127.0.0.1:8765","",`{"mode":"bad"}`)
 if r.Code!=400 {t.Fatal("bad mode accepted")}
}
func TestPremiereQueueModeSnapshotAndQualityOrder(t *testing.T) {
 a:=newApp(t.TempDir(),t.TempDir());a.state.Phase="ready"
 r:=browserRequest(a,"/api/download","http://127.0.0.1:8765","",`{"urls":"https://www.youtube.com/watch?v=abcdefghijk"}`)
 if r.Code!=200 {t.Fatal(r.Body.String())}
 j:=a.state.Jobs[0];a.state.OutputMode=originalMode
 if j.OutputMode!=premiereMode {t.Fatal("queued mode changed")}
 args:=a.downloadArgs(j)
 if !strings.Contains(strings.Join(args," "),"hasvid,res,fps,hdr:12,lang,+vcodec:avc,+acodec:m4a") {t.Fatal("codec preferred over resolution",args)}
 if strings.Contains(strings.Join(a.downloadArgs(&Job{URL:j.URL,OutputMode:originalMode})," "),"--format-sort-force") {t.Fatal("original mode changed")}
}
func TestPremiereConversionOnlyAcceptsKnownJob(t *testing.T) {
 a:=newApp(t.TempDir(),t.TempDir());a.state.Phase="ready"
 r:=browserRequest(a,"/api/premiere/convert","http://127.0.0.1:8765","",`{"id":"absent","path":"/etc/passwd"}`)
 if r.Code!=400 {t.Fatal("arbitrary path accepted")}
 p:=filepath.Join(t.TempDir(),"existing.mkv");os.WriteFile(p,[]byte("fixture"),0600)
 a.state.Jobs=[]*Job{{ID:"old",File:p,Status:"done",URL:"https://www.youtube.com/watch?v=abcdefghijk"}}
 r=browserRequest(a,"/api/premiere/convert","http://127.0.0.1:8765","",`{"id":"old"}`)
 if r.Code!=202 {t.Fatal(r.Body.String())}
 var out map[string]any;json.Unmarshal(r.Body.Bytes(),&out);j:=a.getJob(out["id"].(string))
 if j.Task!="convert"||j.OriginalFile!=p {t.Fatal("lost original")}
 if downloadJobKey(j)==canonicalJobKey(j.URL) {t.Fatal("conversion overwrote download identity")}
 before:=len(a.state.Jobs);_,e:=a.queueEditFile(p,"")
 if e!=nil||len(a.state.Jobs)!=before {t.Fatal("duplicate local conversion queued")}
}
func TestPremiereConversionHistoryDoesNotPolluteDownloadIndex(t *testing.T) {
 a:=newApp(t.TempDir(),t.TempDir());p:=filepath.Join(t.TempDir(),"ready.mp4");os.WriteFile(p,[]byte("fixture"),0600)
 a.state.Jobs=[]*Job{{ID:"pc1",Task:"convert",OriginalFile:"source.mkv",File:p,Status:"done"}}
 a.saveHistory();if len(a.downloaded)!=0 {t.Fatal("local source path added to download index")}
}
func TestPremiereJobUsesLocalSourceWithoutDownloader(t *testing.T) {
 if os.Getenv("CLIPCATCH_EDIT_MEDIA_TEST")!="1" {t.Skip("media fixture")}
 ff,e:=exec.LookPath("ffmpeg");if e!=nil{t.Fatal(e)}
 dir:=t.TempDir();source:=filepath.Join(dir,"old.mkv")
 out,e:=exec.Command(ff,"-v","error","-f","lavfi","-i","testsrc2=size=160x90:rate=10","-t","0.3","-c:v","libx264","-preset","ultrafast",source).CombinedOutput()
 if e!=nil{t.Fatal(e,string(out))}
 a:=newApp(t.TempDir(),dir);a.ffmpeg=ff;a.state.Phase="ready";a.ytDlp=filepath.Join(dir,"downloader-must-not-run")
 id,e:=a.queueEditFile(source,"");if e!=nil{t.Fatal(e)};a.runDownload(id);job:=a.getJob(id)
 if job.Status!="done"||!job.EditReady||job.OriginalFile!=source||!nonempty(job.File){t.Fatalf("job %#v",job)}
 if !nonempty(source){t.Fatal("original removed")}
 a.saveHistory();b:=newApp(a.dataDir,dir);restored:=b.getJob(id)
 if restored==nil||!restored.EditReady||restored.OriginalFile!=source{t.Fatal("lost conversion history")}
}
