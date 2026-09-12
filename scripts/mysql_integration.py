#!/usr/bin/env python3
"""Discover, shard, run, and account for the Admin MySQL integration suite."""

from __future__ import annotations

import argparse
import json
import pathlib
import re
import subprocess
import sys
from collections import defaultdict
from typing import Dict, Iterable, List, Optional, Set, Tuple


SHARD_COUNT = 4
Identity = Tuple[str, str]


class RunnerError(RuntimeError):
    pass


def is_top_level_name(name: str) -> bool:
    return name.startswith(("Test", "Example", "Fuzz")) and not any(
        character.isspace() for character in name
    )


def discover_inventory(admin_root: pathlib.Path) -> Tuple[Set[Identity], str]:
    command = ["go", "test", "-tags=integration", "-list", ".", "-json", "./..."]
    process = subprocess.run(
        command,
        cwd=admin_root,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )
    if process.returncode != 0:
        detail = process.stderr.strip() or process.stdout.strip()
        raise RunnerError(f"integration inventory command failed ({process.returncode}):\n{detail}")

    inventory: Set[Identity] = set()
    duplicate: Set[Identity] = set()
    for number, line in enumerate(process.stdout.splitlines(), 1):
        try:
            event = json.loads(line)
        except json.JSONDecodeError as error:
            raise RunnerError(f"inventory line {number} is not JSON: {error}") from error
        if event.get("Action") != "output" or not event.get("Package"):
            continue
        output = event.get("Output", "")
        if not output.endswith("\n") or "\n" in output[:-1]:
            continue
        name = output[:-1]
        if not is_top_level_name(name):
            continue
        identity = (event["Package"], name)
        if identity in inventory:
            duplicate.add(identity)
        inventory.add(identity)

    if duplicate:
        raise RunnerError(f"inventory contains duplicate identities: {format_identities(duplicate)}")
    if not inventory:
        raise RunnerError("integration inventory is empty")
    return inventory, process.stdout


def assign_inventory(inventory: Set[Identity]) -> Dict[int, Set[Identity]]:
    assignments = {shard: set() for shard in range(1, SHARD_COUNT + 1)}
    for index, identity in enumerate(sorted(inventory)):
        assignments[index % SHARD_COUNT + 1].add(identity)

    assigned = set().union(*assignments.values())
    assigned_count = sum(len(items) for items in assignments.values())
    if assigned != inventory or assigned_count != len(inventory):
        raise RunnerError("shard assignment did not preserve the discovered inventory exactly once")
    if any(not items for items in assignments.values()):
        raise RunnerError("one or more integration shards are empty")
    return assignments


class ResultObserver:
    def __init__(self, package: str, selected: Set[str]) -> None:
        self.package = package
        self.selected = selected
        self.ran: Set[str] = set()
        self.passed: Set[str] = set()
        self.failed: Set[str] = set()
        self.skipped: Set[str] = set()
        self.extra: Set[str] = set()
        self.package_terminal: Optional[str] = None
        self.invalid_lines: List[str] = []

    def consume(self, line: str) -> None:
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            self.invalid_lines.append(line.rstrip())
            return

        if event.get("Package") != self.package:
            return
        action = event.get("Action")
        test = event.get("Test")
        if not test:
            if action in {"pass", "fail", "skip"}:
                self.package_terminal = action
            return

        top_level = test.split("/", 1)[0]
        if top_level not in self.selected:
            self.extra.add(top_level)
            return
        if action == "fail":
            self.failed.add(test)
        elif action == "skip":
            self.skipped.add(test)
        if test != top_level:
            return
        if action == "run":
            self.ran.add(test)
        elif action == "pass":
            self.passed.add(test)

    def verify(self, returncode: int) -> None:
        problems: List[str] = []
        if returncode != 0:
            problems.append(f"go test exited {returncode}")
        if self.package_terminal != "pass":
            problems.append(f"package terminal action is {self.package_terminal!r}")
        if self.invalid_lines:
            problems.append(f"non-JSON output: {self.invalid_lines[:3]!r}")
        missing_run = self.selected - self.ran
        missing_pass = self.selected - self.passed
        if missing_run:
            problems.append(f"never ran: {format_names(missing_run)}")
        if missing_pass:
            problems.append(f"did not pass: {format_names(missing_pass)}")
        if self.failed:
            problems.append(f"failed: {format_names(self.failed)}")
        if self.skipped:
            problems.append(f"skipped: {format_names(self.skipped)}")
        if self.extra:
            problems.append(f"unexpected top-level items ran: {format_names(self.extra)}")
        if problems:
            raise RunnerError(f"{self.package}: " + "; ".join(problems))


def exact_run_pattern(names: Iterable[str]) -> str:
    return "^(?:" + "|".join(re.escape(name) for name in sorted(names)) + ")$"


def run_selected(
    admin_root: pathlib.Path,
    selected_by_package: Dict[str, Set[str]],
    artifact_root: Optional[pathlib.Path],
    shard: Optional[int] = None,
) -> List[dict]:
    results: List[dict] = []
    failures: List[str] = []
    for package in sorted(selected_by_package):
        selected = selected_by_package[package]
        command = [
            "go",
            "test",
            "-p",
            "1",
            "-count=1",
            "-timeout=60m",
            "-tags=integration",
            "-json",
            package,
            "-run",
            exact_run_pattern(selected),
        ]
        observer = ResultObserver(package, selected)
        log_file = None
        if artifact_root is not None:
            safe_package = package.replace("/", "_")
            prefix = f"shard-{shard}-" if shard is not None else ""
            log_file = (artifact_root / f"{prefix}{safe_package}.jsonl").open("w")
        print(f"running {len(selected)} item(s) in {package}", flush=True)
        try:
            try:
                process = subprocess.Popen(
                    command,
                    cwd=admin_root,
                    stdout=subprocess.PIPE,
                    stderr=subprocess.STDOUT,
                    text=True,
                    bufsize=1,
                )
                assert process.stdout is not None
                for line in process.stdout:
                    sys.stdout.write(line)
                    sys.stdout.flush()
                    if log_file is not None:
                        log_file.write(line)
                        log_file.flush()
                    observer.consume(line)
                returncode = process.wait()
                process.stdout.close()
                observer.verify(returncode)
            except OSError as error:
                message = f"{package}: could not run go test: {error}"
                failures.append(message)
                results.append(
                    {"package": package, "selected": len(selected), "status": "fail", "error": message}
                )
            except RunnerError as error:
                failures.append(str(error))
                results.append(
                    {"package": package, "selected": len(selected), "status": "fail", "error": str(error)}
                )
            else:
                results.append({"package": package, "selected": len(selected), "status": "pass"})
        finally:
            if log_file is not None:
                log_file.close()

    if failures:
        raise RunnerError("\n".join(failures))
    return results


def group_by_package(identities: Set[Identity]) -> Dict[str, Set[str]]:
    grouped: Dict[str, Set[str]] = defaultdict(set)
    for package, name in identities:
        grouped[package].add(name)
    return dict(grouped)


def format_names(names: Iterable[str]) -> str:
    return ", ".join(sorted(names))


def format_identities(identities: Iterable[Identity]) -> str:
    return ", ".join(f"{package}:{name}" for package, name in sorted(identities))


def prepare_artifacts(path: Optional[str]) -> Optional[pathlib.Path]:
    if path is None:
        return None
    root = pathlib.Path(path)
    if root.exists() and any(root.iterdir()):
        raise RunnerError(f"artifact directory must be empty: {root}")
    root.mkdir(parents=True, exist_ok=True)
    return root


def write_assignment(artifact_root: pathlib.Path, assignments: Dict[int, Set[Identity]]) -> None:
    records = [
        {"shard": shard, "package": package, "name": name}
        for shard in sorted(assignments)
        for package, name in sorted(assignments[shard])
    ]
    (artifact_root / "assignment.json").write_text(
        json.dumps(
            {
                "shard_count": SHARD_COUNT,
                "total": len(records),
                "counts": {str(shard): len(assignments[shard]) for shard in sorted(assignments)},
                "items": records,
            },
            indent=2,
        )
        + "\n"
    )


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--shard", choices=["all", "1", "2", "3", "4"], default="all")
    parser.add_argument("--artifacts")
    parser.add_argument("--verify-only", action="store_true")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    repo_root = pathlib.Path(__file__).resolve().parents[1]
    admin_root = repo_root / "admin"
    artifact_root: Optional[pathlib.Path] = None
    summary = {"status": "fail", "shards": [], "errors": []}
    try:
        artifact_root = prepare_artifacts(args.artifacts)
        inventory, raw_inventory = discover_inventory(admin_root)
        assignments = assign_inventory(inventory)
        print(
            "discovered "
            f"{len(inventory)} top-level Test/Example/Fuzz item(s); "
            + ", ".join(f"shard {shard}={len(items)}" for shard, items in assignments.items()),
            flush=True,
        )
        if artifact_root is not None:
            (artifact_root / "inventory.jsonl").write_text(raw_inventory)
            write_assignment(artifact_root, assignments)
        if args.verify_only:
            summary["status"] = "pass"
            return 0

        shards = list(range(1, SHARD_COUNT + 1)) if args.shard == "all" else [int(args.shard)]
        errors: List[str] = []
        for shard in shards:
            print(f"starting shard {shard}/{SHARD_COUNT}", flush=True)
            try:
                run_selected(admin_root, group_by_package(assignments[shard]), artifact_root, shard)
                summary["shards"].append({"shard": shard, "selected": len(assignments[shard]), "status": "pass"})
            except RunnerError as error:
                errors.append(f"shard {shard}: {error}")
                summary["shards"].append(
                    {"shard": shard, "selected": len(assignments[shard]), "status": "fail", "error": str(error)}
                )
        if errors:
            summary["errors"] = errors
            raise RunnerError("\n".join(errors))
        summary["status"] = "pass"
        return 0
    except RunnerError as error:
        if not summary["errors"]:
            summary["errors"].append(str(error))
        print(f"mysql integration runner failed: {error}", file=sys.stderr)
        return 1
    finally:
        if artifact_root is not None:
            (artifact_root / "result.json").write_text(json.dumps(summary, indent=2) + "\n")


if __name__ == "__main__":
    sys.exit(main())
