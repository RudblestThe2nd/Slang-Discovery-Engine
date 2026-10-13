"""Reproducible, exposure-normalized temporal trends (hypotheses only)."""
import json, math, re
from collections import defaultdict
from datetime import datetime, timezone
from pathlib import Path
from slang_engine import tokens, PHRASE_TOKEN, norm

def week_of(ts):
    return datetime.fromtimestamp(int(ts),timezone.utc).strftime('%G-W%V')

def timeline(db, term, kind='lexical_novelty', min_authors=2):
    term=term.strip().lower()
    if not term: raise ValueError('term required')
    if kind not in ('lexical_novelty','context_drift','phrase_candidate'):raise ValueError('invalid kind')
    byweek=defaultdict(lambda:{'documents':0,'mentions':0,'authors':set(),'communities':set()})
    word_pattern=re.compile(r'(?<![a-z0-9])'+re.escape(term)+r'(?![a-z0-9])',re.I)
    for stamp, body, author, community in db.execute('SELECT created_utc,body,author_hash,subreddit FROM posts'):
        w=week_of(stamp);bucket=byweek[w];bucket['documents']+=1
        count=len(word_pattern.findall(norm(body)))
        if count:
            bucket['mentions']+=count
            if author not in ('unknown','anonymous'):bucket['authors'].add(author)
            bucket['communities'].add(community)
    rows=[]
    for w, b in sorted(byweek.items()):
        rows.append({'week':w,'documents':b['documents'],'mentions':b['mentions'],
          'distinct_authors':len(b['authors']),'communities':len(b['communities']),
          'per_1000_documents':round(1000*b['mentions']/b['documents'],3) if b['documents'] else 0,
          'eligible':len(b['authors'])>=min_authors})
    return rows

def trends(db, min_authors=2, max_candidates=100):
    result=[]
    for term,kind,score in db.execute('SELECT term,kind,score FROM candidates ORDER BY score DESC LIMIT ?', (max_candidates,)):
        rows=timeline(db,term,kind,min_authors)
        observed=[r for r in rows if r['mentions']]
        if not observed:continue
        # Most recent two weeks, even if a term has zero occurrences in the latest week.
        a,b=(rows[-2:] if len(rows)>=2 else ([{'per_1000_documents':0}]+rows))
        growth=round(math.log2((b['per_1000_documents']+1)/(a['per_1000_documents']+1)),3)
        result.append({'term':term,'kind':kind,'score':score,'first_observed_week':observed[0]['week'],
          'last_observed_week':observed[-1]['week'],'weeks_observed':len(observed),
          'current_mentions':b['mentions'],'current_distinct_authors':b['distinct_authors'],
          'previous_per_1000':a['per_1000_documents'],'current_per_1000':b['per_1000_documents'],
          'log2_growth':growth,'eligible_this_week':b['eligible'],
          'note':'Observed in sampled corpus only; not the first use on the internet.'})
    return sorted(result,key=lambda r:(r['eligible_this_week'],r['log2_growth'],r['score']),reverse=True)

def export_trends(db,path,min_authors=2):
    target=Path(path);target.parent.mkdir(exist_ok=True,parents=True)
    payload={'method':'weekly sampled-document-normalized frequency; no population trend claims',
             'candidates':trends(db,min_authors=min_authors)}
    target.write_text(json.dumps(payload,ensure_ascii=False,indent=2),encoding='utf8')
    return len(payload['candidates'])
