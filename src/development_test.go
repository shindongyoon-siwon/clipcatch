//go:build isolated_dev

package main

import("net/http/httptest";"os";"path/filepath";"strings";"testing")
func TestDevelopmentIsolation(t *testing.T){
 c,v:=appChannel,appVersion;appChannel="test";appVersion="1.9.6-test.1";defer func(){appChannel=c;appVersion=v}()
 if runtimeAppID()!="clipcatch-dev196"||runtimePort()!="18796"||runtimeDataName()!="ClipCatch-Dev196"{t.Fatal("identity not isolated")}
 t.Setenv("LOCALAPPDATA",t.TempDir());t.Setenv("CLIPCATCH_SAVEDIR","stable");t.Setenv("CLIPCATCH_TEST2_SAVEDIR","test2")
 if runtimeSaveDir("home")!=filepath.Join("home","Downloads","ClipCatch-Dev196")||!strings.Contains(installedExePath(),"ClipCatch-Dev196"){t.Fatal("paths leaked")}
 if installAndRelaunch(){t.Fatal("installed itself")};if e:=validateBuildIdentity(currentBuildIdentity());e!=nil{t.Fatal(e)}
 appChannel="stable";if validateBuildIdentity(currentBuildIdentity())==nil{t.Fatal("mixed build accepted")};appChannel="test"
 a:=newApp(t.TempDir(),t.TempDir());if a.state.AppUpdate.Status!="current"||a.state.DistributionURL!=""{t.Fatal("production update link leaked")}
 if c,_:=a.beginAppUpdateCheck(true,true);c!=nil{t.Fatal("release lookup allowed")}
 w:=httptest.NewRecorder();a.installAppUpdateHandler(w,httptest.NewRequest("POST","/api/install-app-update",nil));if w.Code!=403{t.Fatal("installation allowed")}
 a.downloadAndInstall(verifiedRelease{Version:"9.9.9"},installedExePath());if _,e:=os.Stat(filepath.Dir(installedExePath()));!os.IsNotExist(e){t.Fatal("install dir touched")}
 if _,e:=a.installBrowserExtension();e==nil{t.Fatal("extension installed")};applyClipCatchUpdate([]string{installedExePath(),appVersion});if _,e:=os.Stat(installedExePath());!os.IsNotExist(e){t.Fatal("installer ran")}
}
