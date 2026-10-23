package main

import (
 "context"
 "crypto/sha256"
 "encoding/json"
 "os"
 "os/exec"
 "path/filepath"
 "reflect"
 "strings"
 "testing"
)
func sampleEditInfo(v,a string) editMedia {
 m:=editMedia{Streams:[]editStream{{Index:0,Type:"video",Codec:v,Pixel:"yuv420p",Width:1920,Height:1080}}}
 if a!="" {m.Streams=append(m.Streams,editStream{Index:1,Type:"audio",Codec:a})};return m
}
func TestEditPlanCopiesCompatibleStreams(t *testing.T) {
 for _,tc:=range []struct{v,a,plan string;copyV,copyA bool}{{"h264","aac","remux",true,true},{"h264","opus","audio",true,false},{"vp9","aac","video",false,true},{"av1","opus","video-audio",false,false},{"vp9","","video",false,false}} {
  p,e:=planEdit(sampleEditInfo(tc.v,tc.a))
  if e!=nil||p.Strategy!=tc.plan||p.CopyVideo!=tc.copyV {t.Fatalf("%+v: %+v %v",tc,p,e)}
  if tc.a!=""&&p.CopyAudio[0]!=tc.copyA {t.Fatal(tc,p)}
 }
}
func TestEditPlanPreservesGeometryAndRejectsHDR(t *testing.T) {
 m:=sampleEditInfo("vp9","opus");m.Streams[0].Width=1919;m.Streams[0].Height=1079
 p,e:=planEdit(m);if e!=nil||p.Width!=1920||p.Height!=1080 {t.Fatal(p,e)}
 m.Streams[0].Transfer="smpte2084";if _,e=planEdit(m);e==nil {t.Fatal("HDR flattened silently")}
 m.Streams[0].Transfer="arib-std-b67";if _,e=planEdit(m);e==nil {t.Fatal("HLG flattened silently")}
 m=sampleEditInfo("h264","aac");m.Streams=append(m.Streams,m.Streams[0]);if _,e=planEdit(m);e==nil {t.Fatal("extra video silently discarded")}
}
func TestEditArgsNoNetworkOrShellAndNoVideoReencodeForRemux(t *testing.T) {
 p,_:=planEdit(sampleEditInfo("h264","aac"));args:=editFFmpegArgs("input; $test.mkv","out.mp4",p);s:=strings.Join(args," ")
 if strings.Contains(s,"libx264")||!strings.Contains(s,"-c:v copy")||!strings.Contains(s,"-c:a:0 copy")||!strings.Contains(s,"-protocol_whitelist file,pipe") {t.Fatal(args)}
 if !reflect.DeepEqual(args[6:8],[]string{"-protocol_whitelist","file,pipe"}) {t.Fatal("expected fixed args",args)}
}
func TestEditPublishNeverOverwritesAndCancels(t *testing.T) {
 dir:=t.TempDir();tmp:=filepath.Join(dir,"temp.mp4");os.WriteFile(tmp,[]byte("verified"),0600)
 a,e:=publishEditCopy(context.Background(),tmp,dir,"source.mkv");if e!=nil {t.Fatal(e)}
 b,e:=publishEditCopy(context.Background(),tmp,dir,"source.mkv");if e!=nil||a==b {t.Fatal(a,b,e)}
 bytes,_:=os.ReadFile(a);if string(bytes)!="verified" {t.Fatal("existing output changed")}
 ctx,cancel:=context.WithCancel(context.Background());cancel();_,e=publishEditCopy(ctx,tmp,dir,"cancelled.mkv")
 if e==nil {t.Fatal("cancelled publish succeeded")}
 if _,e=os.Stat(filepath.Join(dir,"cancelled_premiere.mp4"));!os.IsNotExist(e) {t.Fatal("partial output retained")}
}
func TestEditRealMedia(t *testing.T) {
 if os.Getenv("CLIPCATCH_EDIT_MEDIA_TEST")!="1" {t.Skip("opt-in FFmpeg media fixtures")}
 ff,e:=exec.LookPath("ffmpeg");if e!=nil {t.Fatal(e)}
 cases:=[]struct{name,v,a,ext,want string}{{"remux","libx264","aac","mkv","remux"},{"audio_only","libx264","libopus","mkv","audio"},{"vp9_opus","libvpx-vp9","libopus","mkv","video-audio"},{"av1_opus","libaom-av1","libopus","mkv","video-audio"},{"silent","libvpx-vp9","","webm","video"},{"already","libx264","aac","mp4","already"}}
 for _,tc:=range cases {t.Run(tc.name,func(t *testing.T){
  dir:=t.TempDir();in:=filepath.Join(dir,tc.name+"."+tc.ext)
  args:=[]string{"-hide_banner","-nostdin","-loglevel","error","-f","lavfi","-i","testsrc2=size=320x180:rate=25"}
  if tc.a!="" {args=append(args,"-f","lavfi","-i","sine=frequency=440:sample_rate=48000")}
  args=append(args,"-t","0.6","-c:v",tc.v,"-threads","2")
  switch tc.v {case "libx264":args=append(args,"-preset","ultrafast");case "libaom-av1":args=append(args,"-cpu-used","8");case "libvpx-vp9":args=append(args,"-deadline","realtime","-cpu-used","8")}
  if tc.a!="" {args=append(args,"-c:a",tc.a)};args=append(args,in)
  if b,e:=exec.Command(ff,args...).CombinedOutput();e!=nil {t.Fatal(e,string(b))}
  original,_:=os.ReadFile(in);hash:=sha256.Sum256(original)
  out,e:=makeEditCopy(context.Background(),ff,in,dir,filepath.Join(dir,"work"),nil,nil);if e!=nil {t.Fatal(e)}
  if out.Strategy!=tc.want {t.Fatalf("strategy %s not %s",out.Strategy,tc.want)}
  after,_:=os.ReadFile(in);if sha256.Sum256(after)!=hash {t.Fatal("source modified")}
  info,e:=inspectEditMedia(context.Background(),ff,out.File);if e!=nil {t.Fatal(e)};if !editIsMP4(info,out.File) {t.Fatal("not MP4")}
  if b,e:=exec.Command(ff,"-v","error","-xerror","-i",out.File,"-f","null","-").CombinedOutput();e!=nil {t.Fatal(e,string(b))}
  if tc.want=="remux" {for _,spec:=range []string{"0:v:0","0:a:0"} {
   h:=func(path string)[]byte{b,e:=exec.Command(ff,"-v","error","-i",path,"-map",spec,"-c","copy","-f","hash","-hash","sha256","-").Output();if e!=nil{t.Fatal(e)};return b}
   if string(h(in))!=string(h(out.File)) {t.Fatal("remux stream payload changed",spec)}
  }}
 })}
 t.Run("malformed",func(t *testing.T){dir:=t.TempDir();p:=filepath.Join(dir,"broken.mkv");os.WriteFile(p,[]byte("not video"),0600);if _,e:=makeEditCopy(context.Background(),ff,p,dir,filepath.Join(dir,"work"),nil,nil);e==nil {t.Fatal("invalid input accepted")}})
}
func TestEditInputSamples(t *testing.T) {
 raw:=os.Getenv("CLIPCATCH_EDIT_SAMPLE_PATHS");if raw=="" {t.Skip("private user samples never in repository")}
 var paths []string;if json.Unmarshal([]byte(raw),&paths)!=nil {t.Fatal("bad sample paths")};ff,_:=exec.LookPath("ffmpeg")
 for _,path:=range paths {t.Run(filepath.Base(path),func(t *testing.T){
  bytes,e:=os.ReadFile(path);if e!=nil{t.Fatal(e)};hash:=sha256.Sum256(bytes);dir:=t.TempDir()
  out,e:=makeEditCopy(context.Background(),ff,path,dir,filepath.Join(dir,"work"),nil,nil);if e!=nil{t.Fatal(e)}
  after,_:=os.ReadFile(path);if sha256.Sum256(after)!=hash{t.Fatal("sample source changed")}
  if b,e:=exec.Command(ff,"-v","error","-xerror","-i",out.File,"-f","null","-").CombinedOutput();e!=nil{t.Fatal(e,string(b))}
 })}
}
func TestEditCancelStopsEncoderWithoutSourceChanges(t *testing.T) {
 if os.Getenv("CLIPCATCH_EDIT_MEDIA_TEST")!="1" {t.Skip("media fixture")}
 ff,e:=exec.LookPath("ffmpeg");if e!=nil{t.Fatal(e)};dir:=t.TempDir();source:=filepath.Join(dir,"cancel.webm")
 b,e:=exec.Command(ff,"-v","error","-f","lavfi","-i","testsrc2=size=640x360:rate=30","-t","2","-c:v","libvpx-vp9","-deadline","realtime","-cpu-used","8",source).CombinedOutput()
 if e!=nil{t.Fatal(e,string(b))};before,_:=os.ReadFile(source);hash:=sha256.Sum256(before)
 ctx,cancel:=context.WithCancel(context.Background());defer cancel()
 _,e=makeEditCopy(ctx,ff,source,dir,filepath.Join(dir,"work"),nil,func(cmd *exec.Cmd){if cmd!=nil{cancel()}})
 if e==nil||ctx.Err()==nil{t.Fatal("cancelled process succeeded")}
 after,_:=os.ReadFile(source);if sha256.Sum256(after)!=hash{t.Fatal("source changed on cancel")}
 temps,_:=filepath.Glob(filepath.Join(dir,"work","*.mp4"));outputs,_:=filepath.Glob(filepath.Join(dir,"*_premiere*.mp4"))
 if len(temps)+len(outputs)!=0{t.Fatal("partial files retained")}
}
