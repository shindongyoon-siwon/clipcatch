package main

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "image"
 "image/color"
 "image/png"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"
)
func batchFixture(t *testing.T,n int)(*App,[]qualityBatchMember){
 t.Helper();a:=newApp(t.TempDir(),t.TempDir());a.ffmpeg=testQualityFF(t);a.state.Phase="ready";var members []qualityBatchMember
 for i:=0;i<n;i++{source:=testQualityImage(t,t.TempDir(),false);q,e:=describeQualitySource(context.Background(),a.ffmpeg,source);if e!=nil{t.Fatal(e)}
  task:=qualityTask{Source:q,Options:qualityOptions{Mode:"ai",Scale:2,Strength:.65},Preview:true,Created:time.Now().Unix()};id:=fmt.Sprintf("q_fixture_%03d",i);members=append(members,qualityBatchMember{id,task})
  j:=&Job{ID:id,Task:"quality",Quality:&task,Status:"queued"};if i==0{j.Status="downloading"};a.state.Jobs=append([]*Job{j},a.state.Jobs...)
 };return a,members
}
// Explicit test double. Real engine output equivalence is tested separately.
func batchFakeInfer(ctx context.Context,engine,input,output string)error{
 entries,e:=os.ReadDir(input);if e!=nil{return e}
 for _,entry:=range entries{
  if ctx.Err()!=nil{return ctx.Err()};f,e:=os.Open(filepath.Join(input,entry.Name()));if e!=nil{return e};im,e:=png.Decode(f);f.Close();if e!=nil{return e}
  b:=im.Bounds();up:=image.NewNRGBA(image.Rect(0,0,b.Dx()*4,b.Dy()*4))
  for y:=0;y<up.Bounds().Dy();y++{for x:=0;x<up.Bounds().Dx();x++{up.Set(x,y,im.At(x/4,y/4))}}
  f,e=os.Create(filepath.Join(output,entry.Name()));if e!=nil{return e};e=png.Encode(f,up);f.Close();if e!=nil{return e}
 };return nil
}
func cancelBatchMember(t *testing.T,a *App,id string){
 t.Helper();w:=httptest.NewRecorder();a.cancelDownloadHandler(w,httptest.NewRequest("POST","/api/cancel-download",strings.NewReader(`{"id":"`+id+`"}`)))
 if w.Code!=200{t.Fatal(w.Code,w.Body.String())}
}
func TestBatchOneEngineInvocationAndOutputLifetime(t *testing.T){
 a,ms:=batchFixture(t,4);calls:=0;a.qualityBatchHooks.infer=func(c context.Context,e,i,o string)error{calls++;return batchFakeInfer(c,e,i,o)}
 a.prepareQualityImageBatch(ms[0].id,ms[0].task,"");if calls!=1{t.Fatalf("expected 1 launch for 4 images, got %d",calls)}
 first,release:=a.takeQualityBatchResult(ms[0].id,ms[0].task);if first==nil||first.err!=nil{t.Fatalf("missing result: %+v",first)};root:=first.group.dir
 for _,m:=range ms[1:]{cancelBatchMember(t,a,m.id)}
 if !nonempty(first.path){t.Fatal("sibling cancellation deleted live result")};release()
 if _,e:=os.Stat(root);!os.IsNotExist(e){t.Fatal("temporary batch retained")};if len(a.qualityBatchResults)!=0{t.Fatal("cache retained")}
}
func TestBatchCancelLeaderDoesNotCancelOtherImages(t *testing.T){
 a,ms:=batchFixture(t,3);a.qualityBatchHooks.infer=func(c context.Context,e,i,o string)error{
  cancelBatchMember(t,a,ms[0].id);select{case <-c.Done():t.Fatal("leader owns shared inference");case <-time.After(100*time.Millisecond):};return batchFakeInfer(c,e,i,o)
 }
 a.prepareQualityImageBatch(ms[0].id,ms[0].task,"")
 if r,_:=a.takeQualityBatchResult(ms[0].id,ms[0].task);r!=nil{t.Fatal("cancelled leader retained")}
 for _,m:=range ms[1:]{r,release:=a.takeQualityBatchResult(m.id,m.task);if r==nil||r.err!=nil{t.Fatalf("other image lost %+v",r)};release()}
}
func TestBatchCancelAllStopsInference(t *testing.T){
 a,ms:=batchFixture(t,3);a.qualityBatchHooks.infer=func(c context.Context,e,i,o string)error{
  for _,m:=range ms{cancelBatchMember(t,a,m.id)};select{case <-c.Done():return c.Err();case <-time.After(time.Second):t.Fatal("engine did not receive cancellation")};return nil
 };a.prepareQualityImageBatch(ms[0].id,ms[0].task,"")
 entries,_:=os.ReadDir(filepath.Join(a.dataDir,"quality-batches"));if len(entries)!=0||len(a.qualityBatchResults)!=0{t.Fatal("cancelled batch retained")}
}
func TestBatchPartialOutputAndTransparentInputAreIsolated(t *testing.T){
 a,ms:=batchFixture(t,3);p:=ms[1].task.Source.Path;f,e:=os.Create(p);if e!=nil{t.Fatal(e)}
 im:=image.NewNRGBA(image.Rect(0,0,8,8));im.SetNRGBA(0,0,color.NRGBA{R:255,A:20});png.Encode(f,im);f.Close()
 q,e:=describeQualitySource(context.Background(),a.ffmpeg,p);if e!=nil{t.Fatal(e)};ms[1].task.Source=q;for _,j:=range a.state.Jobs{if j.ID==ms[1].id{j.Quality.Source=q}}
 calls:=0;a.qualityBatchHooks.infer=func(c context.Context,e,i,o string)error{calls++;if err:=batchFakeInfer(c,e,i,o);err!=nil{return err};os.Remove(filepath.Join(o,"002.png"));return errors.New("quality-gpu")}
 a.prepareQualityImageBatch(ms[0].id,ms[0].task,"");if calls!=1{t.Fatal(calls)}
 for i,m:=range ms{r,release:=a.takeQualityBatchResult(m.id,m.task);if r==nil{t.Fatal("missing result",i)};if i==0&&r.err!=nil{t.Fatal("good output discarded",r.err)};if i==1&&(r.err==nil||r.err.Error()!="quality-alpha"){t.Fatal("transparency not isolated",r.err)};if i==2&&r.err==nil{t.Fatal("missing output reported success")};release()}
}
func TestBatchSelectionBoundsAndCancellationContext(t *testing.T){
 a:=newApp(t.TempDir(),t.TempDir());o:=qualityOptions{Mode:"ai",Scale:2,Strength:.65};task:=qualityTask{Source:qualitySource{Kind:"image",Width:1000,Height:1000},Options:o,Preview:true}
 for i:=5;i>=0;i--{v:=task;a.state.Jobs=append(a.state.Jobs,&Job{ID:fmt.Sprint(i),Task:"quality",Quality:&v,Status:"queued"})}
 ms:=a.qualityBatchMembers("0",task);if len(ms)!=3||ms[1].id!="1"||ms[2].id!="2"{t.Fatal("pixel bound/FIFO violated",ms)}
 for _,j:=range a.state.Jobs{j.Quality.Source.Width=10;j.Quality.Source.Height=10};task.Source.Width=10;task.Source.Height=10
 ms=a.qualityBatchMembers("0",task);if len(ms)!=4{t.Fatal("count bound",len(ms))}
 ctx,done:=a.qualityBatchContext(ms);a.stopWork();select{case <-ctx.Done():case <-time.After(time.Second):t.Fatal("stop did not end context")};done()
}
func TestBatchSingleImageHasNoExtraInference(t *testing.T){
 a:=newApp(t.TempDir(),t.TempDir());a.qualityBatchHooks.infer=func(context.Context,string,string,string)error{t.Fatal("single entered batch path");return nil}
 task:=qualityTask{Options:qualityOptions{Mode:"ai",Scale:2,Strength:.65},Source:qualitySource{Kind:"image",Width:16,Height:16},Preview:true}
 a.state.Jobs=[]*Job{{ID:"one",Task:"quality",Quality:&task,Status:"downloading"}};a.prepareQualityImageBatch("one",task,"")
 if _,e:=os.Stat(filepath.Join(a.dataDir,"quality-batches"));!os.IsNotExist(e){t.Fatal("single created batch workspace")}
}
func TestBatchRealEngineEquivalentToIndependentImages(t *testing.T){
 engine:=os.Getenv("CLIPCATCH_AI_ENGINE_TEST");if engine==""{t.Skip("requires official AI engine; synthetic tests are not a speed claim")}
 a,ms:=batchFixture(t,4);start:=time.Now();a.prepareQualityImageBatch(ms[0].id,ms[0].task,engine)
 if len(a.qualityBatchResults)!=4{t.Fatal("no grouped outputs")};var after []string
 for _,m:=range ms{r,release:=a.takeQualityBatchResult(m.id,m.task);if r==nil||r.err!=nil{t.Fatalf("batch failed %+v",r)};work:=t.TempDir();out:=filepath.Join(work,"out.png");q:=m.task.Source;o:=m.task.Options
  e:=renderQualityImagePrepared(context.Background(),a.ffmpeg,engine,q.Path,out,work,o,q.Width*o.Scale,q.Height*o.Scale,r.path);release();if e!=nil{t.Fatal(e)};after=append(after,out)
 };batch:=time.Since(start);start=time.Now()
 for i,m:=range ms{work:=t.TempDir();out:=filepath.Join(work,"out.png");q:=m.task.Source;o:=m.task.Options
  if e:=renderQualityImage(context.Background(),a.ffmpeg,engine,q.Path,out,work,o,q.Width*o.Scale,q.Height*o.Scale);e!=nil{t.Fatal(e)}
  h,_:=qualityFileHash(out);g,_:=qualityFileHash(after[i]);if h!=g{t.Fatalf("output bytes changed at %d",i)}
 };independent:=time.Since(start)
 summary,_:=json.Marshal(map[string]any{"images":4,"input_width":ms[0].task.Source.Width,"input_height":ms[0].task.Source.Height,"grouped_ms":batch.Milliseconds(),"independent_ms":independent.Milliseconds(),"engine_launches_before":4,"engine_launches_after":1,"identical_png_outputs":true,"note":"Synthetic fixture in CI, not a user GPU benchmark; includes IO and normalization"});t.Log(string(summary))
}
func TestBatchFullJobPathReusesFourPreparedImages(t *testing.T){
 a,ms:=batchFixture(t,4)
 // Test-only receipt fixture. Dummy engine never executes; inference hook below.
 dir:=filepath.Dir(qualityEnginePath(a.dataDir));receipt:=qualityEngineReceipt{Version:qualityEngineVersion,Files:map[string]string{}}
 for _,name:=range qualityRequiredFiles(){p:=filepath.Join(dir,filepath.FromSlash(name));os.MkdirAll(filepath.Dir(p),0700);if e:=os.WriteFile(p,[]byte("test-only-fixture"),0600);e!=nil{t.Fatal(e)};h,e:=qualityFileHash(p);if e!=nil{t.Fatal(e)};receipt.Files[name]=h}
 data,_:=json.Marshal(receipt);os.WriteFile(filepath.Join(dir,"receipt.json"),data,0600)
 for _,j:=range a.state.Jobs{j.Status="queued"};calls:=0;a.qualityBatchHooks.infer=func(c context.Context,e,i,o string)error{calls++;return batchFakeInfer(c,e,i,o)}
 for _,m:=range ms{a.runDownload(m.id);j:=a.getJob(m.id);if j.Status!="done"||!nonempty(j.File)||!sameQualitySource(m.task.Source){t.Fatalf("job unsafe: %+v",j)}}
 if calls!=1||len(a.qualityBatchResults)!=0{t.Fatal("per-image reruns or cache leak",calls,len(a.qualityBatchResults))}
 entries,_:=os.ReadDir(filepath.Join(a.dataDir,"quality-batches"));if len(entries)!=0{t.Fatal("temporary grouped outputs retained")}
}
