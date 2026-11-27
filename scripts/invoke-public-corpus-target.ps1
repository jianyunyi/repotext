param(
    [Parameter(Mandatory=$true)][string]$ScriptPath,
    [Parameter(Mandatory=$true)][string]$EvidenceName,
    [switch]$Quiet
)
$ErrorActionPreference='Stop'
$projectRoot=Split-Path -Parent $PSScriptRoot
$scriptFull=(Resolve-Path -LiteralPath $ScriptPath).Path
if (!$scriptFull.StartsWith($projectRoot+'\work\',[StringComparison]::OrdinalIgnoreCase)) { throw 'Script must remain in workspace work directory' }
if ($EvidenceName -notmatch '^[a-zA-Z0-9-]+$') { throw 'Invalid evidence name' }
$evidenceRoot=Join-Path $projectRoot ('work/'+$EvidenceName)
New-Item -ItemType Directory -Path $evidenceRoot -Force | Out-Null
$utf8=[Text.UTF8Encoding]::new($false)
$scriptText=[IO.File]::ReadAllText($scriptFull).Replace("`r",'').TrimStart([char]0xfeff)
$scriptBytes=$utf8.GetBytes($scriptText)
$info=[Diagnostics.ProcessStartInfo]::new()
$info.FileName='C:\Windows\System32\OpenSSH\ssh.exe'
$info.Arguments='-F C:\Users\asus\.ssh\config -o BatchMode=yes -o StrictHostKeyChecking=yes -o ConnectTimeout=15 instabox-test-env sh -s'
$info.UseShellExecute=$false
$info.CreateNoWindow=$true
$info.RedirectStandardInput=$true
$info.RedirectStandardOutput=$true
$info.RedirectStandardError=$true
[Console]::InputEncoding=$utf8
$proc=[Diagnostics.Process]::new()
$proc.StartInfo=$info
[void]$proc.Start()
$stdoutTask=$proc.StandardOutput.ReadToEndAsync()
$stderrTask=$proc.StandardError.ReadToEndAsync()
$transferError=$null
try { $proc.StandardInput.BaseStream.Write($scriptBytes,0,$scriptBytes.Length) }
catch { $transferError=$_.Exception.Message }
finally { $proc.StandardInput.Close() }
$proc.WaitForExit()
$stdoutText=$stdoutTask.GetAwaiter().GetResult()
$stderrText=$stderrTask.GetAwaiter().GetResult()
$exitCode=$proc.ExitCode
if ($transferError -and $exitCode -eq 0) { $exitCode=1 }
[IO.File]::WriteAllText((Join-Path $evidenceRoot 'stdout.log'),$stdoutText,$utf8)
[IO.File]::WriteAllText((Join-Path $evidenceRoot 'stderr.log'),$stderrText,$utf8)
[IO.File]::WriteAllText((Join-Path $evidenceRoot 'result.json'),(@{exit_code=$exitCode;transfer_error=$transferError;finished_at=[DateTimeOffset]::Now.ToString('o');script_sha256=(Get-FileHash -LiteralPath $scriptFull -Algorithm SHA256).Hash.ToLowerInvariant();target='instabox-test-env'} | ConvertTo-Json),$utf8)
if (!$Quiet) { [Console]::Out.Write($stdoutText) }
[Console]::Error.Write($stderrText)
exit $exitCode
