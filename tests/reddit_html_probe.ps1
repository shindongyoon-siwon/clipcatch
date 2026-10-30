# TEST2 read-only diagnosis of publicly accessible Reddit post pages.
# No cookies, no authentication, no CAPTCHA solving and no remote media download.
$ErrorActionPreference = 'Stop'
$post='https://www.reddit.com/r/catsistanbul/comments/1x0k17b/the_new_employee_has_an_idea/'
$urls=@(
    @{name='www-post'; url=$post},
    @{name='old-post'; url=($post -replace 'https://www.reddit.com/', 'https://old.reddit.com/')},
    @{name='sh-post'; url=($post -replace 'https://www.reddit.com/', 'https://sh.reddit.com/')}
)
$report=@()
foreach($u in $urls){
    $row=[ordered]@{name=$u.name;http=0;bytes=0;media_ids=0;og_video=$false;shreddit_player=$false;blocked=$false}
    try{
        $req=[System.Net.Http.HttpRequestMessage]::new([System.Net.Http.HttpMethod]::Get,$u.url)
        $req.Headers.UserAgent.ParseAdd('ClipCatch-TEST2/2.0.1')
        $client=[System.Net.Http.HttpClient]::new()
        $client.Timeout=[TimeSpan]::FromSeconds(12)
        $resp=$client.SendAsync($req).GetAwaiter().GetResult()
        $row.http=[int]$resp.StatusCode
        $text=$resp.Content.ReadAsStringAsync().GetAwaiter().GetResult()
        $row.bytes=$text.Length
        $matches=[regex]::Matches($text,'(?:https?:)?(?:\\?/\\?/|//)v\\?\\?.redd\\?\\?.it\\?\\?/([A-Za-z0-9]{6,20})','IgnoreCase')
        $row.media_ids=@($matches | ForEach-Object {$_.Groups[1].Value} | Sort-Object -Unique).Count
        $row.og_video=($text -match 'og:video')
        $row.shreddit_player=($text -match 'shreddit-player')
        $row.blocked=($text -match 'Blocked by network security|unable to access the Reddit API|whoa there|You have been blocked')
    }catch{
        if($_.Exception.Response -and $_.Exception.Response.StatusCode){$row.http=[int]$_.Exception.Response.StatusCode}
    }
    finally{if($null -ne $client){$client.Dispose()};$client=$null}
    $report+=$row
}
$report|ConvertTo-Json -Depth 3|Set-Content -LiteralPath test-evidence/reddit-html-probe.json -Encoding UTF8
foreach($row in $report){Write-Host ('HTML '+$row.name+': HTTP '+$row.http+', chars '+$row.bytes+', v.redd.it IDs '+$row.media_ids+', og '+$row.og_video+', player '+$row.shreddit_player+', blocked '+$row.blocked)}
