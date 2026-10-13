"""Interpretability-first semantic clustering, lexicon senses, evaluation and provenance.
Optional SentenceTransformers uses pretrained inference, NOT training on source texts.
"""
import json, math, re, sqlite3
from collections import Counter, defaultdict
from datetime import datetime, timezone

SCHEMA = '''
CREATE TABLE IF NOT EXISTS senses (
 id INTEGER PRIMARY KEY AUTOINCREMENT, term TEXT NOT NULL, gloss TEXT NOT NULL,
 status TEXT NOT NULL DEFAULT 'proposed', model TEXT NOT NULL DEFAULT 'manual',
 created_utc INTEGER NOT NULL, UNIQUE(term,gloss));
CREATE TABLE IF NOT EXISTS sense_evidence (
 sense_id INTEGER NOT NULL, post_id TEXT NOT NULL,
 PRIMARY KEY(sense_id,post_id), FOREIGN KEY(sense_id) REFERENCES senses(id) ON DELETE CASCADE,
 FOREIGN KEY(post_id) REFERENCES posts(id) ON DELETE CASCADE);
CREATE TABLE IF NOT EXISTS review_events (
 id INTEGER PRIMARY KEY AUTOINCREMENT, term TEXT NOT NULL, kind TEXT NOT NULL,
 decision TEXT NOT NULL, note TEXT NOT NULL, reviewed_utc INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS collection_state (
 subreddit TEXT PRIMARY KEY, last_seen_id TEXT NOT NULL DEFAULT '', last_run_utc INTEGER NOT NULL DEFAULT 0,
 last_error TEXT NOT NULL DEFAULT '');
CREATE TABLE IF NOT EXISTS pipeline_runs (
 id INTEGER PRIMARY KEY AUTOINCREMENT, started_utc INTEGER NOT NULL, mode TEXT NOT NULL,
 source_records INTEGER NOT NULL, details TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS semantic_clusters (
 term TEXT NOT NULL, cluster_id INTEGER NOT NULL, example_count INTEGER NOT NULL,
 example TEXT NOT NULL, cohesion REAL NOT NULL, method TEXT NOT NULL,
 PRIMARY KEY(term,cluster_id));
'''

def migrate(db):
    db.executescript(SCHEMA)
    db.commit()

def record_review(db,term,kind,decision,note=''):
    if decision not in {'pending','accepted','rejected','needs_review'}:raise ValueError('invalid decision')
    n=db.execute('UPDATE candidates SET review_status=? WHERE term=? AND kind=?',(decision,term,kind)).rowcount
    if n:
        db.execute('INSERT INTO review_events(term,kind,decision,note,reviewed_utc) VALUES (?,?,?,?,?)',
            (term,kind,decision,note,int(datetime.now(timezone.utc).timestamp())))
    db.commit()
    return n

def add_sense(db,term,gloss,post_ids=(),status='proposed'):
    if status not in {'proposed','verified','rejected'}:raise ValueError('invalid status')
    if not term.strip() or not gloss.strip():raise ValueError('term and gloss required')
    cur=db.execute('INSERT OR IGNORE INTO senses(term,gloss,status,created_utc) VALUES (?,?,?,?)',
       (term.strip().lower(),gloss.strip(),status,int(datetime.now(timezone.utc).timestamp())))
    sid=db.execute('SELECT id FROM senses WHERE term=? AND gloss=?',(term.strip().lower(),gloss.strip())).fetchone()[0]
    db.executemany('INSERT OR IGNORE INTO sense_evidence(sense_id,post_id) VALUES (?,?)',
       [(sid,p) for p in post_ids if db.execute('SELECT 1 FROM posts WHERE id=?',(p,)).fetchone()])
    db.commit()
    return sid

def local_context(body,term,window=14):
    ts=re.findall(r"[a-z][a-z0-9']*",body.lower())
    idx=[i for i,w in enumerate(ts) if w==term.lower()]
    return [' '.join(ts[max(0,i-window):i]+['[TARGET]']+ts[i+1:i+window+1]) for i in idx]

def cluster_senses(db,term,method='tfidf',min_uses=6,min_cluster_size=2):
    """Discover distinct contexts (NOT verified senses); explicit method fallback."""
    from sklearn.cluster import KMeans
    from sklearn.metrics import silhouette_score
    uses=[]
    for pid,body in db.execute('SELECT id,body FROM posts WHERE lower(body) LIKE ? ORDER BY created_utc',(f'%{term.lower()}%',)):
        for context in local_context(body,term):uses.append((pid,context))
    if len(uses)<min_uses:return {'status':'insufficient_evidence','examples':len(uses)}
    texts=[x[1] for x in uses]
    actual_method='tfidf'
    if method=='embeddings':
        try:
            from sentence_transformers import SentenceTransformer
            model=SentenceTransformer('sentence-transformers/all-MiniLM-L6-v2')
            x=model.encode(texts,normalize_embeddings=True,show_progress_bar=False)
            actual_method='sentence-transformers/all-MiniLM-L6-v2'
        except (ImportError, OSError) as exc:
            return {'status':'model_unavailable','reason':str(exc)[:240], 'examples':len(texts)}
    else:
        from sklearn.feature_extraction.text import TfidfVectorizer
        try:x=TfidfVectorizer(ngram_range=(1,2),stop_words='english').fit_transform(texts)
        except ValueError:return {'status':'insufficient_vocabulary','examples':len(texts)}
    if len(set(texts))<2:return {'status':'insufficient_diversity','examples':len(texts)}
    best=None
    for k in range(2,min(4,len(texts)//min_cluster_size)+1):
        labels=KMeans(n_clusters=k,random_state=42,n_init=10).fit_predict(x)
        sizes=Counter(labels)
        if min(sizes.values())<min_cluster_size:continue
        try:score=float(silhouette_score(x,labels,metric='cosine'))
        except ValueError:continue
        if best is None or score>best[0]:best=(score,labels,k)
    if best is None:return {'status':'no_stable_clusters','examples':len(texts)}
    score,labels,k=best
    db.execute('DELETE FROM semantic_clusters WHERE term=?',(term,))
    data=[]
    for c in range(k):
        matches=[texts[i] for i,l in enumerate(labels) if l==c]
        example=matches[0]
        db.execute('INSERT INTO semantic_clusters VALUES (?,?,?,?,?,?)',(term,c,len(matches),example,round(score,4),actual_method))
        data.append({'cluster':c,'count':len(matches),'example':example})
    db.commit()
    return {'status':'clusters_proposed_not_meanings','term':term,'method':actual_method,'silhouette':round(score,4),'clusters':data}

def evaluate_predictions(db, gold_path,top=20):
    """Gold JSONL rows: {'term':..., 'kind':..., 'is_slang':true}; returns ranking precision and label coverage."""
    with open(gold_path,encoding='utf8') as f:gold=[json.loads(x) for x in f if x.strip()]
    labels={(r['term'].lower(),r['kind']):bool(r['is_slang']) for r in gold}
    ranked=[(t,k) for t,k in db.execute('SELECT term,kind FROM candidates ORDER BY score DESC,term')]
    matched=[c for c in ranked if c in labels]
    hits=sum(labels[k] for k in matched[:top]); examined=len(matched[:top])
    positives=sum(labels.values())
    return {'gold_items':len(labels),'ranked_items':len(ranked),'labeled_predictions':len(matched),
            'precision_at_k_labeled_only':round(hits/examined,4) if examined else None,
            'recall_on_labeled_positives':round(sum(labels[k] for k in matched)/positives,4) if positives else None,
            'evaluated_k':examined,'warning':'Only manually labeled candidates count; not an unbiased population estimate.'}
