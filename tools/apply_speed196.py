"""Readable test.2 integration edits. Run after apply_dev196.py and gofmt.
No production refs, release publishing, autostart, model or GPU setting changes.
"""
from pathlib import Path
R=Path(__file__).resolve().parents[1]
def edit(path,before,after):
 p=R/path;s=p.read_text(encoding='utf-8')
 if after in s:return
 if s.count(before)!=1:raise RuntimeError('Unexpected development baseline: '+path+' / '+before[:80])
 p.write_text(s.replace(before,after,1),encoding='utf-8')
edit('src/main.go','type App struct {','type App struct {\n qualityBatchResults map[string]*qualityBatchResult\n qualityBatchHooks qualityBatchHooks')
edit('src/quality_media.go','func renderQualityImage(ctx context.Context, ff, engine, source, dest, work string, o qualityOptions, w, h int) error {','''func renderQualityImage(ctx context.Context, ff, engine, source, dest, work string, o qualityOptions, w, h int) error {
 return renderQualityImagePrepared(ctx,ff,engine,source,dest,work,o,w,h,"")
}
func renderQualityImagePrepared(ctx context.Context, ff, engine, source, dest, work string, o qualityOptions, w, h int, prepared string) error {''')
edit('src/quality_media.go','\tin := filepath.Join(work, "input")\n\tout := filepath.Join(work, "output")\n\tos.MkdirAll(in, 0700)','\tif prepared == "" {\n\tin := filepath.Join(work, "input")\n\tout := filepath.Join(work, "output")\n\tos.MkdirAll(in, 0700)')
edit('src/quality_media.go','\tscaled := filepath.Join(work, "scaled.png")\n\tif e := qualityPNG(ctx, ff, filepath.Join(out, "frame.png"), scaled, w, h); e != nil {','\tprepared = filepath.Join(out,"frame.png")\n\t}\n\tscaled := filepath.Join(work, "scaled.png")\n\tif e := qualityPNG(ctx, ff, prepared, scaled, w, h); e != nil {')
edit('src/quality.go','func (a *App) runQualityTask(ctx context.Context, id string, j *Job) {','func (a *App) runQualityTask(ctx context.Context, id string, j *Job) {\n defer a.discardQualityBatchResult(id)')
edit('src/quality.go','\t\te = renderQualityImage(ctx, a.ffmpeg, engine, q.Path, output, work, o, q.Width*o.Scale, q.Height*o.Scale)','''        prepared := ""
        if o.Mode == "ai" && task.Preview {
            cached, release := a.takeQualityBatchResult(id,task)
            if cached == nil {
                a.prepareQualityImageBatch(id,task,engine)
                cached, release = a.takeQualityBatchResult(id,task)
            }
            defer release()
            if ctx.Err()!=nil || a.isJobCancelled(id) { return }
            if cached!=nil {
                if cached.err!=nil {fail(cached.err);return}
                prepared=cached.path
            }
        }
        e = renderQualityImagePrepared(ctx,a.ffmpeg,engine,q.Path,output,work,o,q.Width*o.Scale,q.Height*o.Scale,prepared)''')
edit('src/quality.go','\ta.cleanQualityInputs()','\ta.cleanQualityInputs()\n _ = os.RemoveAll(filepath.Join(a.dataDir,"quality-batches"))')
edit('src/queue_controls.go','\tif stop != nil {\n\t\tstop()\n\t}\n\ta.saveHistory()','\tif stop != nil {\n\t\tstop()\n\t}\n a.discardQualityBatchResult(input.ID)\n\ta.saveHistory()')
edit('src/quality_media.go','func qualityAI(ctx context.Context, engine, indir, outdir string) error {','''func qualityAI(ctx context.Context, engine, indir, outdir string) error {
 return qualityAIWithin(ctx,engine,indir,outdir,10*time.Minute)
}
func qualityAIWithin(ctx context.Context, engine, indir, outdir string, limit time.Duration) error {''')
edit('src/quality_media.go','context.WithTimeout(ctx, 10*time.Minute)','context.WithTimeout(ctx, limit)')
(R/'TEST_VERSION').write_text('1.9.6-test.2\n',encoding='utf-8')
p=R/'tests/windows_dev196.py';p.write_text(p.read_text(encoding='utf-8').replace('1.9.6-test.1','1.9.6-test.2'),encoding='utf-8')
print('Applied bounded image grouping only to development source')
