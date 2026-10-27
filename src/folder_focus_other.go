//go:build !windows

package main
import "context"
func openDirectoryFocusedNative(ctx context.Context,path string)(folderFocusResult,error){
 if e:=ctx.Err();e!=nil{return folderFocusResult{},e}
 e:=openDirectoryNative(path);return folderFocusResult{Requested:e==nil,Reason:"unverified"},e
}
func handleFolderFocusCommand()bool{return false}
