"""Live integration test against a running API. Not auto-discovered (filename does
not match test*.py). Run explicitly:

    AETERNISLOG_BASE_URL=http://localhost:5001 \\
      python -m unittest tests.integration_test -v

Set AETERNISLOG_API_KEY too when the API has auth enabled.
"""
import os
import time
import unittest

from aeternislog import Client


@unittest.skipUnless(os.getenv("AETERNISLOG_BASE_URL"), "set AETERNISLOG_BASE_URL to run the live test")
class TestSDKLive(unittest.TestCase):
    def setUp(self):
        self.client = Client(os.environ["AETERNISLOG_BASE_URL"], api_key=os.getenv("AETERNISLOG_API_KEY", ""))
        self.domain = "pysdktest"

    def test_create_batch_verify(self):
        # create_record raises if the server hash != the locally computed hash, so a
        # pass here already proves Python<->Go canonicalization parity.
        rec = self.client.create_record(self.domain, "pysdk", {"foo": "bar", "n": 7})
        self.assertEqual(rec.hash, rec.compute_hash())

        # Fetch it back; its independently recomputed hash must match the stored one.
        got = self.client.get_record(self.domain, rec.id)
        self.assertEqual(got.compute_hash(), got.hash)

        # Batch + anchor on the blockchain, then verify server-side.
        time.sleep(0.2)
        batch = self.client.batch_records(self.domain)
        self.assertTrue(batch.anchored, batch)
        self.assertTrue(batch.tx_id, "expected a real transaction id")

        result = self.client.verify_batch(self.domain, batch.batch_id)
        self.assertTrue(result.is_valid, result)


if __name__ == "__main__":
    unittest.main()
