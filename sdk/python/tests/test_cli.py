import contextlib
import io
import os
import tempfile
import unittest

from aeternislog import cli
from aeternislog.record import Record, merkle_root


def _write_csv(rows):
    fd, path = tempfile.mkstemp(suffix=".csv")
    with os.fdopen(fd, "w", encoding="utf-8") as f:
        f.write("id,timestamp,source,payload\n")
        for r in rows:
            f.write(f'{r[0]},{r[1]},{r[2]},"{r[3]}"\n')
    return path


class TestCLI(unittest.TestCase):
    def setUp(self):
        self.rows = [
            ("a", "2026-01-01T00:00:00Z", "app", '{""k"":1}'),  # CSV-escaped quotes
            ("b", "2026-01-01T00:00:01Z", "app", '{""k"":2}'),
        ]
        self.path = _write_csv(self.rows)
        self.addCleanup(os.remove, self.path)
        # Expected root computed via the library itself.
        recs = [
            Record(id="a", timestamp="2026-01-01T00:00:00Z", source="app", payload={"k": 1}),
            Record(id="b", timestamp="2026-01-01T00:00:01Z", source="app", payload={"k": 2}),
        ]
        self.root = merkle_root(recs)

    def _run(self, argv):
        out = io.StringIO()
        with contextlib.redirect_stdout(out):
            rc = cli.main(argv)
        return rc, out.getvalue()

    def test_merkle_prints_root(self):
        rc, out = self._run(["merkle", "--file", self.path])
        self.assertEqual(rc, 0)
        self.assertEqual(out.strip(), self.root)

    def test_verify_valid_exit_0(self):
        rc, out = self._run(["verify", "--file", self.path, "--expected-root", self.root])
        self.assertEqual(rc, 0)
        self.assertIn("VALID", out)

    def test_verify_corrupted_exit_2(self):
        rc, out = self._run(["verify", "--file", self.path, "--expected-root", "wrongroot"])
        self.assertEqual(rc, 2)
        self.assertIn("CORRUPTED", out)

    def test_hash_single_record(self):
        rc, out = self._run([
            "hash", "--id", "a", "--timestamp", "2026-01-01T00:00:00Z",
            "--source", "app", "--payload", '{"k":1}',
        ])
        self.assertEqual(rc, 0)
        expected = Record(id="a", timestamp="2026-01-01T00:00:00Z", source="app", payload={"k": 1}).compute_hash()
        self.assertEqual(out.strip(), expected)


if __name__ == "__main__":
    unittest.main()
