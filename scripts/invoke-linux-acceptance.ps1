param(
    [string]$RemoteScript = 'scripts/verify-linux-acceptance.sh',
    [string]$EvidenceName = 's4-linux-20261121',
    [switch]$PrepareOnly
)
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$evidenceRoot = Join-Path $projectRoot ('work/' + $EvidenceName)
New-Item -ItemType Directory -Path $evidenceRoot -Force | Out-Null
$sourceFiles = @(Get-ChildItem (Join-Path $projectRoot 'cmd'), (Join-Path $projectRoot 'internal'), (Join-Path $projectRoot 'scripts'), (Join-Path $projectRoot 'deploy') -Recurse -File)
$sourceFiles += Get-Item (Join-Path $projectRoot 'go.mod'), (Join-Path $projectRoot 'go.sum')
$manifest = $sourceFiles | Sort-Object FullName | ForEach-Object {
    $relative = $_.FullName.Substring($projectRoot.Length + 1).Replace('\', '/')
    ('{0}  {1}' -f (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant(), $relative)
}
[IO.File]::WriteAllText((Join-Path $evidenceRoot 'source.sha256'), (($manifest -join "`n") + "`n"), [Text.UTF8Encoding]::new($false))
$archivePath = Join-Path $evidenceRoot 'source.tar.gz'
& tar -czf $archivePath -C $projectRoot cmd internal scripts deploy go.mod go.sum -C $evidenceRoot source.sha256
if ($LASTEXITCODE -ne 0) { throw 'Source archive failed' }
Get-FileHash -LiteralPath $archivePath -Algorithm SHA256 | Format-List | Out-File (Join-Path $evidenceRoot 'archive.sha256')
if ($PrepareOnly) {
    Write-Output "ACCEPTANCE_PACKAGE_PREPARED: $evidenceRoot"
    exit 0
}
$script:remoteCommand = 'set -eu; d=$(mktemp -d /tmp/repotest-acceptance.XXXXXXXX); trap ''rm -rf -- "$d"'' EXIT HUP INT TERM; tar -xzf - -C "$d"; cd "$d"; sha256sum -c source.sha256; find scripts deploy -type f -exec sed -i ''s/\r$//'' {} +; sh ' + $RemoteScript
$startInfo = [Diagnostics.ProcessStartInfo]::new()
$startInfo.FileName = 'C:\Windows\System32\OpenSSH\ssh.exe'
$startInfo.Arguments = '-F C:\Users\asus\.ssh\config -o BatchMode=yes -o StrictHostKeyChecking=yes -o ConnectTimeout=15 instabox-test-env "' + $script:remoteCommand.Replace('"', '\"') + '"'
$startInfo.UseShellExecute = $false
$startInfo.CreateNoWindow = $true
$startInfo.RedirectStandardInput = $true
[Console]::InputEncoding = [Text.UTF8Encoding]::new($false)
$startInfo.RedirectStandardOutput = $true
$startInfo.RedirectStandardError = $true
$remoteProcess = [Diagnostics.Process]::new()
$remoteProcess.StartInfo = $startInfo
[void]$remoteProcess.Start()
$stdoutTask = $remoteProcess.StandardOutput.ReadToEndAsync()
$stderrTask = $remoteProcess.StandardError.ReadToEndAsync()
$archiveStream = [IO.File]::OpenRead($archivePath)
$transferError = $null
try { $archiveStream.CopyTo($remoteProcess.StandardInput.BaseStream) }
catch { $transferError = $_.Exception.Message }
finally { $archiveStream.Dispose(); $remoteProcess.StandardInput.Close() }
$remoteProcess.WaitForExit()
$stdoutText = $stdoutTask.GetAwaiter().GetResult()
$stderrText = $stderrTask.GetAwaiter().GetResult()
$exitCode = $remoteProcess.ExitCode
if ($transferError -and $exitCode -eq 0) { $exitCode = 1 }
[IO.File]::WriteAllText((Join-Path $evidenceRoot 'stdout.log'), $stdoutText, [Text.UTF8Encoding]::new($false))
[IO.File]::WriteAllText((Join-Path $evidenceRoot 'stderr.log'), $stderrText, [Text.UTF8Encoding]::new($false))
[IO.File]::WriteAllText((Join-Path $evidenceRoot 'result.json'), (@{ exit_code = $exitCode; remote_exit_code = $remoteProcess.ExitCode; transfer_error = $transferError; finished_at = [DateTimeOffset]::Now.ToString('o'); remote_script = $RemoteScript } | ConvertTo-Json), [Text.UTF8Encoding]::new($false))
[Console]::Out.Write($stdoutText)
[Console]::Error.Write($stderrText)
if ($transferError) { [Console]::Error.WriteLine('ACCEPTANCE_TRANSFER_FAIL: ' + $transferError) }
exit $exitCode
