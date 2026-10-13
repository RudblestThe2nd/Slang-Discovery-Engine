import json
import sys
import tempfile
import types
import unittest
from pathlib import Path
from unittest.mock import patch
from slang_engine import db_open,ingest,detect

class LexicalTests(unittest.TestCase):
    def test_wordfreq_candidate_and_phrase(self):
        with tempfile.TemporaryDirectory() as td:
            db=db_open(Path(td)/'test.db')
            rows=[json.loads(s) for s in (Path(__file__).parents[1]/'data/demo.jsonl').read_text().splitlines()]
            ingest(db,rows)
            mock=types.ModuleType('wordfreq')
            mock.zipf_frequency=lambda word,language: 0.0 if word=='glorpmaxxing' else 5.0
            with patch.dict(sys.modules,{'wordfreq':mock}):
                found=detect(db,3,2,4)
            keys={(item[0],item[1]) for item in found}
            self.assertIn(('glorpmaxxing','lexical_novelty'),keys)
            self.assertTrue(any(kind=='phrase_candidate' and 'glorpmaxxing' in term for term,kind in keys))
            db.close()

if __name__=='__main__':unittest.main()
