import json, tempfile, unittest
from pathlib import Path
from datetime import datetime, timezone
from slang_engine import db_open, ingest
from trends import week_of, timeline, export_trends

class TrendsTest(unittest.TestCase):
    def setUp(self):
        self.t=tempfile.TemporaryDirectory();self.db=db_open(Path(self.t.name)/'test.sqlite')
    def tearDown(self):self.db.close();self.t.cleanup()
    def test_week_iso(self):
        self.assertEqual(week_of(int(datetime(2026,1,1,tzinfo=timezone.utc).timestamp())),'2026-W01')
    def test_counts_and_distinct_authors(self):
        week1=int(datetime(2026,9,7,tzinfo=timezone.utc).timestamp())
        week2=int(datetime(2026,9,14,tzinfo=timezone.utc).timestamp())
        ingest(self.db,[{'id':str(i),'created_utc':week1 if i<2 else week2,
             'body':b,'author_hash':f'author{i%3}','subreddit':'genz'} for i,b in enumerate([
                'normal example of zoomblorp','regular text goes here','zoomblorp zoomblorp is spreading',
                'wow zoomblorp is everywhere','ordinary simple words here'])])
        rows=timeline(self.db,'zoomblorp',min_authors=2)
        self.assertEqual(rows[0]['mentions'],1)
        self.assertEqual(rows[1]['mentions'],3)
        self.assertEqual(rows[1]['distinct_authors'],2)
        self.assertEqual(rows[1]['per_1000_documents'],1000.0)
    def test_export(self):
        path=Path(self.t.name)/'trends.json'
        self.assertEqual(export_trends(self.db,path),0)
        self.assertIn('candidates',json.loads(path.read_text()))
    def test_reject_invalid(self):
        with self.assertRaises(ValueError):timeline(self.db,'foo','unrecognized')

if __name__=='__main__':unittest.main()
