package main

import (
 "errors"
 "encoding/json"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "testing"
)
func TestFolderRequestPreservesUnicodeAndSpaces(t *testing.T){
 path:=filepath.Join(t.TempDir(),"한글 폴더 & images (1)");calls:=0
 w:=httptest.NewRecorder();r:=httptest.NewRequest("POST","/api/open-folder",nil)
 serveOpenSaveFolder(w,r,path,func(got string)error{calls++;if got!=path{t.Fatalf("path changed: %q",got)};return nil})
 var body map[string]any;_ = json.Unmarshal(w.Body.Bytes(),&body)
 if w.Code!=200||calls!=1||body["requested"]!=true||body["path"]!=path{t.Fatal(w.Code,w.Body.String(),calls)}
}
func TestFolderFailureIsVisibleAndNeverSuccess(t *testing.T){
 path:=t.TempDir();w:=httptest.NewRecorder()
 serveOpenSaveFolder(w,httptest.NewRequest("POST","/api/open-folder",nil),path,func(string)error{return errors.New("shell unavailable")})
 var body map[string]any;_ = json.Unmarshal(w.Body.Bytes(),&body)
 if w.Code!=500||body["ok"]!=false||body["path"]!=path||!strings.Contains(body["error"].(string),"shell unavailable"){t.Fatal(w.Code,w.Body.String())}
}
func TestFolderRejectsFileAndInvalidPaths(t *testing.T){
 path:=filepath.Join(t.TempDir(),"not-folder");_ = os.WriteFile(path,[]byte("preserve"),0600)
 for _,p:=range []string{path,"", "bad\x00path"}{if _,err:=checkedFolderPath(p);err==nil{t.Fatal("accepted",p)}}
 calls:=0;w:=httptest.NewRecorder();serveOpenSaveFolder(w,httptest.NewRequest("POST","/api/open-folder",nil),path,func(string)error{calls++;return nil})
 if w.Code!=500||calls!=0{t.Fatal("invalid folder launched")};data,_:=os.ReadFile(path);if string(data)!="preserve"{t.Fatal("file modified")}
}
func TestFolderUIWiredWithoutDuplicateHandler(t *testing.T){
 html,_:=pages.ReadFile("web/index.html");js,_:=pages.ReadFile("web/folder-open.js")
 if strings.Contains(string(html),"$('folderBtn').addEventListener")||!strings.Contains(string(html),"/folder-open.js"){t.Fatal("folder handler not migrated")}
 for _,token:=range []string{"AbortSignal.timeout","data.requested!==true","button.disabled","textContent"}{if !strings.Contains(string(js),token){t.Fatal("missing folder feedback guard",token)}}
}
