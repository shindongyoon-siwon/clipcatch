package main

import (
 "errors"
 "fmt"
 "io"
 "net/http"
 "os"
 "path/filepath"
 "sort"
 "strings"
 "unicode"
)
const qualitySourceLimit=50
const qualityImageBytes int64=32<<20
const qualityVideoBytes int64=512<<20
const qualityInputBudget int64=1024<<20
func cleanQualityName(name string)string{name=filepath.Base(strings.ReplaceAll(name,"\\","/"));name=strings.Map(func(r rune)rune{if unicode.IsControl(r){return -1};return r},name);return cut(strings.TrimSpace(name),160)}
func qualityUploadLimit(name string)int64{switch strings.ToLower(filepath.Ext(name)){case ".png",".jpg",".jpeg",".webp":return qualityImageBytes;case ".mp4",".mkv",".mov",".webm",".m4v":return qualityVideoBytes};return 0}
func (a *App) cleanQualityInputs(){root:=filepath.Join(a.dataDir,"quality-input");entries,_:=os.ReadDir(root);for _,d:=range entries{if d.IsDir()&&strings.HasPrefix(d.Name(),"src_"){_=os.RemoveAll(filepath.Join(root,d.Name()))}}}
func (a *App) removeOwnedQualityInput(q qualitySource){if !q.Owned{return};dir:=filepath.Dir(q.Path);root:=filepath.Join(a.dataDir,"quality-input");if filepath.Dir(dir)==root&&strings.HasPrefix(filepath.Base(dir),"src_"){_=os.RemoveAll(dir)}}
func (a *App) uploadQualitySource(w http.ResponseWriter,r *http.Request){
 a.mu.Lock();if a.state.Phase!="ready"||a.newWorkBlockedLocked(){a.mu.Unlock();writeError(w,409,qualityMessage(errors.New("quality-not-ready")));return}
 if len(a.qualitySources)+a.qualityUploads>=qualitySourceLimit||a.qualityUploads>=2{a.mu.Unlock();writeError(w,429,"Remove an unused item or wait for the current upload");return}
 ff:=a.ffmpeg;a.qualityUploads++;a.mu.Unlock();defer func(){a.mu.Lock();a.qualityUploads--;a.mu.Unlock()}()
 r.Body=http.MaxBytesReader(w,r.Body,qualityVideoBytes+(1<<20));reader,e:=r.MultipartReader();if e!=nil{writeError(w,400,"Select an image or video file");return}
 part,e:=reader.NextPart();if e!=nil{writeError(w,400,"No file received");return};defer part.Close()
 name:=cleanQualityName(part.FileName());limit:=qualityUploadLimit(name)
 if part.FormName()!="file"||name==""||limit==0{writeError(w,415,qualityMessage(errors.New("quality-image-format")));return}
 root:=filepath.Join(a.dataDir,"quality-input");if e=os.MkdirAll(root,0700);e!=nil{writeError(w,500,qualityMessage(errors.New("quality-disk")));return}
 dir,e:=os.MkdirTemp(root,"src_");if e!=nil{writeError(w,500,qualityMessage(errors.New("quality-disk")));return}
 keep:=false;defer func(){if !keep{_=os.RemoveAll(dir)}}()
 path:=filepath.Join(dir,"source"+strings.ToLower(filepath.Ext(name)));file,e:=os.OpenFile(path,os.O_CREATE|os.O_EXCL|os.O_WRONLY,0600);if e!=nil{writeError(w,500,"Could not create local input copy");return}
 n,copyErr:=io.Copy(file,io.LimitReader(part,limit+1));closeErr:=file.Close()
 if n>limit{writeError(w,413,fmt.Sprintf("File exceeds %d MB",limit>>20));return}
 if copyErr!=nil||closeErr!=nil||n==0{writeError(w,400,"Could not read the selected file completely");return}
 if _,e=reader.NextPart();e!=io.EOF{writeError(w,400,"Send one file per upload request");return}
 q,e:=describeQualitySource(r.Context(),ff,path);if e!=nil{writeError(w,400,qualityMessage(e));return};q.Name=name;q.Owned=true
 a.mu.Lock();defer a.mu.Unlock();used:=q.Size;for _,v:=range a.qualitySources{if v.Owned{used+=v.Size}}
 if used>qualityInputBudget||len(a.qualitySources)>=qualitySourceLimit{writeError(w,413,"Input storage limit reached; remove unused items first");return}
 if a.stopping||r.Context().Err()!=nil{writeError(w,409,"Upload cancelled");return}
 a.qualitySources[q.ID]=q;keep=true;writeJSON(w,201,q)
}
func (a *App) listQualitySources(w http.ResponseWriter,r *http.Request){a.mu.Lock();items:=make([]qualitySource,0,len(a.qualitySources));for _,q:=range a.qualitySources{items=append(items,q)};a.mu.Unlock();sort.Slice(items,func(i,j int)bool{return items[i].ID<items[j].ID});writeJSON(w,200,map[string]any{"sources":items,"limit":qualitySourceLimit,"image_limit_mb":qualityImageBytes>>20,"video_limit_mb":qualityVideoBytes>>20})}
func (a *App) releaseQualitySource(w http.ResponseWriter,r *http.Request){
 var in struct{ID string `json:"id"`};if premiereJSON(w,r,&in)!=nil{writeError(w,400,"Invalid request");return}
 a.mu.Lock();q,ok:=a.qualitySources[in.ID];if !ok{a.mu.Unlock();writeJSON(w,200,map[string]bool{"ok":true});return}
 for _,j:=range a.state.Jobs{if j.Quality!=nil&&j.Quality.Source.ID==in.ID&&(j.Status=="queued"||j.Status=="downloading"){a.mu.Unlock();writeError(w,409,"Cancel the active item before removing it");return}}
 delete(a.qualitySources,in.ID);a.mu.Unlock();a.removeOwnedQualityInput(q);writeJSON(w,200,map[string]bool{"ok":true})
}
