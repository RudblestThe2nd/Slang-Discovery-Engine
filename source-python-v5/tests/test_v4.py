import json, os, tempfile, unittest
from pathlib import Path
from slang_engine import db_open, ingest
from intelligence import migrate, record_review, add_sense, local_context, evaluate_predictions, cluster_senses

class V4(unittest.TestCase):
 def setUp(self):
  self.tmp=tempfile.TemporaryDirectory();self.db=db_open(Path(self.tmp.name)/'db.sqlite')
 def tearDown(self):self.db.close();self.tmp.cleanup()
 def test_migration_idempotent(self):
  migrate(self.db);migrate(self.db)
  self.assertTrue(self.db.execute("SELECT name FROM sqlite_master WHERE name='senses'").fetchone())
 def test_context_multiple(self):
  self.assertEqual(len(local_context('cooked and very cooked','cooked')),2)
 def test_sense_evidence_and_review(self):
  ingest(self.db,[dict(id='a',body='That is totally cooked today',author_hash='xyz',created_utc=2000,subreddit='test')])
  self.db.execute("INSERT INTO candidates VALUES ('cooked','context_drift',55,1,1,'{}',1,'pending')")
  self.assertEqual(record_review(self.db,'cooked','context_drift','accepted','verified manually'),1)
  self.assertEqual(self.db.execute('SELECT COUNT(*) FROM review_events').fetchone()[0],1)
  sid=add_sense(self.db,'cooked','in trouble',['a'])
  self.assertEqual(self.db.execute('SELECT COUNT(*) FROM sense_evidence WHERE sense_id=?',(sid,)).fetchone()[0],1)
 def test_gold_metric(self):
  self.db.execute("INSERT INTO candidates VALUES ('blorp','lexical_novelty',90,5,3,'{}',1,'pending')")
  f=Path(self.tmp.name)/'labels.jsonl';f.write_text(json.dumps({'term':'blorp','kind':'lexical_novelty','is_slang':True})+'\n')
  self.assertEqual(evaluate_predictions(self.db,f)['precision_at_k_labeled_only'],1.)
 def test_cluster_insufficient(self):
  self.assertEqual(cluster_senses(self.db,'cooked')['status'],'insufficient_evidence')

if __name__=='__main__':unittest.main()
