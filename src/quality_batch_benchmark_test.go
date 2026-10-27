package main

import (
 "context"
 "encoding/json"
 "os"
 "path/filepath"
 "testing"
 "time"
)

// This diagnostic is deliberately not a performance pass/fail threshold.
// Warm both the model path and driver caches, then compare ABBA order.
func TestBatchWarmAlternatingDiagnostic(t *testing.T){
 if os.Getenv("CLIPCATCH_BATCH_BENCH")!="1"{t.Skip("explicit diagnostic only")}
 engine:=os.Getenv("CLIPCATCH_AI_ENGINE_TEST");if engine==""{t.Fatal("official engine required")}
 a,ms:=batchFixture(t,4)
 independent:=func()time.Duration{
  start:=time.Now()
  for _,m:=range ms{work:=t.TempDir();q:=m.task.Source;o:=m.task.Options
   if e:=renderQualityImage(context.Background(),a.ffmpeg,engine,q.Path,filepath.Join(work,"after.png"),work,o,q.Width*o.Scale,q.Height*o.Scale);e!=nil{t.Fatal(e)}
  };return time.Since(start)
 }
 grouped:=func()time.Duration{
  start:=time.Now();a.prepareQualityImageBatch(ms[0].id,ms[0].task,engine)
  for _,m:=range ms{r,release:=a.takeQualityBatchResult(m.id,m.task);if r==nil||r.err!=nil{t.Fatalf("missing group output %+v",r)}
   work:=t.TempDir();q:=m.task.Source;o:=m.task.Options
   e:=renderQualityImagePrepared(context.Background(),a.ffmpeg,engine,q.Path,filepath.Join(work,"after.png"),work,o,q.Width*o.Scale,q.Height*o.Scale,r.path);release();if e!=nil{t.Fatal(e)}
  };return time.Since(start)
 }
 // Both paths get an unmeasured warm-up; do not discard any measured run.
 independent();grouped()
 values:=[]map[string]any{}
 for _,kind:=range []string{"independent","grouped","grouped","independent"}{
  var d time.Duration;if kind=="grouped"{d=grouped()}else{d=independent()}
  values=append(values,map[string]any{"method":kind,"milliseconds":d.Milliseconds()})
 }
 data,_:=json.Marshal(map[string]any{"protocol":"warm both then ABBA","images":4,"input_width":ms[0].task.Source.Width,"input_height":ms[0].task.Source.Height,"measurements":values,"limitation":"Tiny synthetic images and CI graphics environment, not the user's GPU or production photos"});t.Log(string(data))
}
