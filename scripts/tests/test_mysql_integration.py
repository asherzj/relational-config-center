import importlib.util
import json
import os
import pathlib
import tempfile
import textwrap
import unittest


SCRIPT = pathlib.Path(__file__).parents[1] / "mysql_integration.py"
SPEC = importlib.util.spec_from_file_location("mysql_integration", SCRIPT)
mysql_integration = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(mysql_integration)


class ResultObserverTest(unittest.TestCase):
    def test_assignment_is_balanced_and_preserves_every_identity_once(self):
        inventory = {("p", f"Test{index:03d}") for index in range(431)}
        assignments = mysql_integration.assign_inventory(inventory)

        self.assertEqual([len(assignments[index]) for index in range(1, 5)], [108, 108, 108, 107])
        self.assertEqual(set().union(*assignments.values()), inventory)
        self.assertEqual(sum(map(len, assignments.values())), len(inventory))

    def test_accepts_subtests_but_requires_top_level_pass(self):
        observer = mysql_integration.ResultObserver(
            "example.com/fixture/a", {"TestNested"}
        )
        for event in (
            {"Action": "run", "Package": "example.com/fixture/a", "Test": "TestNested"},
            {
                "Action": "run",
                "Package": "example.com/fixture/a",
                "Test": "TestNested/child",
            },
            {
                "Action": "pass",
                "Package": "example.com/fixture/a",
                "Test": "TestNested/child",
            },
            {"Action": "pass", "Package": "example.com/fixture/a", "Test": "TestNested"},
            {"Action": "pass", "Package": "example.com/fixture/a"},
        ):
            observer.consume(json.dumps(event))

        observer.verify(0)

    def test_rejects_skip_missing_extra_and_process_failure(self):
        cases = (
            ([{"Action": "skip", "Package": "p", "Test": "TestWanted"}], 0),
            ([{"Action": "pass", "Package": "p"}], 0),
            (
                [
                    {"Action": "run", "Package": "p", "Test": "TestOther"},
                    {"Action": "pass", "Package": "p", "Test": "TestOther"},
                    {"Action": "pass", "Package": "p"},
                ],
                0,
            ),
            (
                [
                    {"Action": "run", "Package": "p", "Test": "TestWanted"},
                    {"Action": "pass", "Package": "p", "Test": "TestWanted"},
                    {"Action": "pass", "Package": "p"},
                ],
                2,
            ),
        )
        for events, returncode in cases:
            with self.subTest(events=events, returncode=returncode):
                observer = mysql_integration.ResultObserver("p", {"TestWanted"})
                for event in events:
                    observer.consume(json.dumps(event))
                with self.assertRaises(mysql_integration.RunnerError):
                    observer.verify(returncode)

    def test_rejects_skipped_or_failed_child_and_reports_its_full_path(self):
        for terminal in ("skip", "fail"):
            with self.subTest(terminal=terminal):
                observer = mysql_integration.ResultObserver("p", {"TestWanted"})
                for event in (
                    {"Action": "run", "Package": "p", "Test": "TestWanted"},
                    {"Action": "run", "Package": "p", "Test": "TestWanted/unsupported"},
                    {"Action": terminal, "Package": "p", "Test": "TestWanted/unsupported"},
                    {"Action": "pass", "Package": "p", "Test": "TestWanted"},
                    {"Action": "pass", "Package": "p"},
                ):
                    observer.consume(json.dumps(event))

                with self.assertRaisesRegex(mysql_integration.RunnerError, "TestWanted/unsupported"):
                    observer.verify(0)


class GoRunnerEndToEndTest(unittest.TestCase):
    def test_discovers_and_runs_exact_top_level_items_across_packages(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = pathlib.Path(temporary)
            (root / "go.mod").write_text("module example.com/fixture\n\ngo 1.24\n")
            for package in ("a", "b"):
                (root / package).mkdir()
            (root / "a" / "a_test.go").write_text(
                textwrap.dedent(
                    """
                    package a

                    import (
                        "fmt"
                        "testing"
                    )

                    func TestShared(t *testing.T) {}
                    func Test容量(t *testing.T) {}
                    func TestNested(t *testing.T) { t.Run("child", func(t *testing.T) {}) }
                    func FuzzEcho(f *testing.F) {
                        f.Add("seed")
                        f.Fuzz(func(t *testing.T, value string) {})
                    }
                    func Example() {
                        fmt.Println("ok")
                        // Output: ok
                    }
                    """
                )
            )
            (root / "b" / "b_test.go").write_text(
                textwrap.dedent(
                    """
                    package b

                    import "testing"

                    func TestShared(t *testing.T) {}
                    """
                )
            )
            prior_cache = os.environ.get("GOCACHE")
            os.environ["GOCACHE"] = str(root / "gocache")
            try:
                inventory, _ = mysql_integration.discover_inventory(root)
                self.assertEqual(
                    inventory,
                    {
                        ("example.com/fixture/a", "Example"),
                        ("example.com/fixture/a", "FuzzEcho"),
                        ("example.com/fixture/a", "TestNested"),
                        ("example.com/fixture/a", "TestShared"),
                        ("example.com/fixture/a", "Test容量"),
                        ("example.com/fixture/b", "TestShared"),
                    },
                )
                selected = {}
                for package, name in inventory:
                    selected.setdefault(package, set()).add(name)
                mysql_integration.run_selected(root, selected, None)

                for package, body in (
                    ("c", 'package c\nimport "testing"\nfunc TestFails(t *testing.T) { t.Fatal("expected") }\n'),
                    ("d", 'package d\nimport "testing"\nfunc TestStillRuns(t *testing.T) {}\n'),
                ):
                    (root / package).mkdir()
                    (root / package / f"{package}_test.go").write_text(body)
                evidence = root / "evidence"
                evidence.mkdir()
                with self.assertRaises(mysql_integration.RunnerError):
                    mysql_integration.run_selected(
                        root,
                        {
                            "example.com/fixture/c": {"TestFails"},
                            "example.com/fixture/d": {"TestStillRuns"},
                        },
                        evidence,
                        1,
                    )
                logs = {path.name for path in evidence.iterdir()}
                self.assertIn("shard-1-example.com_fixture_c.jsonl", logs)
                self.assertIn("shard-1-example.com_fixture_d.jsonl", logs)
            finally:
                if prior_cache is None:
                    os.environ.pop("GOCACHE", None)
                else:
                    os.environ["GOCACHE"] = prior_cache


if __name__ == "__main__":
    unittest.main()
