$ErrorActionPreference = "Stop"

Set-Location $PSScriptRoot

if (-not (Test-Path ".venv")) {
  python -m venv .venv
}

& ".\.venv\Scripts\Activate.ps1"
python -m pip install -r requirements.txt

$sidecarHost = if ($env:SIDECAR_HOST) { $env:SIDECAR_HOST } else { "127.0.0.1" }
$sidecarPort = if ($env:SIDECAR_PORT) { $env:SIDECAR_PORT } else { "8765" }

python -m uvicorn main:app --host $sidecarHost --port $sidecarPort
