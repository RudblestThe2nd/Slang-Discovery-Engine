"""Slang Discovery MVP: offline-capable candidate mining and context-drift detection.

Candidates are hypotheses, not validated slang or confirmed semantic changes.
"""
import argparse
import csv
import json
import math
import os
import re
import sqlite3
import hashlib
from itertools import chain
from collections import Counter, defaultdict
from datetime import datetime, timezone
from pathlib import Path

URL = re.compile(r'https?://\S+|www\.\S+', re.I)
TOKEN = re.compile(r"(?u)\b[a-z][a-z0-9']{2,}\b", re.I)
PHRASE_TOKEN = re.compile(r"(?u)\b[a-z][a-z0-9']{1,}\b", re.I)
EXCLUDED = {'unknown','anonymous','deleted'}
STOP = set('the and are for you that this with have from they your was were but not can just its into about like what when where why how them then than all one out our who get got has had been there their his her she him its lol lmao omg tbh imo idk bruh yeah yes nah now very even really also some more much good bad used using make made people thing things will would could should only know said say dont didn didnt isnt wasnt ive youre im cant wasnt'.split())
COMMON = set('today tomorrow yesterday friend friends school work class game games chat post online internet someone something anyone everybody everybody really always never great awful tired exhausted trouble cooked fire ate fit drip sick random crazy awesome normal cooked blue hello love viral hype meme trend funny party music dude person day night week summer winter morning evening happy sad running going talking tried trying exam homework done over food lunch dinner tired failed stressed dancing dance dancing context word usage group early late comment words word term meaning phrase'.split())

def norm(s):
    return URL.sub(' ', s.lower()).replace('’', "'")

def tokens(s):
    return [w.strip("'") for w in TOKEN.findall(norm(s)) if w.strip("'") not in STOP]

def db_open(path):
    p=Path(path); p.parent.mkdir(parents=True,exist_ok=True)
    db=sqlite3.connect(p)
    db.executescript('''
    CREATE TABLE IF NOT EXISTS posts (
      id TEXT PRIMARY KEY, subreddit TEXT NOT NULL, author_hash TEXT NOT NULL,
      created_utc INTEGER NOT NULL, body TEXT NOT NULL, url TEXT NOT NULL,
      source TEXT NOT NULL DEFAULT 'demo', collected_utc INTEGER NOT NULL
    );
    CREATE TABLE IF NOT EXISTS candidates (
      term TEXT NOT NULL, kind TEXT NOT NULL, score REAL NOT NULL,
      mentions INTEGER NOT NULL, authors INTEGER NOT NULL, detail TEXT NOT NULL,
      updated_utc INTEGER NOT NULL, review_status TEXT NOT NULL DEFAULT 'pending',
      PRIMARY KEY(term,kind)
    );
    CREATE TABLE IF NOT EXISTS evidence (
      term TEXT NOT NULL, kind TEXT NOT NULL, post_id TEXT NOT NULL,
      PRIMARY KEY(term,kind,post_id), FOREIGN KEY(post_id) REFERENCES posts(id)
    );
    ''')
    db.execute('PRAGMA foreign_keys=ON')
    from intelligence import migrate
    migrate(db)
    return db

def ingest(db, records):
    n=0; now=int(datetime.now(timezone.utc).timestamp())
    for r in records:
        if not r.get('id') or not r.get('body') or r['body'] in ('[removed]','[deleted]'): continue
        if not r.get('created_utc'): continue
        if len(str(r['body']))<6: continue
        db.execute('INSERT OR IGNORE INTO posts VALUES (?,?,?,?,?,?,?,?)',(
          str(r['id']),str(r.get('subreddit','unknown')),
          str(r.get('author_hash','anonymous')),int(r['created_utc']),
          str(r['body']),str(r.get('url','')),str(r.get('source','demo')),now))
        n+=db.execute('SELECT changes()').fetchone()[0]
    db.commit(); return n

def load_jsonl(path):
    with open(path,encoding='utf-8') as f:
        for line in f:
            if line.strip():yield json.loads(line)

def collect_reddit(subreddits,limit):
    try:import praw
    except ImportError as e:raise SystemExit('Install PRAW: pip install -r requirements.txt') from e
    needed=['REDDIT_CLIENT_ID','REDDIT_CLIENT_SECRET','REDDIT_USER_AGENT']
    missing=[k for k in needed if not os.getenv(k)]
    if missing:raise SystemExit('Missing environment vars: '+', '.join(missing))
    api=praw.Reddit(client_id=os.environ['REDDIT_CLIENT_ID'],client_secret=os.environ['REDDIT_CLIENT_SECRET'],user_agent=os.environ['REDDIT_USER_AGENT'])
    def hash_author(author):
        # Stable pseudonym for diversity estimation only; not irreversible anonymization.
        raw=str(author or 'unknown')
        if raw in ('unknown','None','[deleted]'):return 'unknown'
        salt=os.getenv('AUTHOR_HASH_SALT')
        if not salt:raise SystemExit('Set AUTHOR_HASH_SALT to a private long random value before Reddit collection')
        return hashlib.sha256((salt+raw).encode()).hexdigest()[:20]
    for sr in subreddits:
        for submission in api.subreddit(sr).new(limit=limit):
            yield dict(id='t3_'+submission.id,subreddit=sr,author_hash=hash_author(submission.author),created_utc=int(submission.created_utc),body=(submission.title+' '+(submission.selftext or ''))[:10000],url='https://reddit.com'+submission.permalink,source='reddit')
            submission.comments.replace_more(limit=0)
            for c in submission.comments.list()[:min(100,limit)]:
                yield dict(id='t1_'+c.id,subreddit=sr,author_hash=hash_author(c.author),created_utc=int(c.created_utc),body=c.body[:10000],url='https://reddit.com'+c.permalink,source='reddit')

def detect(db, min_mentions=3, min_authors=2, drift_min=4, drift_threshold=.70):
    rows=db.execute('SELECT id,body,author_hash,created_utc FROM posts ORDER BY created_utc,id').fetchall()
    if not rows:return []
    try:
        from wordfreq import zipf_frequency
        def freq(w):return zipf_frequency(w,'en')
        dictionary='wordfreq'
    except ImportError:
        print('WARNING: wordfreq is not installed; lexical novelty discovery disabled. Install requirements.txt.')
        def freq(w):return 5.0 if w in COMMON else 0.0
        dictionary='unavailable (install wordfreq)'
    occurrence=defaultdict(set); authors=defaultdict(set); contexts=defaultdict(list); phrase_occurrence=defaultdict(set); phrase_authors=defaultdict(set)
    for id,body,author,stamp in rows:
        words=tokens(body); unique=set(words)
        # N-grams use contiguous tokens and never span sentence boundaries.
        sentences=re.split(r'[.!?;\n]+',norm(body))
        phrases=set()
        for sentence in sentences:
            seq=[w.strip("'") for w in PHRASE_TOKEN.findall(sentence)]
            for size in (2,3,4):
                for i in range(max(0,len(seq)-size+1)):
                    part=seq[i:i+size]
                    if part[0] in STOP or part[-1] in STOP:continue
                    if not any(w not in STOP and len(w)>=3 for w in part):continue
                    phrases.add(' '.join(part))
        for phrase in phrases:
            phrase_occurrence[phrase].add(id);phrase_authors[phrase].add(author)
        for w in unique:
            occurrence[w].add(id);authors[w].add(author);contexts[w].append((stamp,id,body))
    found=[]; now=int(datetime.now(timezone.utc).timestamp())
    db.execute('DELETE FROM evidence')
    # Recompute candidates so scores are not stale; preserve review statuses.
    statuses={(t,k):s for t,k,s in db.execute('SELECT term,kind,review_status FROM candidates')}
    db.execute('DELETE FROM candidates')
    def add(term,kind,score,ids,detail):
        score=round(max(0,min(100,score)),2)
        au=len(phrase_authors[term]) if kind=='phrase_candidate' else len(authors[term]); mentions=len(ids)
        db.execute('INSERT INTO candidates VALUES (?,?,?,?,?,?,?,?)', (term,kind,score,mentions,au,json.dumps(detail,ensure_ascii=False),now,statuses.get((term,kind),'pending')))
        db.executemany('INSERT OR IGNORE INTO evidence VALUES (?,?,?)',[(term,kind,i) for i in ids])
        found.append((term,kind,score,mentions,au,detail))
    for term,ids in occurrence.items():
        count=len(ids); au=len(authors[term])
        if count<min_mentions or len(authors[term]-{'unknown','anonymous'})<min_authors:continue
        f=freq(term)
        if dictionary=='wordfreq' and f<=2.5 and term not in COMMON and len(term)>=4 and not term.isnumeric():
            score= min(98,30 + 10*math.log2(count+1)+9*math.log2(au+1)+max(0,(3-f))*8)
            add(term,'lexical_novelty',score,ids,{'dictionary':dictionary,'zipf_frequency':round(f,3),'assessment':'candidate_only'})
    # Phrase ranking favors repeated multi-author expressions with uncommon components.
    if dictionary=='wordfreq':
        for phrase,ids in phrase_occurrence.items():
            au=len(phrase_authors[phrase]); count=len(ids)
            if count<max(3,min_mentions) or len(phrase_authors[phrase]-{'unknown','anonymous'})<min_authors:continue
            pieces=phrase.split()
            rarity=min(freq(w) for w in pieces)
            # Restrict to patterns involving a rare component. Not a claim of slang.
            if rarity>3.3:continue
            score=min(95, 19+11*math.log2(count+1)+10*math.log2(au+1)+max(0,3.5-rarity)*6)
            add(phrase,'phrase_candidate',score,ids,{'dictionary':dictionary,'rarest_word_zipf':round(rarity,3),'assessment':'unvalidated_phrase'})
    # Drift compares chronologically oldest and newest usage contexts.
    # TF-IDF context centroid distance is a weak signal, NOT proof of meaning shift.
    try:
        from sklearn.feature_extraction.text import TfidfVectorizer
        import numpy as np
        for term,uses in contexts.items():
            if len(uses)<2*drift_min or len(authors[term]-{'unknown','anonymous'})<min_authors or (dictionary=='wordfreq' and freq(term)<2.5):continue
            ordered=sorted(uses)
            early=ordered[:drift_min]; late=ordered[-drift_min:]
            if early[-1][0]>=late[0][0]:continue
            # Exclude target term; surrounding language drives the signal.
            # Prefer short local context window over whole comments (topic drift).
            def local_context(body):
                wordlist=PHRASE_TOKEN.findall(norm(body)); matches=[i for i,w in enumerate(wordlist) if w==term]
                if not matches:return ''
                i=matches[0]; return ' '.join(wordlist[max(0,i-12):i]+wordlist[i+1:i+13])
            docs=[local_context(x[2]) for x in early+late]
            if len(set(docs))<2:continue
            try:matrix=TfidfVectorizer(stop_words='english',ngram_range=(1,2)).fit_transform(docs)
            except ValueError:continue
            a=np.asarray(matrix[:drift_min].mean(axis=0)).ravel(); b=np.asarray(matrix[drift_min:].mean(axis=0)).ravel()
            sim=float(a@b/(np.linalg.norm(a)*np.linalg.norm(b)+1e-12))
            distance=1-sim
            # A strong difference in topics is only a hypothesis; require temporal separation.
            if distance>=drift_threshold:
                ids=[x[1] for x in early+late]
                add(term,'context_drift',25+65*distance,ids,{'distance':round(distance,3),'early_end':early[-1][0],'late_start':late[0][0],'assessment':'context_changed_not_confirmed_meaning'})
    except ImportError:
        pass
    db.commit()
    return sorted(found,key=lambda r:-r[2])

def review(db,term,kind,status,note=''):
    from intelligence import record_review
    return record_review(db,term,kind,status,note)

def evidence_for(db,term,kind,limit=15):
    return db.execute("""SELECT p.subreddit,p.created_utc,p.body,p.url,p.source
      FROM evidence e JOIN posts p ON p.id=e.post_id
      WHERE e.term=? AND e.kind=? ORDER BY p.created_utc DESC LIMIT ?""",(term,kind,limit)).fetchall()

def stats(db):
    return {'posts':db.execute('SELECT count(*) FROM posts').fetchone()[0],
            'candidates':db.execute('SELECT count(*) FROM candidates').fetchone()[0],
            'subreddits':db.execute('SELECT count(DISTINCT subreddit) FROM posts').fetchone()[0],
            'kinds':dict(db.execute('SELECT kind,count(*) FROM candidates GROUP BY kind')),
            'statuses':dict(db.execute('SELECT review_status,count(*) FROM candidates GROUP BY review_status'))}

def export_json(db,path):
    out=Path(path);out.parent.mkdir(parents=True,exist_ok=True)
    rows=db.execute('SELECT term,kind,score,mentions,authors,detail,review_status FROM candidates ORDER BY score DESC').fetchall()
    records=[dict(term=t,kind=k,score=s,mentions=m,authors=a,detail=json.loads(d),review_status=st)
             for t,k,s,m,a,d,st in rows]
    out.write_text(json.dumps(records,ensure_ascii=False,indent=2),encoding='utf-8')
    return len(records)

def export(db,path):
    out=Path(path);out.parent.mkdir(parents=True,exist_ok=True)
    rows=db.execute('SELECT term,kind,score,mentions,authors,detail,review_status FROM candidates ORDER BY score DESC').fetchall()
    with out.open('w',newline='',encoding='utf-8-sig') as f:
        w=csv.writer(f);w.writerow(['term','kind','score','mentions','authors','detail','review_status']);w.writerows(rows)
    return len(rows)

def main():
    p=argparse.ArgumentParser(description='Slang discovery MVP — hypotheses for human review')
    p.add_argument('command',choices=['demo','ingest','reddit','analyze','export','export-json','show','stats','review','evidence','report','purge','cluster','sense','evaluate','sync-deletions','run-pipeline','trends','timeline'])
    p.add_argument('--db',default='data/slang.db');p.add_argument('--input',default='data/demo.jsonl')
    p.add_argument('--output',default='data/candidates.csv');p.add_argument('--subreddits',default='GenZ,OutOfTheLoop')
    p.add_argument('--limit',type=int,default=10);p.add_argument('--min-mentions',type=int,default=3)
    p.add_argument('--min-authors',type=int,default=2);p.add_argument('--drift-min',type=int,default=4)
    p.add_argument('--term',default='');p.add_argument('--kind',default='lexical_novelty')
    p.add_argument('--note',default='');p.add_argument('--gloss',default='');p.add_argument('--method',choices=['tfidf','embeddings'],default='tfidf')
    p.add_argument('--gold',default='data/gold.jsonl')
    p.add_argument('--status',choices=['pending','accepted','rejected','needs_review'],default='needs_review')
    p.add_argument('--drift-threshold',type=float,default=.70)
    p.add_argument('--retention-days',type=int,default=90);p.add_argument('--top',type=int,default=30)
    args=p.parse_args();db=db_open(args.db)
    if args.command=='demo':
        path=Path(__file__).parent/'data'/'demo.jsonl'
        print('Ingested',ingest(db,load_jsonl(path)),'synthetic demo records')
        args.command='analyze'
    if args.command=='ingest':print('Ingested',ingest(db,load_jsonl(args.input)),'records')
    if args.command=='reddit':
        print('Ingested',ingest(db,collect_reddit([s.strip() for s in args.subreddits.split(',')],args.limit)),'Reddit records')
    if args.command=='analyze':
        result=detect(db,args.min_mentions,args.min_authors,args.drift_min,args.drift_threshold)
        for term,kind,score,count,au,_ in result[:25]:print(f'{term:22} {kind:18} score={score:5.1f} posts={count:3} authors={au}')
        print('Total candidates:',len(result))
    if args.command=='show':
        for row in db.execute('SELECT term,kind,score,mentions FROM candidates ORDER BY score DESC LIMIT 50'):print(*row,sep=' | ')
    if args.command=='export':print('Exported',export(db,args.output),'candidates to',args.output)
    if args.command=='export-json':print('Exported',export_json(db,args.output),'candidates to',args.output)
    if args.command=='stats':print(json.dumps(stats(db),indent=2))
    if args.command=='report':
        from analytics import build_report
        target=args.output if args.output!='data/candidates.csv' else 'data/analysis-report.md'
        print('Report rows:',build_report(db,target,args.top),'to',target)
    if args.command=='purge':
        from analytics import delete_old_posts
        print('Purged old documents:',delete_old_posts(db,args.retention_days),'— run analyze to rebuild candidates')
    if args.command=='review':print('Updated',review(db,args.term,args.kind,args.status,args.note),'candidate(s)')
    if args.command=='cluster':
        from intelligence import cluster_senses
        print(json.dumps(cluster_senses(db,args.term,args.method),indent=2))
    if args.command=='sense':
        from intelligence import add_sense
        print('Sense ID',add_sense(db,args.term,args.gloss))
    if args.command=='evaluate':
        from intelligence import evaluate_predictions
        print(json.dumps(evaluate_predictions(db,args.gold),indent=2))
    if args.command=='sync-deletions':
        from operations import sync_deletions
        print(json.dumps(sync_deletions(db),indent=2))
    if args.command=='run-pipeline':
        from operations import run_pipeline
        print(json.dumps(run_pipeline(db,args.subreddits.split(','),args.limit),indent=2))
    if args.command=='trends':
        from trends import export_trends
        target=args.output if args.output!='data/candidates.csv' else 'data/trends.json'
        print('Exported',export_trends(db,target,args.min_authors),'sample-based trend candidates to',target)
    if args.command=='timeline':
        from trends import timeline
        print(json.dumps(timeline(db,args.term,args.kind,args.min_authors),indent=2))
    if args.command=='evidence':
        for sr,dt,body,url,source in evidence_for(db,args.term,args.kind):
            print(f'[{source}/{sr}] {datetime.fromtimestamp(dt,timezone.utc).date()} {body[:300]}\n{url}\n')
    db.close()
if __name__=='__main__':main()
