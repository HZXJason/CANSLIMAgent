#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")"

if command -v uv >/dev/null 2>&1; then
  export UV_HTTP_TIMEOUT="${UV_HTTP_TIMEOUT:-300}"
  UV_PYTHON_VERSION="${UV_PYTHON_VERSION:-$(cat .python-version 2>/dev/null || echo 3.12)}"
  if [ ! -x ".venv/bin/python" ]; then
    uv venv --python "$UV_PYTHON_VERSION" .venv
  elif ! .venv/bin/python - <<'PY'
import sys

raise SystemExit(0 if sys.version_info >= (3, 10) else 1)
PY
  then
    uv venv --clear --python "$UV_PYTHON_VERSION" .venv
  fi
  uv pip install --python .venv/bin/python -r requirements.txt
else
  python3 - <<'PY'
import sys

if sys.version_info < (3, 10):
    raise SystemExit("Python 3.10+ is required for the sidecar")
PY
  if [ ! -d ".venv" ]; then
    python3 -m venv .venv
  fi
  source .venv/bin/activate
  python -m ensurepip --upgrade >/dev/null 2>&1 || true
  python -m pip install -r requirements.txt
fi

source .venv/bin/activate
exec uvicorn main:app --host "${SIDECAR_HOST:-127.0.0.1}" --port "${SIDECAR_PORT:-8765}"
