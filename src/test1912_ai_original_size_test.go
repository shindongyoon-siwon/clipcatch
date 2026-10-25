package main

import (
 "context"
 "crypto/sha256"
 "os"
 "path/filepath"
 "testing"
)

// This tests the unchanged image inference pipeline at scale=1. The TEST 2
// option validator/UI is tested locally; this does not claim Windows UI coverage.
func Test1912ActualOriginalSizeAI(t *testing.T) {
 engine:=os.Getenv("CLIPCATCH_AI_ENGINE_TEST")
 if engine=="" {t.Skip("real engine required")}
 ff:=testQualityFF(t)
 dir:=t.TempDir()
 input:=testQualityImage(t,dir,false)
 original,_:=os.ReadFile(input)
 output:=filepath.Join(dir,"same-size.png")
 options:=qualityOptions{Mode:"ai",Scale:1,Strength:.65}
 if e:=renderQualityImage(context.Background(),ff,engine,input,output,dir,options,31,23);e!=nil {t.Fatal(e)}
 before,e:=readPNG(filepath.Join(dir,"reference.png"));if e!=nil{t.Fatal(e)}
 after,e:=readPNG(output);if e!=nil{t.Fatal(e)}
 if after.Bounds().Dx()!=31||after.Bounds().Dy()!=23{t.Fatal("unexpected size")}
 changed:=0
 for y:=0;y<23;y++{for x:=0;x<31;x++{r,g,b,_:=before.At(x,y).RGBA();rr,gg,bb,_:=after.At(x,y).RGBA();if r!=rr||g!=gg||b!=bb{changed++}}}
 if changed==0{t.Fatal("AI output is identical to ordinary resize")}
 preserved,_:=os.ReadFile(input)
 if sha256.Sum256(original)!=sha256.Sum256(preserved){t.Fatal("source changed")}
 t.Logf("Real model completed at original dimensions: 31x23; %d pixels differ. Not a subjective quality guarantee.",changed)
}
