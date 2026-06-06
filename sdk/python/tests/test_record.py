import hashlib
import unittest

from anchor.record import Record, canonical, merkle_root, verify_records_locally


class TestCanonical(unittest.TestCase):
    def test_keys_sorted_and_compact(self):
        self.assertEqual(canonical({"b": 1, "a": "x"}), '{"a":"x","b":1}')

    def test_insertion_order_irrelevant(self):
        self.assertEqual(canonical({"a": 1, "b": 2}), canonical({"b": 2, "a": 1}))

    def test_go_html_escaping(self):
        # Go's encoding/json escapes < > & inside strings; Python's json does not.
        self.assertEqual(canonical({"x": "a<b>c&d"}), '{"x":"a\\u003cb\\u003ec\\u0026d"}')

    def test_integer_valued_float_has_no_decimal(self):
        # Server decodes numbers as float64; Go marshals 7.0 as "7".
        self.assertEqual(canonical({"n": 7.0}), '{"n":7}')
        self.assertEqual(canonical({"n": 7}), '{"n":7}')

    def test_nested_and_types(self):
        self.assertEqual(
            canonical({"o": {"k": True}, "a": [1, 2], "z": None}),
            '{"a":[1,2],"o":{"k":true},"z":null}',
        )


class TestRecordHash(unittest.TestCase):
    def test_compute_hash_matches_formula(self):
        rec = Record(id="r1", timestamp="2026-01-01T00:00:00Z", source="app", payload={"k": "v"})
        content = "r1" + "2026-01-01T00:00:00Z" + "app" + '{"k":"v"}'
        self.assertEqual(rec.compute_hash(), hashlib.sha256(content.encode()).hexdigest())

    def test_hash_is_stable_across_payload_order(self):
        a = Record(id="r", timestamp="t", source="s", payload={"x": 1, "y": 2})
        b = Record(id="r", timestamp="t", source="s", payload={"y": 2, "x": 1})
        self.assertEqual(a.compute_hash(), b.compute_hash())

    def test_hash_fields_restricts_payload(self):
        full = Record(id="r", timestamp="t", source="s", payload={"keep": 1, "ignore": 2})
        restricted = Record(
            id="r", timestamp="t", source="s",
            payload={"keep": 1, "ignore": 999}, hash_fields=["keep"],
        )
        self.assertEqual(full.compute_hash(), full.compute_hash())
        # Changing a non-hashed field must not change the restricted hash.
        same = Record(
            id="r", timestamp="t", source="s",
            payload={"keep": 1, "ignore": 2}, hash_fields=["keep"],
        )
        self.assertEqual(restricted.compute_hash(), same.compute_hash())

    def test_verify_detects_tamper(self):
        rec = Record(id="r", timestamp="t", source="s", payload={"k": "v"})
        rec.hash = rec.compute_hash()
        self.assertTrue(rec.verify())
        rec.payload["k"] = "tampered"
        self.assertFalse(rec.verify())


class TestMerkle(unittest.TestCase):
    def _h(self, rec):
        return rec.compute_hash()

    def test_single_record_root_is_its_hash(self):
        rec = Record(id="a", timestamp="t", source="s", payload={})
        self.assertEqual(merkle_root([rec]), rec.compute_hash())

    def test_two_records(self):
        r1 = Record(id="a", timestamp="t", source="s", payload={"n": 1})
        r2 = Record(id="b", timestamp="t", source="s", payload={"n": 2})
        expected = hashlib.sha256((r1.compute_hash() + r2.compute_hash()).encode()).hexdigest()
        self.assertEqual(merkle_root([r1, r2]), expected)

    def test_odd_count_duplicates_last(self):
        recs = [Record(id=str(i), timestamp="t", source="s", payload={"n": i}) for i in range(3)]
        h = [r.compute_hash() for r in recs]
        left = hashlib.sha256((h[0] + h[1]).encode()).hexdigest()
        right = hashlib.sha256((h[2] + h[2]).encode()).hexdigest()  # last duplicated
        expected = hashlib.sha256((left + right).encode()).hexdigest()
        self.assertEqual(merkle_root(recs), expected)

    def test_verify_records_locally(self):
        recs = [Record(id=str(i), timestamp="t", source="s", payload={"n": i}) for i in range(4)]
        root = merkle_root(recs)
        self.assertTrue(verify_records_locally(recs, root))
        recs[1].payload["n"] = 999
        self.assertFalse(verify_records_locally(recs, root))


if __name__ == "__main__":
    unittest.main()
