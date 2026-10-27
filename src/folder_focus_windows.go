//go:build windows

package main

import (
 "context"
 "encoding/binary"
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "os/exec"
 "path/filepath"
 "runtime"
 "strings"
 "syscall"
 "time"
 "unsafe"
)

var folderUser32=syscall.NewLazyDLL("user32.dll")
var folderGetForeground=folderUser32.NewProc("GetForegroundWindow")
var folderSetForeground=folderUser32.NewProc("SetForegroundWindow")
var folderIsIconic=folderUser32.NewProc("IsIconic")
var folderShowAsync=folderUser32.NewProc("ShowWindowAsync")
var folderGetClass=folderUser32.NewProc("GetClassNameW")
var folderIsVisible=folderUser32.NewProc("IsWindowVisible")
var folderLastInput=folderUser32.NewProc("GetLastInputInfo")
var folderCoCreate=syscall.NewLazyDLL("ole32.dll").NewProc("CoCreateInstance")
var folderVariantClear=syscall.NewLazyDLL("oleaut32.dll").NewProc("VariantClear")
var folderStringLen=syscall.NewLazyDLL("oleaut32.dll").NewProc("SysStringLen")
var folderShellWindowsCLSID=syscall.GUID{Data1:0x9ba05972,Data2:0xf6a8,Data3:0x11cf,Data4:[8]byte{0xa4,0x42,0,0xa0,0xc9,0x0a,0x8f,0x39}}
var folderDispatchIID=syscall.GUID{Data1:0x00020400,Data4:[8]byte{0xc0,0,0,0,0,0,0,0x46}}
var folderNullIID=syscall.GUID{}
var folderProviderIID=syscall.GUID{Data1:0x6d5140c1,Data2:0x7436,Data3:0x11ce,Data4:[8]byte{0x80,0x34,0,0xaa,0,0x60,0x09,0xfa}}
var folderTopBrowserSID=syscall.GUID{Data1:0x4c96be40,Data2:0x915c,Data3:0x11cf,Data4:[8]byte{0x99,0xd3,0,0xaa,0,0x4a,0xe8,0x37}}
var folderBrowserIID=syscall.GUID{Data1:0x000214e2,Data4:[8]byte{0xc0,0,0,0,0,0,0,0x46}}

// VARIANT on the 64-bit Windows build. Pointer-valued unions are read directly
// from the returned storage; no pointer is round-tripped through a Go integer.
type folderVariant struct { _ [0]uint64; VT uint16; Reserved [3]uint16; Value [16]byte }
type folderDispParams struct { Args *folderVariant; Named *int32; Count, NamedCount uint32 }
type folderDispatch struct { VTable *[7]uintptr }
func folderHR(hr uintptr) error {if int32(uint32(hr))<0{return fmt.Errorf("shell HRESULT %08x",uint32(hr))};return nil}
func (d *folderDispatch) release(){if d!=nil{syscall.SyscallN(d.VTable[2],uintptr(unsafe.Pointer(d)))}}
func (v *folderVariant) clear(){folderVariantClear.Call(uintptr(unsafe.Pointer(v)))}
func (v *folderVariant) integer() int64 {
 switch v.VT {case 3,22:return int64(int32(binary.LittleEndian.Uint32(v.Value[:4])));case 19,23:return int64(binary.LittleEndian.Uint32(v.Value[:4]));case 20:return int64(binary.LittleEndian.Uint64(v.Value[:8]))}
 return 0
}
func (v *folderVariant) text() string {
 if v.VT!=8{return ""}
 p:=*(**uint16)(unsafe.Pointer(&v.Value[0]));if p==nil{return ""}
 n,_,_:=folderStringLen.Call(uintptr(unsafe.Pointer(p)));if n>32768{return ""}
 return syscall.UTF16ToString(unsafe.Slice(p,int(n)))
}
func (v *folderVariant) dispatch() *folderDispatch {
 if v.VT!=9{return nil}
 d:=*(**folderDispatch)(unsafe.Pointer(&v.Value[0]));if d!=nil{syscall.SyscallN(d.VTable[1],uintptr(unsafe.Pointer(d)))}
 return d
}
func (d *folderDispatch) invoke(name string, flags uint16, arg *folderVariant)(folderVariant,error){
 var result folderVariant
 if d==nil{return result,errors.New("missing shell object")}
 n,e:=syscall.UTF16PtrFromString(name);if e!=nil{return result,e};var id int32
 hr,_,_:=syscall.SyscallN(d.VTable[5],uintptr(unsafe.Pointer(d)),uintptr(unsafe.Pointer(&folderNullIID)),uintptr(unsafe.Pointer(&n)),1,0,uintptr(unsafe.Pointer(&id)))
 runtime.KeepAlive(n);if e=folderHR(hr);e!=nil{return result,e}
 params:=folderDispParams{Args:arg};if arg!=nil{params.Count=1}
 hr,_,_ = syscall.SyscallN(d.VTable[6],uintptr(unsafe.Pointer(d)),uintptr(id),uintptr(unsafe.Pointer(&folderNullIID)),0,uintptr(flags),uintptr(unsafe.Pointer(&params)),uintptr(unsafe.Pointer(&result)),0,0)
 runtime.KeepAlive(arg);runtime.KeepAlive(d)
 return result,folderHR(hr)
}
func(d *folderDispatch) object(name string)*folderDispatch{v,e:=d.invoke(name,2,nil);defer v.clear();if e!=nil{return nil};return v.dispatch()}
func(d *folderDispatch) text(name string)string{v,e:=d.invoke(name,2,nil);defer v.clear();if e!=nil{return ""};return v.text()}
func(d *folderDispatch) integer(name string)int64{v,e:=d.invoke(name,2,nil);defer v.clear();if e!=nil{return 0};return v.integer()}
func folderExplorerClass(hwnd uintptr)bool{
 if hwnd==0{return false};var name [128]uint16
 n,_,_:=folderGetClass.Call(hwnd,uintptr(unsafe.Pointer(&name[0])),uintptr(len(name)))
 if n==0{return false};s:=syscall.UTF16ToString(name[:]);return s=="CabinetWClass"||s=="ExploreWClass"
}
func folderExactPath(a,b string)bool{return a!=""&&b!=""&&strings.EqualFold(filepath.Clean(a),filepath.Clean(b))}
func folderWindowPath(w *folderDispatch)string {
 doc:=w.object("Document");if doc==nil{return ""};defer doc.release()
 folder:=doc.object("Folder");if folder==nil{return ""};defer folder.release()
 self:=folder.object("Self");if self==nil{return ""};defer self.release()
 return self.text("Path")
}
// Hidden Explorer tabs must not count as visible matching folder windows.
func folderVisibleView(w *folderDispatch)bool {
 var provider,browser,view *folderDispatch
 hr,_,_:=syscall.SyscallN(w.VTable[0],uintptr(unsafe.Pointer(w)),uintptr(unsafe.Pointer(&folderProviderIID)),uintptr(unsafe.Pointer(&provider)))
 if folderHR(hr)!=nil||provider==nil{return false};defer provider.release()
 hr,_,_=syscall.SyscallN(provider.VTable[3],uintptr(unsafe.Pointer(provider)),uintptr(unsafe.Pointer(&folderTopBrowserSID)),uintptr(unsafe.Pointer(&folderBrowserIID)),uintptr(unsafe.Pointer(&browser)))
 if folderHR(hr)!=nil||browser==nil{return false};defer browser.release()
 table:=(*[18]uintptr)(unsafe.Pointer(browser.VTable))
 hr,_,_=syscall.SyscallN(table[15],uintptr(unsafe.Pointer(browser)),uintptr(unsafe.Pointer(&view)))
 if folderHR(hr)!=nil||view==nil{return false};defer view.release()
 var hwnd uintptr
 hr,_,_=syscall.SyscallN(view.VTable[3],uintptr(unsafe.Pointer(view)),uintptr(unsafe.Pointer(&hwnd)))
 if folderHR(hr)!=nil||hwnd==0{return false}
 visible,_,_:=folderIsVisible.Call(hwnd);return visible!=0
}
func folderFindWindow(shell *folderDispatch,path string,wanted uintptr)uintptr {
 if shell==nil{return 0}
 count:=shell.integer("Count");if count<0||count>512{return 0}
 foreground,_,_:=folderGetForeground.Call();var match uintptr
 for i:=int64(0);i<count;i++ {
  arg:=folderVariant{VT:3};binary.LittleEndian.PutUint32(arg.Value[:4],uint32(i))
  value,e:=shell.invoke("Item",1,&arg);if e!=nil{value.clear();continue};w:=value.dispatch();value.clear();if w==nil{continue}
  hwnd:=uintptr(uint32(w.integer("HWND")))
  if (wanted==0||hwnd==wanted)&&folderExplorerClass(hwnd)&&strings.EqualFold(filepath.Base(w.text("FullName")),"explorer.exe")&&folderExactPath(folderWindowPath(w),path)&&folderVisibleView(w){match=hwnd}
  w.release()
  if match!=0 && (wanted!=0||match==foreground){return match}
 }
 return match
}
func folderFocusOnThread(path string)(folderFocusResult,error){
 full,e:=checkedFolderPath(path);if e!=nil{return folderFocusResult{},e}
 // ARM/32-bit ABI is not guessed; retain ordinary shell opening there.
 if runtime.GOARCH!="amd64"{e=openDirectoryNative(full);return folderFocusResult{Requested:e==nil,Reason:"unverified"},e}
 runtime.LockOSThread();defer runtime.UnlockOSThread()
 for _,p:=range []*syscall.LazyProc{folderCoInit,folderCoUninit,folderCoCreate,folderVariantClear,folderStringLen,folderGetForeground,folderSetForeground,folderIsIconic,folderShowAsync,folderGetClass,folderLastInput,folderIsVisible}{if e=p.Find();e!=nil{return folderFocusResult{},e}}
 hr,_,_:=folderCoInit.Call(0,0x2|0x4);if e=folderHR(hr);e!=nil{return folderFocusResult{},e};defer folderCoUninit.Call()
 var shell *folderDispatch
 hr,_,_=folderCoCreate.Call(uintptr(unsafe.Pointer(&folderShellWindowsCLSID)),0,23,uintptr(unsafe.Pointer(&folderDispatchIID)),uintptr(unsafe.Pointer(&shell)))
 if folderHR(hr)!=nil||shell==nil {e=openDirectoryNative(full);return folderFocusResult{Requested:e==nil,Reason:"unverified"},e}
 defer shell.release()
 stamp:=func()uint32{v:=struct{Size,Tick uint32}{Size:8};folderLastInput.Call(uintptr(unsafe.Pointer(&v)));return v.Tick}
 ops:=folderFocusOps{
  Find:func()uintptr{return folderFindWindow(shell,full,0)},Open:func()error{return openDirectoryNative(full)},
  Foreground:func()uintptr{h,_,_:=folderGetForeground.Call();return h},InputStamp:stamp,
  Minimized:func(h uintptr)bool{v,_,_:=folderIsIconic.Call(h);return v!=0},
  Restore:func(h uintptr){folderShowAsync.Call(h,9)}, // SW_RESTORE, no permanent topmost.
  Activate:func(h uintptr){folderSetForeground.Call(h)},
  StillTarget:func(h uintptr)bool{return folderFindWindow(shell,full,h)==h},Sleep:time.Sleep,
 }
 ctx,cancel:=context.WithTimeout(context.Background(),3*time.Second);defer cancel()
 return requestFolderFocus(ctx,ops,25)
}

// Run COM in a short-lived instance of the verified app itself, not a
// PowerShell script. A hung shell extension cannot pin the main UI thread.
func openDirectoryFocusedNative(ctx context.Context,path string)(folderFocusResult,error){
 self,e:=os.Executable();if e!=nil{return folderFocusResult{},e}
 limited,cancel:=context.WithTimeout(ctx,6*time.Second);defer cancel()
 cmd:=exec.CommandContext(limited,self,"--clipcatch-folder-focus",path)
 hideProcessWindow(cmd) // helper has no UI; Explorer uses SW_SHOWNORMAL above.
 cmd.WaitDelay=250*time.Millisecond // kill this helper only, never Explorer.
 output:=&cappedBuffer{Limit:4096};cmd.Stdout=output;cmd.Stderr=&cappedBuffer{Limit:2048}
 if e=cmd.Run();e!=nil {return folderFocusResult{},fmt.Errorf("folder helper: %w",e)}
 var response struct{Result folderFocusResult `json:"result"`;Error string `json:"error,omitempty"`}
 if e=json.Unmarshal(output.Data,&response);e!=nil{return folderFocusResult{},errors.New("folder helper returned an invalid response")}
 if response.Error!=""{return response.Result,errors.New(response.Error)}
 if !response.Result.Requested{return response.Result,errors.New("folder request was not accepted")}
 return response.Result,nil
}
func handleFolderFocusCommand()bool{
 if len(os.Args)<2||os.Args[1]!="--clipcatch-folder-focus"{return false}
 r:=folderFocusResult{};var e error
 if len(os.Args)!=3{e=errors.New("invalid folder helper arguments")}else{r,e=folderFocusOnThread(os.Args[2])}
 response:=struct{Result folderFocusResult `json:"result"`;Error string `json:"error,omitempty"`}{Result:r}
 if e!=nil{response.Error=e.Error()};_ = json.NewEncoder(os.Stdout).Encode(response)
 return true
}
