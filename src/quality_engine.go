package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const qualityEngineVersion = "realesrgan-ncnn-20220424"
const qualityEngineURL = "https://github.com/xinntao/Real-ESRGAN/releases/download/v0.2.5.0/realesrgan-ncnn-vulkan-20220424-windows.zip"

// Computed from the official version-pinned release in the reviewed CI job.
// Integrity pinning is not a malware scan or a code-signing certificate.
const qualityEngineSHA = "abc02804e17982a3be33675e4d471e91ea374e65b70167abc09e31acb412802d"

type qualityEngineReceipt struct {
	Version string            `json:"version"`
	Files   map[string]string `json:"files"`
}

func qualityFileHash(path string) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return "", e
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func qualityEnginePath(data string) string {
	return filepath.Join(data, "quality-engine", qualityEngineVersion, "realesrgan-ncnn-vulkan.exe")
}
func qualityRequiredFiles() []string {
	return []string{"realesrgan-ncnn-vulkan.exe", "vcomp140.dll", "vcomp140d.dll", "models/realesrgan-x4plus.bin", "models/realesrgan-x4plus.param"}
}
func verifyQualityEngine(data string) (string, error) {
	engine := qualityEnginePath(data)
	dir := filepath.Dir(engine)
	b, e := os.ReadFile(filepath.Join(dir, "receipt.json"))
	if e != nil || len(b) > 65536 {
		return "", errors.New("quality-engine")
	}
	var receipt qualityEngineReceipt
	if json.Unmarshal(b, &receipt) != nil || receipt.Version != qualityEngineVersion {
		return "", errors.New("quality-engine")
	}
	for _, name := range qualityRequiredFiles() {
		path := filepath.Join(dir, filepath.FromSlash(name))
		st, e := os.Lstat(path)
		if e != nil || !st.Mode().IsRegular() {
			return "", errors.New("quality-engine")
		}
		got, e := qualityFileHash(path)
		if e != nil || got != receipt.Files[name] {
			return "", errors.New("quality-integrity")
		}
	}
	return engine, nil
}
func qualityDownloadAllowed(raw string) bool {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	h := u.Hostname()
	return h == "github.com" || h == "release-assets.githubusercontent.com" || h == "objects.githubusercontent.com"
}
func downloadQualityEngine(ctx context.Context, target string) error {
	client := &http.Client{Timeout: 5 * time.Minute, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) > 4 || !qualityDownloadAllowed(r.URL.String()) {
			return errors.New("quality-download-redirect")
		}
		return nil
	}}
	req, e := http.NewRequestWithContext(ctx, "GET", qualityEngineURL, nil)
	if e != nil {
		return e
	}
	req.Header.Set("User-Agent", "ClipCatch-Test2/"+appVersion)
	resp, e := client.Do(req)
	if e != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var ne net.Error
		if errors.As(e, &ne) && ne.Timeout() {
			return errors.New("quality-download-timeout")
		}
		return errors.New("quality-download-network")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("quality-download-http-%d", resp.StatusCode)
	}
	if resp.ContentLength > 64<<20 {
		return errors.New("quality-archive")
	}
	f, e := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	h := sha256.New()
	n, e := io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, (64<<20)+1))
	ce := f.Close()
	if e != nil || ce != nil || n > 64<<20 {
		return errors.New("quality-download")
	}
	if hex.EncodeToString(h.Sum(nil)) != qualityEngineSHA {
		return errors.New("quality-integrity")
	}
	return nil
}

// The ClipCatch Windows EXE embeds the official archive. User consent still
// controls unpacking. This path makes zero HTTP requests and never falls back
// to an unverified mirror or modifies browser/Windows security settings.
func obtainQualityArchive(ctx context.Context, target string) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	if len(bundledQualityEngine) == 0 {
		return downloadQualityEngine(ctx, target)
	}
	if len(bundledQualityEngine) > 64<<20 {
		return errors.New("quality-archive")
	}
	got := sha256.Sum256(bundledQualityEngine)
	if hex.EncodeToString(got[:]) != qualityEngineSHA {
		return errors.New("quality-integrity")
	}
	f, e := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return errors.New("quality-disk")
	}
	_, e = io.Copy(f, bytes.NewReader(bundledQualityEngine))
	ce := f.Close()
	if e != nil || ce != nil {
		return errors.New("quality-disk")
	}
	return ctx.Err()
}

func extractQualityEngine(archive, dest string) error {
	z, e := zip.OpenReader(archive)
	if e != nil {
		return errors.New("quality-archive")
	}
	defer z.Close()
	if len(z.File) > 200 {
		return errors.New("quality-archive")
	}
	receipt := qualityEngineReceipt{Version: qualityEngineVersion, Files: map[string]string{}}
	var total int64
	for _, f := range z.File {
		name := strings.ReplaceAll(f.Name, "\\", "/")
		// Validate every archive member, including entries that will not be installed.
		if strings.HasPrefix(name, "/") || strings.Contains(name, ":") || f.Mode()&os.ModeSymlink != 0 {
			return errors.New("quality-archive")
		}
		for _, part := range strings.Split(name, "/") {
			if part == ".." {
				return errors.New("quality-archive")
			}
		}
		if f.FileInfo().IsDir() {
			continue
		}
		rel := ""
		for _, need := range qualityRequiredFiles() {
			if name == need || strings.HasSuffix(name, "/"+need) {
				rel = need
				break
			}
		}
		base := filepath.Base(name)
		if strings.HasPrefix(strings.ToUpper(base), "LICENSE") || strings.HasPrefix(strings.ToUpper(base), "README") {
			if f.UncompressedSize64 <= 256<<10 {
				rel = base
			}
		}
		if rel == "" {
			continue
		}
		if _, ok := receipt.Files[rel]; ok || f.UncompressedSize64 > 128<<20 {
			return errors.New("quality-archive")
		}
		total += int64(f.UncompressedSize64)
		if total > 160<<20 {
			return errors.New("quality-archive")
		}
		path := filepath.Join(dest, filepath.FromSlash(rel))
		if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
			return e
		}
		in, e := f.Open()
		if e != nil {
			return e
		}
		out, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			in.Close()
			return e
		}
		h := sha256.New()
		n, err := io.Copy(io.MultiWriter(out, h), io.LimitReader(in, (128<<20)+1))
		in.Close()
		ce := out.Close()
		if err != nil || ce != nil || n != int64(f.UncompressedSize64) {
			return errors.New("quality-archive")
		}
		receipt.Files[rel] = hex.EncodeToString(h.Sum(nil))
	}
	for _, need := range qualityRequiredFiles() {
		if receipt.Files[need] == "" {
			return errors.New("quality-archive")
		}
	}
	if e := os.WriteFile(filepath.Join(dest, "LICENSE-Real-ESRGAN.txt"), []byte(qualityLicense), 0600); e != nil {
		return e
	}
	b, _ := json.MarshalIndent(receipt, "", "  ")
	return os.WriteFile(filepath.Join(dest, "receipt.json"), b, 0600)
}
func installQualityEngine(ctx context.Context, data string) error {
	if runtime.GOOS != "windows" {
		return errors.New("quality-windows")
	}
	return prepareQualityEngine(ctx, data)
}

// Extracts an integrity-checked bundle without executing its Windows binary.
func prepareQualityEngine(ctx context.Context, data string) error {
	if _, e := verifyQualityEngine(data); e == nil {
		return nil
	}
	dir := filepath.Join(data, "quality-engine")
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	work, e := os.MkdirTemp(dir, "install-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(work)
	archive := filepath.Join(work, "engine.zip")
	if e = obtainQualityArchive(ctx, archive); e != nil {
		return e
	}
	prepared := filepath.Join(work, qualityEngineVersion)
	if e = extractQualityEngine(archive, prepared); e != nil {
		if _, ok := e.(*os.PathError); ok {
			return errors.New("quality-disk")
		}
		return e
	}
	final := filepath.Dir(qualityEnginePath(data))
	// Never overwrite a live engine. A damaged old installation is kept as a backup.
	backup := final + ".backup"
	if _, e = os.Stat(final); e == nil {
		if _, e = os.Stat(backup); e == nil {
			return errors.New("quality-engine-backup")
		}
		if e = os.Rename(final, backup); e != nil {
			return e
		}
	}
	if e = os.Rename(prepared, final); e != nil {
		_ = os.Rename(backup, final)
		return e
	}
	_, e = verifyQualityEngine(data)
	return e
}

const qualityLicense = `BSD 3-Clause License

Copyright (c) 2021, Xintao Wang
All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:

1. Redistributions of source code must retain the above copyright notice, this
   list of conditions and the following disclaimer.

2. Redistributions in binary form must reproduce the above copyright notice,
   this list of conditions and the following disclaimer in the documentation
   and/or other materials provided with the distribution.

3. Neither the name of the copyright holder nor the names of its
   contributors may be used to endorse or promote products derived from
   this software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
`
