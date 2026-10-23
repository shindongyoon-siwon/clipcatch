package main

import("encoding/json";"fmt";"os";"path/filepath";"strings";"testing")
func TestFeedSeparateMediaJobsAndDurableDedup(t *testing.T){
 root:=t.TempDir();a:=newApp(root,t.TempDir());a.state.Phase="ready";token:=browserTestPair(t,a)
 for _,id:=range []string{"firstmedia123","secondmedia123"}{in:=browserMedia{PageURL:testPostURL,Title:"same post",MediaURL:"https://v.redd.it/"+id+"/DASHPlaylist.mpd",Separate:true};body,_:=json.Marshal(in);w:=browserRequest(a,"/bridge/download",testExtensionOrigin,token,string(body));if w.Code!=202{t.Fatal(w.Code,w.Body.String())}}
 if len(a.state.Jobs)!=2{t.Fatal("second video collapsed into first",len(a.state.Jobs))};if downloadJobKey(a.state.Jobs[0])==downloadJobKey(a.state.Jobs[1]){t.Fatal("media keys collided")}
 in:=browserMedia{PageURL:testPostURL,Title:"same post",MediaURL:"https://v.redd.it/firstmedia123/DASHPlaylist.mpd",Separate:true};body,_:=json.Marshal(in)
 w:=browserRequest(a,"/bridge/download",testExtensionOrigin,token,string(body));if w.Code!=200||!strings.Contains(w.Body.String(),"already_queued"){t.Fatal("duplicate pending did not skip",w.Body.String())}
 for i,j:=range a.state.Jobs{j.Status="done";j.File=filepath.Join(root,fmt.Sprintf("fixture%d.mp4",i));if e:=os.WriteFile(j.File,[]byte("fixture-only"),0600);e!=nil{t.Fatal(e)}}
 a.saveHistory();b:=newApp(root,t.TempDir());b.state.Phase="ready";w=browserRequest(b,"/bridge/download",testExtensionOrigin,token,string(body));if w.Code!=409{t.Fatal("individual media dedup lost after restart",w.Code,w.Body.String())}
 disk,_:=os.ReadFile(filepath.Join(root,"history.json"));if strings.Contains(string(disk),"v.redd.it"){t.Fatal("media URL persisted")}
}
func TestFeedNumberedXAndLegacyKeyCompatibility(t *testing.T){
 one:=canonicalJobKey("https://x.com/u/status/12345678/video/1");two:=canonicalJobKey("https://x.com/u/status/12345678/video/2");if one==two{t.Fatal("numbered videos collapsed")};if one!=canonicalJobKey("https://twitter.com/u/status/12345678/video/1?utm_source=x"){t.Fatal("aliases not normalized")};legacy:=&Job{ID:"123-1",URL:testPostURL,BrowserSource:true};if downloadJobKey(legacy)!=canonicalJobKey(testPostURL){t.Fatal("old history key changed")}
}
