"""Atomic, secret-safe evidence store for architecture ablation studies."""

from __future__ import annotations

import hashlib
import json
import os
from pathlib import Path
import tempfile
from typing import Any

from quant_trend.artifacts import stable_json_hash


SECRET_KEYS = {
    "api_key",
    "apikey",
    "authorization",
    "credential",
    "credentials",
    "password",
    "secret",
    "token",
}


def _assert_secret_free(value: Any, path: str = "root") -> None:
    if isinstance(value, dict):
        for key, child in value.items():
            normalized = str(key).strip().lower()
            if normalized in SECRET_KEYS or normalized.endswith("_api_key"):
                raise ValueError(f"secret-like field is forbidden in evidence: {path}.{key}")
            _assert_secret_free(child, f"{path}.{key}")
    elif isinstance(value, (list, tuple)):
        for index, child in enumerate(value):
            _assert_secret_free(child, f"{path}[{index}]")


def _sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


class StudyArtifactStore:
    def __init__(self, root: str | Path, study_id: str) -> None:
        if not study_id or any(character not in "0123456789abcdef" for character in study_id):
            raise ValueError("study_id must be a lowercase hexadecimal hash")
        self.root = Path(root).expanduser().resolve() / study_id

    def path(self, relative: str | Path) -> Path:
        candidate = (self.root / relative).resolve()
        if self.root != candidate and self.root not in candidate.parents:
            raise ValueError("artifact path escapes study root")
        return candidate

    def load_json(self, relative: str | Path) -> dict[str, Any] | None:
        try:
            payload = json.loads(self.path(relative).read_text(encoding="utf-8"))
        except (FileNotFoundError, OSError, json.JSONDecodeError):
            return None
        return payload if isinstance(payload, dict) else None

    def write_json(self, relative: str | Path, payload: dict[str, Any]) -> Path:
        _assert_secret_free(payload)
        target = self.path(relative)
        target.parent.mkdir(parents=True, exist_ok=True)
        descriptor, temporary_name = tempfile.mkstemp(
            prefix=f".{target.name}.", suffix=".tmp", dir=target.parent
        )
        try:
            with os.fdopen(descriptor, "w", encoding="utf-8") as handle:
                json.dump(payload, handle, ensure_ascii=False, sort_keys=True, indent=2, default=str)
                handle.write("\n")
                handle.flush()
                os.fsync(handle.fileno())
            Path(temporary_name).replace(target)
        except Exception:
            Path(temporary_name).unlink(missing_ok=True)
            raise
        return target

    def write_immutable_json(self, relative: str | Path, payload: dict[str, Any]) -> Path:
        existing = self.load_json(relative)
        if existing is not None and stable_json_hash(existing) != stable_json_hash(payload):
            raise ValueError(f"immutable artifact already exists with different content: {relative}")
        return self.write_json(relative, payload)

    def write_text(self, relative: str | Path, content: str) -> Path:
        target = self.path(relative)
        target.parent.mkdir(parents=True, exist_ok=True)
        descriptor, temporary_name = tempfile.mkstemp(
            prefix=f".{target.name}.", suffix=".tmp", dir=target.parent
        )
        try:
            with os.fdopen(descriptor, "w", encoding="utf-8") as handle:
                handle.write(content)
                if not content.endswith("\n"):
                    handle.write("\n")
                handle.flush()
                os.fsync(handle.fileno())
            Path(temporary_name).replace(target)
        except Exception:
            Path(temporary_name).unlink(missing_ok=True)
            raise
        return target

    def finalize_manifest(self) -> dict[str, Any]:
        files = []
        if self.root.exists():
            for path in sorted(self.root.rglob("*")):
                if not path.is_file() or path.name == "manifest.json" or path.name.startswith("."):
                    continue
                files.append(
                    {
                        "path": str(path.relative_to(self.root)),
                        "bytes": path.stat().st_size,
                        "sha256": _sha256_file(path),
                    }
                )
        core = {
            "schema_version": "ta.ablation_manifest.v1",
            "study_id": self.root.name,
            "files": files,
        }
        manifest = {**core, "manifest_id": stable_json_hash(core)}
        self.write_json("manifest.json", manifest)
        return manifest
