import tempfile,unittest
from pathlib import Path
from unittest.mock import patch
from slang_engine import db_open
from operations import collect_incremental

class Submission:
 def __init__(self,id):
  self.id=id; self.author='author';self.created_utc=1600000000;self.title='hello world';self.selftext='ordinary hello world';self.permalink='/r/test/'+id
  class Comments:
   def replace_more(self,limit):pass
   def list(self):return []
  self.comments=Comments()
class Api:
 def subreddit(self,sub):return self
 def new(self,limit):return iter([Submission('recent'),Submission('older')][:limit])
class Tests(unittest.TestCase):
 def test_checkpoint_not_advanced_when_unseen_backlog(self):
  with tempfile.TemporaryDirectory() as tmp:
   db=db_open(Path(tmp)/'test.db')
   db.execute("INSERT INTO collection_state VALUES ('test','t3_veryold',0,'')");db.commit()
   with patch('operations.api',return_value=Api()),patch('operations.author_id',return_value='pseudo'):
    result=collect_incremental(db,['test'],limit=2)
   self.assertEqual(len(result['warnings']),1)
   self.assertEqual(db.execute("SELECT last_seen_id FROM collection_state WHERE subreddit='test'").fetchone()[0],'t3_veryold')
   self.assertEqual(result['new_posts'],2)
   db.close()
if __name__=='__main__':unittest.main()
