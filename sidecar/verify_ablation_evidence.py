"""CLI for independent ablation design/study verification."""

from __future__ import annotations

import argparse
import json
from pathlib import Path

from ablation_study.artifacts import StudyArtifactStore
from ablation_study.verifier import verify_design, verify_study


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("kind", choices=("design", "study"))
    parser.add_argument("root", type=Path)
    parser.add_argument("--output-root", type=Path)
    args = parser.parse_args()
    result = verify_design(args.root) if args.kind == "design" else verify_study(args.root)
    output = dict(result)
    if args.output_root is not None:
        subject_id = str(result["design_id"] if args.kind == "design" else result["study_id"])
        store = StudyArtifactStore(args.output_root / args.kind, subject_id)
        relative = Path("reports") / f"{result['verification_id']}.json"
        path = store.write_immutable_json(relative, result)
        manifest = store.finalize_manifest()
        output["verification_artifact"] = str(path.resolve())
        output["verification_manifest_id"] = manifest["manifest_id"]
    print(json.dumps(output, ensure_ascii=False, indent=2))
    if not result["integrity_verified"]:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
