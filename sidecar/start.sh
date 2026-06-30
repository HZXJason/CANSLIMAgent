#!/bin/bash
set -e
cd "$(dirname "$0")"
if [ ! -d ".venv" ]; then
  python3 -m venv .venv
fi
source .venv/bin/activate
pip install -r requirements.txt
uvicorn main:app --host "${SIDECAR_HOST:-127.0.0.1}" --port "${SIDECAR_PORT:-8765}"
