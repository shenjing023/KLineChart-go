param(
    [Parameter(Mandatory = $true)]
    [string]$RepositoryRoot
)

$ErrorActionPreference = 'Stop'
$reviewRoot = (Resolve-Path -LiteralPath $RepositoryRoot).Path
if (-not (Test-Path -LiteralPath (Join-Path $reviewRoot 'go.mod') -PathType Leaf)) {
    throw 'RepositoryRoot must be the KLineChart-go repository root.'
}
$probeRoot = Join-Path ([IO.Path]::GetTempPath()) ('kline-review-reproduce-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $probeRoot | Out-Null

$sourceFiles = git -C $reviewRoot ls-files --cached --others --exclude-standard
if ($LASTEXITCODE -ne 0) { throw 'Cannot list repository files.' }
foreach ($relative in ($sourceFiles | Sort-Object -Unique)) {
    if ($relative -notmatch '\.go$|^go\.(mod|sum)$') { continue }
    $source = Join-Path $reviewRoot $relative
    if (-not (Test-Path -LiteralPath $source -PathType Leaf)) { continue }
    $destination = Join-Path $probeRoot $relative
    New-Item -ItemType Directory -Path (Split-Path -Parent $destination) -Force | Out-Null
    Copy-Item -LiteralPath $source -Destination $destination
}
foreach ($probe in (Get-ChildItem -LiteralPath $PSScriptRoot -Filter '*_test.go.txt')) {
    Copy-Item -LiteralPath $probe.FullName -Destination (Join-Path $probeRoot ('kline/' + $probe.Name.Substring(0, $probe.Name.Length - 4)))
}

$testPattern = '^TestReview(StartReplayDuringOnBarPreservesNewCursor|CloseDuringOnBarStopsAdvance|PauseDuringOnBarStopsTick|CoarseReplayHidesFutureMarkers|PeriodSwitchReindexesLastBarWithNewPeriod|InitHeldCap|CompareFallbackNeedsBothSeries|NilLinePlotCallsDrawMain|SessionGapMisplacesTimeTick|MarkersMustStayInsidePricePlot|SecondMACDActuallyPaints|BuiltinIndicatorsStayReserved|FiniteIndicatorTails)$'
Write-Output ('Isolated source and output: ' + $probeRoot)
Push-Location -LiteralPath $probeRoot
try {
    go test ./kline -run $testPattern -count=1 -v 2>&1 | Tee-Object -FilePath (Join-Path $probeRoot 'review-results.log')
    $reviewExitCode = $LASTEXITCODE
} finally {
    Pop-Location
}
Write-Output ('Go exit code: ' + $reviewExitCode)
Write-Output 'This reviewed version is expected to fail the defect assertions; original repository files were not changed.'
exit $reviewExitCode
