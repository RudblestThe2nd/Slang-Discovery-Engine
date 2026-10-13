"""Allowed-API incremental acquisition, deletion reconciliation and orchestration."""
import os, json, time
from datetime import datetime,timezone
from slang_engine import ingest,detect

def api():
    try:import praw
    except ImportError as e:raise RuntimeError('Install praw from requirements.txt') from e
    keys=['REDDIT_CLIENT_ID','REDDIT_CLIENT_SECRET','REDDIT_USER_AGENT','AUTHOR_HASH_SALT']
    missing=[k for k in keys if not os.getenv(k)]
    if missing:raise RuntimeError('Missing credentials: '+', '.join(missing))
    if len(os.environ['AUTHOR_HASH_SALT'])<20:raise RuntimeError('AUTHOR_HASH_SALT must be 20+ characters')
    return praw.Reddit(client_id=os.environ['REDDIT_CLIENT_ID'],client_secret=os.environ['REDDIT_CLIENT_SECRET'],user_agent=os.environ['REDDIT_USER_AGENT'])

def author_id(author):
    import hashlib
    raw=str(author or '')
    if raw in ('','None','[deleted]'):return 'unknown'
    return hashlib.sha256((os.environ['AUTHOR_HASH_SALT']+raw).encode()).hexdigest()[:20]

def collect_incremental(db,subs,limit=40):
    r=api();total=0;errors=[];warnings=[]
    for sub in [x.strip() for x in subs if x.strip()]:
        prev=db.execute('SELECT last_seen_id FROM collection_state WHERE subreddit=?',(sub,)).fetchone()
        marker=prev[0] if prev else ''
        newest='';batch=[];found_marker=False;seen=0
        try:
            for s in r.subreddit(sub).new(limit=limit):
                sid='t3_'+s.id
                if not newest:newest=sid
                if sid==marker:
                    found_marker=True
                    break
                seen+=1
                batch.append({'id':sid,'subreddit':sub,'author_hash':author_id(s.author),'created_utc':int(s.created_utc),
                    'body':(s.title+' '+(s.selftext or ''))[:10000],'url':'https://reddit.com'+s.permalink,'source':'reddit'})
                # Limit comment retrieval and avoid expansive tree traversal.
                s.comments.replace_more(limit=0)
                for c in s.comments.list()[:min(limit,30)]:
                    batch.append({'id':'t1_'+c.id,'subreddit':sub,'author_hash':author_id(c.author),'created_utc':int(c.created_utc),
                        'body':str(c.body)[:10000],'url':'https://reddit.com'+c.permalink,'source':'reddit'})
            # If the old marker was not found in a full page, never advance it: doing so
            # would silently skip unseen submissions. Repeated fetch is safe (deduplicated).
            truncated=bool(marker and not found_marker and seen>=limit)
            total+=ingest(db,batch)
            if truncated:
                warnings.append({'subreddit':sub,'warning':'checkpoint_not_reached; cursor unchanged; potential backlog. Increase limit or backfill.'})
            db.execute('INSERT INTO collection_state(subreddit,last_seen_id,last_run_utc,last_error) VALUES (?,?,?,?) '
               'ON CONFLICT(subreddit) DO UPDATE SET last_seen_id=excluded.last_seen_id,last_run_utc=excluded.last_run_utc,last_error=""',
               (sub,marker if truncated else (newest or marker),int(time.time()),''));db.commit()
        except Exception as e:
            msg=f'{type(e).__name__}: {str(e)[:240]}'
            errors.append({'subreddit':sub,'error':msg})
            db.execute('INSERT INTO collection_state(subreddit,last_seen_id,last_run_utc,last_error) VALUES (?,?,?,?) '
               'ON CONFLICT(subreddit) DO UPDATE SET last_run_utc=excluded.last_run_utc,last_error=excluded.last_error',
               (sub,marker,int(time.time()),msg));db.commit()
    return {'new_posts':total,'errors':errors,'warnings':warnings}

def sync_deletions(db,limit=100):
    """Best-effort limited synchronization; cannot guarantee platform-wide deletion compliance.
    Call regularly and follow platform deletion obligations separately.
    """
    r=api()
    records=db.execute("SELECT id FROM posts WHERE source='reddit' ORDER BY collected_utc DESC LIMIT ?",(limit,)).fetchall()
    checked=0;removed=0;errors=[]
    for (pid,) in records:
        try:
            if pid.startswith('t1_'):
                obj=r.comment(id=pid[3:]); body=obj.body
            elif pid.startswith('t3_'):
                obj=r.submission(id=pid[3:]);body=obj.selftext
            else:continue
            checked+=1
            if body in ('[deleted]','[removed]') or getattr(obj,'author',None) is None:
                db.execute('DELETE FROM evidence WHERE post_id=?',(pid,))
                db.execute('DELETE FROM sense_evidence WHERE post_id=?',(pid,))
                db.execute('DELETE FROM posts WHERE id=?',(pid,));removed+=1
        except Exception as e:errors.append({'id':pid,'error':str(e)[:140]})
    db.commit()
    return {'checked':checked,'removed':removed,'errors':errors,'warning':'Subset only; review API rules and deletion obligations'}

def run_pipeline(db,subreddits,limit):
    start=int(time.time())
    result=collect_incremental(db,subreddits,limit)
    if not result['errors']:
        candidates=detect(db)
        result['candidates']=len(candidates)
    else:result['candidates']=None
    db.execute('INSERT INTO pipeline_runs(started_utc,mode,source_records,details) VALUES (?,?,?,?)',
               (start,'reddit',result['new_posts'],json.dumps(result)))
    db.commit()
    return result
