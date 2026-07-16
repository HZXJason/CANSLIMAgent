"""Content-addressed snapshots and run cards for reproducible experiments."""

from __future__ import annotations

import hashlib
import json
import os
from pathlib import Path
import tempfile
from typing import Any

import pandas as pd


def stable_json_bytes(value: Any) -> bytes:
    return json.dumps(
        value,
        ensure_ascii=False,
        sort_keys=True,
        separators=(",", ":"),
        default=str,
    ).encode("utf-8")


def stable_json_hash(value: Any) -> str:
    return hashlib.sha256(stable_json_bytes(value)).hexdigest()


def frame_content_hash(frame: pd.DataFrame) -> str:
    """Hash canonical values and index without relying on CSV formatting."""

    digest = hashlib.sha256()
    digest.update("|".join(str(column) for column in frame.columns).encode("utf-8"))
    hashed = pd.util.hash_pandas_object(frame, index=True, categorize=False)
    digest.update(hashed.to_numpy().tobytes())
    return digest.hexdigest()


class QuantArtifactStore:
    """Small atomic JSON store; production DB persistence can replace this boundary."""

    def __init__(self, root: str | Path | None = None) -> None:
        configured = root or os.getenv("QUANT_TREND_ARTIFACT_DIR", "")
        self.root = Path(configured) if configured else Path(__file__).resolve().parents[1] / "data" / "quant-trend"

    def _path(self, category: str, identifier: str) -> Path:
        safe = "".join(character for character in identifier if character.isalnum() or character in "-_")
        if not safe:
            raise ValueError("artifact identifier is empty")
        return self.root / category / f"{safe}.json"

    def load_snapshot(self, request_hash: str) -> dict[str, Any] | None:
        path = self._path("snapshot-requests", request_hash)
        try:
            payload = json.loads(path.read_text(encoding="utf-8"))
        except (FileNotFoundError, OSError, json.JSONDecodeError):
            return None
        return payload if isinstance(payload, dict) and payload.get("schema_version") == "quant.trend_snapshot.v1" else None

    def save_snapshot(self, request_hash: str, snapshot: dict[str, Any]) -> Path:
        if snapshot.get("schema_version") != "quant.trend_snapshot.v1":
            raise ValueError("a quant.trend_snapshot.v1 record is required")
        identifier = str(snapshot.get("snapshot_id") or "")
        if identifier != stable_json_hash(
            {key: value for key, value in snapshot.items() if key != "snapshot_id"}
        ):
            raise ValueError("snapshot content does not match snapshot_id")
        content_path = self._atomic_write(self._path("snapshots", identifier), snapshot)
        # The mutable request cache may point to a refreshed vendor revision;
        # the content-addressed copy above keeps every prior experiment input.
        self._atomic_write(self._path("snapshot-requests", request_hash), snapshot)
        return content_path

    def load_snapshot_by_id(self, snapshot_id: str) -> dict[str, Any] | None:
        path = self._path("snapshots", snapshot_id)
        try:
            payload = json.loads(path.read_text(encoding="utf-8"))
        except (FileNotFoundError, OSError, json.JSONDecodeError):
            return None
        if not isinstance(payload, dict) or payload.get("schema_version") != "quant.trend_snapshot.v1":
            return None
        expected = stable_json_hash({key: value for key, value in payload.items() if key != "snapshot_id"})
        return payload if payload.get("snapshot_id") == expected == snapshot_id else None

    def save_evaluation(self, evaluation: dict[str, Any]) -> Path:
        identifier = str(evaluation.get("run_id") or stable_json_hash(evaluation))
        return self._atomic_write(self._path("evaluations", identifier), evaluation)

    def save_prediction(self, prediction: dict[str, Any]) -> Path:
        if prediction.get("schema_version") != "quant.ablation_prediction.v1":
            raise ValueError("a quant.ablation_prediction.v1 record is required")
        identifier = str(prediction.get("prediction_id") or "")
        if identifier != stable_json_hash(
            {key: value for key, value in prediction.items() if key != "prediction_id"}
        ):
            raise ValueError("prediction content does not match prediction_id")
        return self._atomic_write(self._path("predictions", identifier), prediction)

    def save_outcome(self, outcome: dict[str, Any]) -> Path:
        if outcome.get("schema_version") != "quant.ablation_outcome.v1":
            raise ValueError("a quant.ablation_outcome.v1 record is required")
        identifier = str(outcome.get("outcome_id") or "")
        if identifier != stable_json_hash(
            {key: value for key, value in outcome.items() if key != "outcome_id"}
        ):
            raise ValueError("outcome content does not match outcome_id")
        return self._atomic_write(self._path("outcomes", identifier), outcome)

    def save_experiment_manifest(self, manifest: dict[str, Any]) -> Path:
        if manifest.get("schema_version") != "quant.ablation_manifest.v1":
            raise ValueError("a quant.ablation_manifest.v1 record is required")
        identifier = str(manifest.get("manifest_id") or "")
        if identifier != stable_json_hash(
            {key: value for key, value in manifest.items() if key != "manifest_id"}
        ):
            raise ValueError("manifest content does not match manifest_id")
        return self._atomic_write(self._path("experiment-manifests", identifier), manifest)

    @staticmethod
    def _atomic_write(path: Path, payload: dict[str, Any]) -> Path:
        path.parent.mkdir(parents=True, exist_ok=True)
        descriptor, temporary_name = tempfile.mkstemp(prefix=f".{path.name}.", suffix=".tmp", dir=path.parent)
        try:
            with os.fdopen(descriptor, "w", encoding="utf-8") as handle:
                json.dump(payload, handle, ensure_ascii=False, sort_keys=True, indent=2, default=str)
                handle.flush()
                os.fsync(handle.fileno())
            Path(temporary_name).replace(path)
        except Exception:
            Path(temporary_name).unlink(missing_ok=True)
            raise
        return path
