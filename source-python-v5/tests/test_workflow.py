import json
import sqlite3
import tempfile
import unittest
from pathlib import Path
from slang_engine import db_open,ingest,detect,review,evidence_for,stats,export_json

class WorkflowTests(unittest.TestCase):
    def test_review_evidence_export(self):
        with tempfile.TemporaryDirectory() as td:
            db=db_open(Path(td)/'test.db')
            demo=Path(__file__).parents[1]/'data/demo.jsonl'
            records=[json.loads(s) for s in demo.read_text(encoding='utf-8').splitlines()]
            self.assertEqual(ingest(db,records),len(records))
            found=detect(db,min_mentions=3,min_authors=2,drift_min=4)
            self.assertTrue(found)
            term,kind,*_=found[0]
            self.assertGreaterEqual(len(evidence_for(db,term,kind)),1)
            self.assertEqual(review(db,term,kind,'accepted'),1)
            self.assertEqual(stats(db)['statuses'].get('accepted'),1)
            detect(db,min_mentions=3,min_authors=2,drift_min=4)
            self.assertEqual(db.execute('SELECT review_status FROM candidates WHERE term=? AND kind=?',(term,kind)).fetchone()[0],'accepted')
            dest=Path(td)/'out.json'
            self.assertGreater(export_json(db,dest),0)
            self.assertIsInstance(json.loads(dest.read_text()),list)
            with self.assertRaises(ValueError):review(db,term,kind,'garbage')
            db.close()
    def test_empty_database(self):
        with tempfile.TemporaryDirectory() as td:
            db=db_open(Path(td)/'empty.db')
            self.assertEqual(detect(db),[])
            self.assertEqual(stats(db)['posts'],0)
            db.close()

if __name__=='__main__':unittest.main()
