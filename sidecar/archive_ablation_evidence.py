"""Archive completed graph checkpoints and frozen sources into a study.

This post-run evidence operation is intentionally outside ``ablation_study``:
it must not change the preregistered runner/source fingerprint. It is safe to
rerun while a study is in progress; only records already marked complete are
archived.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import sqlite3
import tempfile
from typing import Any

from ablation_study.artifacts import StudyArtifactStore
from quant_trend.artifacts import stable_json_hash


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def atomic_copy(source: Path, target: Path) -> None:
    target.parent.mkdir(parents=True, exist_ok=True)
    descriptor, temporary_name = tempfile.mkstemp(
        prefix=f".{target.name}.", suffix=".tmp", dir=target.parent
    )
    os.close(descriptor)
    try:
        shutil.copy2(source, temporary_name)
        Path(temporary_name).replace(target)
    except Exception:
        Path(temporary_name).unlink(missing_ok=True)
        raise


def atomic_sqlite_backup(source: Path, target: Path) -> None:
    """Create one transactionally consistent DB file without copying WAL/SHM."""
    target.parent.mkdir(parents=True, exist_ok=True)
    for stale in target.parent.glob(f".{target.name}.*.tmp-*"):
        stale.unlink(missing_ok=True)
    descriptor, temporary_name = tempfile.mkstemp(
        prefix=f".{target.name}.", suffix=".tmp", dir=target.parent
    )
    os.close(descriptor)
    temporary = Path(temporary_name)
    try:
        with sqlite3.connect(str(source)) as source_db, sqlite3.connect(
            str(temporary)
        ) as snapshot_db:
            source_db.backup(snapshot_db)
            snapshot_db.execute("PRAGMA journal_mode=DELETE")
            result = snapshot_db.execute("PRAGMA integrity_check").fetchone()
            if not result or result[0] != "ok":
                raise ValueError(f"SQLite integrity check failed: {source}")
        target.with_name(target.name + "-wal").unlink(missing_ok=True)
        target.with_name(target.name + "-shm").unlink(missing_ok=True)
        temporary.replace(target)
    except Exception:
        temporary.unlink(missing_ok=True)
        raise
    finally:
        temporary.with_name(temporary.name + "-wal").unlink(missing_ok=True)
        temporary.with_name(temporary.name + "-shm").unlink(missing_ok=True)


def archive(study_root: Path, ai_runs_root: Path, repo_root: Path) -> dict[str, Any]:
    protocol = json.loads((study_root / "protocol.json").read_text(encoding="utf-8"))
    store = StudyArtifactStore(study_root.parent, protocol["study_id"])
    checkpoint_entries = []
    for record_path in sorted((study_root / "runs").glob("*/*/*.json")):
        record = json.loads(record_path.read_text(encoding="utf-8"))
        if record.get("status") != "complete":
            continue
        run_id = str((record.get("request_config") or {}).get("run_id") or "")
        run_key = str(record.get("run_key") or "")
        if not run_id.startswith("abl-") or len(run_key) != 64:
            raise ValueError(f"invalid completed run identity in {record_path}")
        source_dir = ai_runs_root / run_id / "checkpoints"
        files = []
        for source in sorted(source_dir.glob("*")):
            if not source.is_file() or source.name.endswith(("-wal", "-shm")):
                continue
            relative = Path("archived-checkpoints") / run_key / source.name
            target = store.path(relative)
            if source.suffix == ".db":
                atomic_sqlite_backup(source, target)
            else:
                atomic_copy(source, target)
            files.append(
                {
                    "path": str(relative),
                    "bytes": target.stat().st_size,
                    "sha256": sha256_file(target),
                }
            )
        if not files:
            raise FileNotFoundError(f"completed run checkpoint is missing: {run_id}")
        checkpoint_entries.append(
            {
                "run_key": run_key,
                "run_id": run_id,
                "record_path": str(record_path.relative_to(study_root)),
                "files": files,
            }
        )

    source_entries = []
    for source_record in protocol["source"]["files"]:
        relative_source = Path(source_record["path"])
        source = (repo_root / relative_source).resolve()
        if repo_root.resolve() not in source.parents or not source.is_file():
            raise ValueError(f"frozen source is missing or escapes repository: {relative_source}")
        content = source.read_text(encoding="utf-8")
        if stable_json_hash(content) != source_record["sha256"]:
            archived_source = store.path(Path("source-snapshot") / relative_source)
            if not archived_source.is_file() or stable_json_hash(
                archived_source.read_text(encoding="utf-8")
            ) != source_record["sha256"]:
                raise ValueError(f"source changed after protocol registration: {relative_source}")
            source = archived_source
        relative_target = Path("source-snapshot") / relative_source
        target = store.path(relative_target)
        atomic_copy(source, target)
        source_entries.append(
            {
                "path": str(relative_target),
                "bytes": target.stat().st_size,
                "sha256": sha256_file(target),
                "protocol_content_hash": source_record["sha256"],
            }
        )

    narrative = repo_root / "sidecar" / "ablation_study" / "PROTOCOL.md"
    if narrative.is_file():
        target = store.path("source-snapshot/sidecar/ablation_study/PROTOCOL.md")
        atomic_copy(narrative, target)
        source_entries.append(
            {
                "path": str(target.relative_to(study_root)),
                "bytes": target.stat().st_size,
                "sha256": sha256_file(target),
            }
        )

    core = {
        "schema_version": "ta.ablation_evidence_archive.v1",
        "study_id": protocol["study_id"],
        "completed_runs_archived": len(checkpoint_entries),
        "checkpoints": checkpoint_entries,
        "sources": source_entries,
    }
    index = {**core, "archive_id": stable_json_hash(core)}
    store.write_json("evidence_archive.json", index)
    manifest = store.finalize_manifest()
    return {**index, "manifest_id": manifest["manifest_id"]}


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("study_root", type=Path)
    parser.add_argument(
        "--ai-runs-root",
        type=Path,
        default=Path(__file__).resolve().parent / "data" / "ai-runs",
    )
    parser.add_argument("--repo-root", type=Path, default=Path(__file__).resolve().parent.parent)
    args = parser.parse_args()
    result = archive(args.study_root.resolve(), args.ai_runs_root.resolve(), args.repo_root.resolve())
    print(json.dumps({
        "study_id": result["study_id"],
        "completed_runs_archived": result["completed_runs_archived"],
        "archive_id": result["archive_id"],
        "manifest_id": result["manifest_id"],
    }, ensure_ascii=False))


if __name__ == "__main__":
    main()
