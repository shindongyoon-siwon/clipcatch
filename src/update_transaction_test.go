package main

import (
 "encoding/json"
 "errors"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "sync/atomic"
 "testing"
)
func updateTestFile(t *testing.T,path,body string){t.Helper();if e:=os.WriteFile(path,[]byte(body),0600);e!=nil{t.Fatal(e)}}
func updateTestContents(t *testing.T,path,want string){t.Helper();b,e:=os.ReadFile(path);if e!=nil||string(b)!=want{t.Fatalf("%s: %q %v",path,b,e)}}
func TestUpdateStageFailurePreservesOriginal(t *testing.T){
 dir:=t.TempDir();target:=filepath.Join(dir,"ClipCatch.exe");updateTestFile(t,target,"working original")
 if _,e:=stageUpdateFile(filepath.Join(dir,"missing"),target);e==nil{t.Fatal("missing source accepted")}
 updateTestContents(t,target,"working original")
}
func TestUpdateUniqueBackupAndRollback(t *testing.T){
 dir:=t.TempDir();target:=filepath.Join(dir,"ClipCatch.exe");source:=filepath.Join(dir,"new.exe")
 updateTestFile(t,target,"working original");updateTestFile(t,target+".backup","older backup");updateTestFile(t,source,"MZnew version")
 staged,e:=stageUpdateFile(source,target);if e!=nil{t.Fatal(e)}
 if !installedBinaryMatches(staged,source){t.Fatal("copy changed")}
 swap,e:=startUpdateSwap(staged,target,"unique");if e!=nil{t.Fatal(e)}
 updateTestContents(t,target,"MZnew version");updateTestContents(t,target+".backup","older backup");updateTestContents(t,swap.Backup,"working original")
 if e=swap.rollback();e!=nil{t.Fatal(e)};updateTestContents(t,target,"working original");updateTestContents(t,target+".backup","older backup")
}
func TestUpdateRollbackNeverOverwritesUnrelatedChange(t *testing.T){
 dir:=t.TempDir();target:=filepath.Join(dir,"ClipCatch.exe");source:=filepath.Join(dir,"new.exe")
 updateTestFile(t,target,"old");updateTestFile(t,source,"new");staged,e:=stageUpdateFile(source,target);if e!=nil{t.Fatal(e)}
 swap,e:=startUpdateSwap(staged,target,"nonce");if e!=nil{t.Fatal(e)}
 updateTestFile(t,target,"unexpected external version")
 if e=swap.rollback();e==nil{t.Fatal("foreign file overwritten")};updateTestContents(t,target,"unexpected external version");updateTestContents(t,swap.Backup,"old")
}
func TestUpdateUniqueBackupCollisionLeavesOriginal(t *testing.T){
 dir:=t.TempDir();target:=filepath.Join(dir,"ClipCatch.exe");source:=filepath.Join(dir,"new.exe")
 updateTestFile(t,target,"old");updateTestFile(t,source,"new");updateTestFile(t,target+".backup-collision","must keep")
 if _,e:=startUpdateSwap(source,target,"collision");e==nil{t.Fatal("backup collision ignored")};updateTestContents(t,target,"old");updateTestContents(t,target+".backup-collision","must keep")
}
func testUpdatePeer(t *testing.T,active bool,identity string,stopStatus int,stops *atomic.Int32)*httptest.Server{
 t.Helper();s:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  switch r.URL.Path{
  case "/api/health":json.NewEncoder(w).Encode(map[string]string{"app":identity,"channel":appChannel})
  case "/api/state":state:=ClientState{};if active{state.Jobs=[]*Job{{Status:"downloading"}}};json.NewEncoder(w).Encode(state)
  case "/api/stop":if r.Method!="POST"||r.Header.Get("Origin")==""{t.Error("invalid shutdown request")};stops.Add(1);w.WriteHeader(stopStatus)
  default:w.WriteHeader(404)
  }
 }));t.Cleanup(s.Close);return s
}
func TestUpdateChecksAllPeersBeforeStoppingAny(t *testing.T){
 var stops atomic.Int32;a:=testUpdatePeer(t,false,runtimeAppID(),200,&stops);b:=testUpdatePeer(t,true,runtimeAppID(),200,&stops)
 if e:=stopUpdatePeers([]updatePeer{{Base:a.URL},{Base:b.URL}});e==nil{t.Fatal("active work not protected")}
 if stops.Load()!=0{t.Fatal("a process stopped before checking all work")}
}
func TestUpdateRefusesOtherChannelAndShutdownFailure(t *testing.T){
 var stops atomic.Int32;s:=testUpdatePeer(t,false,"clipcatch-other",200,&stops)
 if e:=stopUpdatePeers([]updatePeer{{Base:s.URL}});e==nil{t.Fatal("foreign process accepted")}
 if stops.Load()!=0{t.Fatal("foreign channel stopped")}
 s2:=testUpdatePeer(t,false,runtimeAppID(),409,&stops)
 if e:=stopUpdatePeers([]updatePeer{{Base:s2.URL}});e==nil||!strings.Contains(e.Error(),"409"){t.Fatal("refused shutdown treated as success",e)}
}
func TestUpdateStageNoOriginalDeletionBeforeStaging(t *testing.T){
 dir:=t.TempDir();source:=filepath.Join(dir,"new.exe");updateTestFile(t,source,"MZnew")
 _,e:=stageUpdateFile(source,filepath.Join(dir,"absent","ClipCatch.exe"));if e==nil||!errors.Is(e,os.ErrNotExist){t.Fatal(e)}
 updateTestContents(t,source,"MZnew")
}
