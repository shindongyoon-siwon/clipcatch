//go:build windows

package main

import (
 "os"
 "path/filepath"
 "syscall"
 "testing"
)
func TestFolderWindowsUsesVisibleUnicodeShellRequest(t *testing.T){
 path:=`C:\Users\동윤\Downloads\ClipCatch & 작업 (1)`
 verb,file,err:=folderWindowsArgs(path)
 if err!=nil||syscall.UTF16ToString(verb)!="open"||syscall.UTF16ToString(file)!=path||folderShowNormal!=1{t.Fatal("not a visible exact directory request",err)}
 for _,n:=range []uintptr{0,2,3,5,31,32}{if folderShellResult(n)==nil{t.Fatal("shell failure hidden",n)}}
 if folderShellResult(33)!=nil{t.Fatal("successful shell request rejected")}
}
func TestFolderNativeWindowsDispatch(t *testing.T){
 if os.Getenv("CLIPCATCH_FOLDER_NATIVE_TEST")!="1"{t.Skip("explicit native shell test")}
 path:=filepath.Join(t.TempDir(),"ClipCatch 한글 폴더 (1)");if err:=os.MkdirAll(path,0700);err!=nil{t.Fatal(err)}
 if err:=openDirectory(path);err!=nil{t.Fatal(err)}
 t.Log("Native Unicode directory request accepted; foreground focus is not asserted")
}
