# Real Windows TEST2 run: public Reddit post, isolated local port/data/output.
# Never uses a production ClipCatch process, settings, pairing key or download directory.
param([Parameter(Mandatory = $true)][string] $Executable)
$ErrorActionPreference = 'Stop'
$version = '2.0.1-test.1'
$source = 'https://www.reddit.com/r/catsistanbul/comments/1x0k17b/the_new_employee_has_an_idea/'
$base = 'http://127.0.0.1:18766'
$evidence = Join-Path $PWD 'test-evidence'
New-Item -ItemType Directory -Force -Path $evidence | Out-Null
$reportPath = Join-Path $evidence 'reddit-test2-live.json'
$report = [ordered]@{
    app = 'clipcatch-test2'
    version = $version
    source_post_id = '1x0k17b'
    environment = 'ephemeral-windows-runner'
    test_result = 'not_started'
    job_status = ''
    error_code = ''
    error_title = ''
    failure_stage = ''
    failure_http_code = 0
    valid_media_files = 0
    valid_media_bytes = 0
    ffprobe_verified = $false
}
$step = 'launch'
$proc = $null
$exitCode = 1
try {
    $env:CLIPCATCH_PORTABLE = '1'
    $env:CLIPCATCH_NO_BROWSER = '1'
    $env:CLIPCATCH_TEST2_SAVEDIR = Join-Path $env:RUNNER_TEMP 'ClipCatch-Test2-Reddit-media'
    $exe = (Resolve-Path -LiteralPath $Executable).Path
    $proc = Start-Process -FilePath $exe -PassThru
    $step = 'health'
    $health = $null
    for ($i=0; $i -lt 45; $i++) {
        Start-Sleep -Seconds 2
        try { $health = Invoke-RestMethod -Uri "$base/api/health" -TimeoutSec 4; break }
        catch { if ($proc.HasExited) { throw 'TEST2 executable exited before health check' } }
    }
    if ($null -eq $health -or $health.app -ne 'clipcatch-test2' -or
        $health.channel -ne 'test' -or $health.version -ne $version) {
        throw 'TEST2 health identity not verified'
    }
    $report.test_result = 'test2_health_confirmed'
    $step = 'bootstrap'
    $state = $null
    for ($i=0; $i -lt 150; $i++) {
        Start-Sleep -Seconds 2
        $state = Invoke-RestMethod -Uri "$base/api/state" -TimeoutSec 5
        if ($state.phase -eq 'ready' -and -not $state.update_checking -and -not $state.update_required) { break }
        if ($state.phase -eq 'error') { throw 'TEST2 bootstrap failed' }
    }
    if ($state.phase -ne 'ready' -or $state.update_checking -or $state.update_required) {
        throw 'TEST2 not ready for download'
    }
    $report.test_result = 'test2_engine_ready'
    $step = 'submit'
    $body = @{ urls = $source; output_mode = 'original' } | ConvertTo-Json -Compress
    $queued = Invoke-RestMethod -Uri "$base/api/download" -Method Post -ContentType 'application/json' -Headers @{ Origin = $base } -Body $body -TimeoutSec 12
    if (-not $queued.ok -or @($queued.job_ids).Count -ne 1) { throw 'TEST2 did not queue requested media' }
    $id = [string] $queued.job_ids[0]
    $step = 'download'
    $job = $null
    for ($i=0; $i -lt 240; $i++) {
        Start-Sleep -Seconds 2
        $state = Invoke-RestMethod -Uri "$base/api/state" -TimeoutSec 5
        $job = @($state.jobs) | Where-Object { $_.id -eq $id } | Select-Object -First 1
        if ($null -ne $job -and $job.status -in @('done','error','cancelled')) { break }
    }
    if ($null -eq $job -or $job.status -notin @('done','error','cancelled')) {
        $report.test_result = 'download_timeout'
        throw 'TEST2 download timed out'
    }
    $report.job_status = [string] $job.status
    $report.error_code = [string] $job.error_code
    $report.error_title = [string] $job.error_title
    if ($job.status -ne 'done') {
        $report.test_result = 'live_reddit_download_failed'
    } else {
        $paths = @()
        if ($job.file) { $paths += [string]$job.file }
        foreach ($media in @($job.media)) { if ($media -and $media.file) { $paths += [string] $media.file } }
        $paths = $paths | Sort-Object -Unique
        $probe = Join-Path $env:LOCALAPPDATA 'ClipCatch-Test2/tools/ffprobe.exe'
        $count = 0
        $bytes = [int64]0
        $probeOK = $false
        foreach ($path in $paths) {
            if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { continue }
            $item = Get-Item -LiteralPath $path
            if ($item.Length -le 0) { continue }
            $count++
            $bytes += $item.Length
            if (Test-Path -LiteralPath $probe -PathType Leaf) {
                $probeOutput = & $probe -v error -show_entries stream=codec_type -of csv=p=0 "$path" 2>$null
                if ($LASTEXITCODE -eq 0 -and ($probeOutput -match 'video')) { $probeOK = $true }
            }
        }
        $report.valid_media_files = $count
        $report.valid_media_bytes = $bytes
        $report.ffprobe_verified = $probeOK
        $report.test_result = if ($count -gt 0 -and $probeOK) { 'live_download_verified' } else { 'download_reported_done_but_file_unverified' }
        if ($report.test_result -eq 'live_download_verified') { $exitCode = 0 }
    }
} catch {
    if ($report.test_result -notin @('download_timeout', 'live_reddit_download_failed')) {
        $report.test_result = 'test_harness_failed_at_' + $step
    }
    $report.failure_stage = $step
    if ($_.Exception.Response -and $_.Exception.Response.StatusCode) {
        $report.failure_http_code = [int]$_.Exception.Response.StatusCode
    }
    Write-Warning ('TEST2 live verification was not successful at ' + $step + ' (HTTP ' + $report.failure_http_code + '); this is not a production result.')
} finally {
    if ($null -ne $proc) {
        try { if (-not $proc.HasExited) { Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue } } catch {}
    }
    $report | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath $reportPath -Encoding UTF8
    Write-Host ('TEST2 test result: ' + $report.test_result)
    Write-Host ('TEST2 job: ' + $report.job_status + ' / ' + $report.error_code)
    Write-Host ('Verified video files: ' + $report.valid_media_files)
}
exit $exitCode
