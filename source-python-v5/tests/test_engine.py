import json
import sqlite3
import tempfile
import unittest
from pathlib import Path
from slang_engine import db_open,ingest,detect,export
class TestEngine(unittest.TestCase):
 def test_demo(self):
  with tempfile.TemporaryDirectory() as td:
   db=db_open(Path(td)/'test.db')
   path=Path(__file__).parents[1]/'data/demo.jsonl'
   rows=[json.loads(line) for line in path.read_text().splitlines()]
   self.assertEqual(ingest(db,rows),len(rows))
   self.assertEqual(ingest(db,rows),0)
   terms=detect(db,3,2,4)
   try:
    import wordfreq
    self.assertIn(('glorpmaxxing','lexical_novelty'),{(r[0],r[1]) for r in terms})
   except ImportError:
    self.assertFalse(any(r[1]=='lexical_novelty' for r in terms))
   self.assertTrue(any(r[0]=='cooked' and r[1]=='context_drift' for r in terms))
   self.assertGreater(export(db,Path(td)/'results.csv'),0)
if __name__=='__main__': unittest.main()
