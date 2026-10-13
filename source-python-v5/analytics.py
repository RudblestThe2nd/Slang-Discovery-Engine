"""Auditable discovery reporting; no external services required."""
import csv
import json
import math
from collections import Counter, defaultdict
from datetime import datetime, timezone
from pathlib import Path


def candidate_profile(db, term, kind):
    rows = db.execute('''SELECT p.id,p.subreddit,p.author_hash,p.created_utc,p.body,p.source
                         FROM evidence e JOIN posts p ON p.id=e.post_id
                         WHERE e.term=? AND e.kind=? ORDER BY p.created_utc''', (term,kind)).fetchall()
    if not rows:
        return {}
    daily=Counter(datetime.fromtimestamp(r[3], timezone.utc).strftime('%Y-%m-%d') for r in rows)
    communities=Counter(r[1] for r in rows)
    author_counts=Counter(r[2] for r in rows)
    peak=max(daily.values())
    first=min(r[3] for r in rows); last=max(r[3] for r in rows)
    period_days=max(1,(last-first)/86400)
    warnings=[]
    if len(communities)==1: warnings.append('Only one community: possible local jargon')
    if peak/len(rows)>=0.75: warnings.append('Highly concentrated on a single day')
    if max(author_counts.values())/len(rows)>=0.5: warnings.append('One author dominates the evidence')
    if len(rows)<5: warnings.append('Small evidence sample')
    if len(set(r[4].strip().casefold() for r in rows))<len(rows)*0.6: warnings.append('Repeated or near-identical content')
    return {
        'first_seen_utc':datetime.fromtimestamp(first,timezone.utc).isoformat(),
        'last_seen_utc':datetime.fromtimestamp(last,timezone.utc).isoformat(),
        'span_days':round(period_days,2),
        'communities':dict(communities.most_common()),
        'daily_mentions':dict(sorted(daily.items())),
        'unique_authors':len(author_counts),
        'evidence_count':len(rows),
        'evidence_warnings':warnings,
    }


def build_report(db, output, top=30):
    output=Path(output);output.parent.mkdir(parents=True,exist_ok=True)
    n_posts=db.execute('SELECT COUNT(*) FROM posts').fetchone()[0]
    n_reviewed=db.execute("SELECT COUNT(*) FROM candidates WHERE review_status!='pending'").fetchone()[0]
    rows=db.execute('SELECT term,kind,score,mentions,authors,detail,review_status FROM candidates ORDER BY score DESC,term LIMIT ?', (max(1,top),)).fetchall()
    lines=[
        '# Slang Discovery — Analysis Report', '',
        f'Generated (UTC): {datetime.now(timezone.utc).isoformat(timespec="seconds")}', '',
        f'- Documents stored: **{n_posts}**',
        f'- Candidates stored: **{db.execute("SELECT COUNT(*) FROM candidates").fetchone()[0]}**',
        f'- Candidates reviewed: **{n_reviewed}**',
        '- Scores are heuristic ranking scores, **not probabilities**.',
        '- Context drift means changing co-occurring vocabulary, **not proven semantic change**.',
        '', '## Highest-ranked candidates', '',
        '| Expression | Type | Score | Evidence docs | Authors | Review | Warnings |',
        '|---|---|---:|---:|---:|---|---|',
    ]
    detailed=[]
    for term,kind,score,mentions,authors,detail,status in rows:
        profile=candidate_profile(db,term,kind)
        warnings=profile.get('evidence_warnings',[])
        lines.append(f'| {term.replace("|","/")} | {kind} | {score:.1f} | {mentions} | {authors} | {status} | {"; ".join(warnings).replace("|","/")} |')
        detailed.append({'term':term,'kind':kind,'score':score,'mentions':mentions,'authors':authors,'review_status':status,'detail':json.loads(detail),'profile':profile})
    lines+=['', '## Quality and interpretation', '',
            'Lexical candidates require a frequency lexicon. Rare spellings may be typos, names, or coordinated memes.',
            'Phrase candidates are repeated n-grams, not confirmed idioms. A reviewer must check compositionality.',
            'Topic shifts, small data sets and sudden bursts can inflate context-drift scores.',
            'Evidence links must be checked before asserting a first-ever use. Earliest in this dataset is not earliest online.',
            '', '## Next validation steps', '',
            '1. Human-label a stratified random sample as slang / non-slang and evaluate precision.',
            '2. Add independent communities and longer before/after time windows.',
            '3. Validate candidate meanings with separate examples, not only examples used for discovery.',
            '4. Audit platform terms, attribution, retention and deletion handling before production use.', '']
    output.write_text('\n'.join(lines),encoding='utf-8')
    output.with_suffix('.json').write_text(json.dumps({'documents':n_posts,'reviewed':n_reviewed,'candidates':detailed},indent=2,ensure_ascii=False),encoding='utf-8')
    return len(rows)


def delete_old_posts(db, days, now=None):
    if days < 1:raise ValueError('Retention days must be >=1')
    now = int(datetime.now(timezone.utc).timestamp()) if now is None else int(now)
    cutoff=now-days*86400
    # Discard evidence referencing soon-to-be deleted source posts and candidates' stale results.
    db.execute('DELETE FROM evidence WHERE post_id IN (SELECT id FROM posts WHERE collected_utc < ?)',(cutoff,))
    cur=db.execute('DELETE FROM posts WHERE collected_utc < ?', (cutoff,))
    db.execute('DELETE FROM candidates')  # requires re-analysis: rankings otherwise stale
    db.execute('DELETE FROM evidence')
    db.commit()
    return cur.rowcount
