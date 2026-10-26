package main

import (
 "context"
 "errors"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
)

// Only freshly downloaded, receipt-bound intermediates may be removed.
// Existing downloads and user-selected source files are never deleted.
func (a *App) managedDownloadDir(j *Job) bool {
 if j==nil || j.Task!="" || j.OutputMode!=premiereMode || j.DownloadDir=="" { return false }
 root,e:=filepath.Abs(filepath.Join(a.dataDir,"download-work"));if e!=nil{return false}
 dir,e:=filepath.Abs(j.DownloadDir);if e!=nil || filepath.Dir(dir)!=root || !strings.HasPrefix(filepath.Base(dir),"job-"){return false}
 st,e:=os.Lstat(dir);if e!=nil || !st.IsDir() || st.Mode()&os.ModeSymlink!=0{return false}
 actual,e:=filepath.EvalSymlinks(dir);if e!=nil{return false}
 actualRoot,e:=filepath.EvalSymlinks(root);if e!=nil || filepath.Dir(actual)!=actualRoot{return false}
 receipt,e:=os.ReadFile(filepath.Join(dir,".clipcatch-job"));return e==nil && string(receipt)==j.ID
}
func (a *App) downloadRoot(j *Job) string {
 if a.managedDownloadDir(j){return j.DownloadDir};return a.saveDir
}
func (a *App) prepareManagedDownload(j *Job) error {
 if j.Task!="" || j.OutputMode!=premiereMode{return nil}
 if j.DownloadDir!="" {if a.managedDownloadDir(j){return nil};return errors.New("invalid managed directory")}
 root:=filepath.Join(a.dataDir,"download-work")
 if e:=os.MkdirAll(root,0700);e!=nil{return e}
 dir,e:=os.MkdirTemp(root,"job-");if e!=nil{return e}
 if e=os.WriteFile(filepath.Join(dir,".clipcatch-job"),[]byte(j.ID),0600);e!=nil{os.RemoveAll(dir);return e}
 j.DownloadDir=dir
 a.mutateJob(j.ID,func(st *Job){st.DownloadDir=dir})
 return nil
}
func (a *App) makeDownloadEditCopy(j *Job,ctx context.Context,ff,source,folder,work string,progress func(string,float64),process func(*exec.Cmd),force ...bool)(editResult,error){
 if a.managedDownloadDir(j) && !validPostSource(j.DownloadDir,source){return editResult{},errors.New("edit-input")}
 r,e:=makeEditCopy(ctx,ff,source,folder,work,progress,process,force...)
 if e!=nil{return r,e}
 // Already-compatible MP4 is byte-copied into the output folder, not re-encoded.
 if a.managedDownloadDir(j) && r.File==source { r.File,e=publishEditCopy(ctx,source,folder,source) }
 return r,e
}
func (a *App) finishManagedDownload(id string) {
 j:=a.getJob(id)
 if !a.managedDownloadDir(j){return}
 if j.Status!="done" || !j.EditReady || !allSavedPostFiles(j){return}
 files:=completedPostFiles(j);if len(files)==0{return}
 for _,p:=range files {if !strings.EqualFold(filepath.Ext(p),".mp4") || !validPostSource(a.saveDir,p) || validPostSource(j.DownloadDir,p){return}}
 // Format and full decode checks passed before publication. Partial/failed/cancelled jobs retain their sources.
 if e:=os.RemoveAll(j.DownloadDir);e!=nil{
  a.mutateJob(id,func(st *Job){st.Detail+=" - temporary file cleanup pending"});return
 }
 a.mutateJob(id,func(st *Job){
  st.OriginalFile="";st.DownloadDir=""
  for i:=range st.Media{st.Media[i].Original=""}
  st.Detail="MP4 \uc800\uc7a5 \uc644\ub8cc"
 })
}
