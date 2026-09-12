# Run on the signing machine with a trusted code-signing certificate in its certificate store.
param(
  [Parameter(Mandatory=$true)][string]$CertificateThumbprint,
  [Parameter(Mandatory=$true)][string]$TimestampUrl,
  [Parameter(Mandatory=$true)][string[]]$Files,
  [string]$SignTool = "signtool.exe"
)
$ErrorActionPreference = "Stop"
foreach ($File in $Files) {
  if (!(Test-Path -LiteralPath $File -PathType Leaf)) { throw "Missing executable: $File" }
}
foreach ($File in $Files) {
  & $SignTool sign /sha1 $CertificateThumbprint /fd SHA256 /tr $TimestampUrl /td SHA256 $File
  if ($LASTEXITCODE -ne 0) { throw "Signing failed: $File" }
  & $SignTool verify /pa /all /v $File
  if ($LASTEXITCODE -ne 0) { throw "Signature verification failed: $File" }
}
Write-Output "All executable signatures verified. Recreate the ZIP and SHA256SUMS after signing."
