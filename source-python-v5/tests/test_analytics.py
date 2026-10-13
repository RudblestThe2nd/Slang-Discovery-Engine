import json
import tempfile
import unittest
from pathlib import Path
from analytics import build_report, candidate_profile, delete_old_posts
from slang_engine import db_open, ingest, detect

class AnalyticsTests(unittest.TestCase):
    def test_report_and_profiles(self):
        with tempfile.TemporaryDirectory() as td:
            db=db_open(Path(td)/'db.sqlite3')
            rows=[json.loads(line) for line in (Path(__file__).parents[1]/'data/demo.jsonl').read_text().splitlines()]
            ingest(db, rows)
            found=detect(db)
            self.assertTrue(found)
            term,kind=found[0][0],found[0][1]
            profile=candidate_profile(db,term,kind)
            self.assertEqual(profile['evidence_count'],found[0][3])
            self.assertIn('communities',profile)
            report=Path(td)/'report.md'
            self.assertGreater(build_report(db,report),0)
            self.assertIn('Highest-ranked',report.read_text())
            details=json.loads(report.with_suffix('.json').read_text())
            self.assertEqual(details['documents'],len(rows))
            db.close()

    def test_retention_purges_old_sources_and_scores(self):
        with tempfile.TemporaryDirectory() as td:
            db=db_open(Path(td)/'db.sqlite3')
            sample={'id':'x1','body':'We are test driving content','author_hash':'a', 'created_utc':1000000000,'subreddit':'test'}
            ingest(db,[sample])
            db.execute('UPDATE posts SET collected_utc=1000000000 WHERE id="x1"')
            db.commit()
            self.assertEqual(delete_old_posts(db,90,now=2000000000),1)
            self.assertEqual(db.execute('SELECT count(*) FROM posts').fetchone()[0],0)
            with self.assertRaises(ValueError):delete_old_posts(db,0)
            db.close()
if __name__=='__main__':unittest.main()
