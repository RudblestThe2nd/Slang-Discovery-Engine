"""Generate fully synthetic discussion examples for offline smoke testing."""
import json
from pathlib import Path
p=Path(__file__).parent/'data'/'demo.jsonl'
records=[]
for i in range(12):
    early=i<6
    msg=(f'Our dinner was cooked in the kitchen with soup and vegetables; the cooked meal was delicious {i}.' if early else f'I am cooked after that impossible exam and failed homework; everyone says we are cooked {i}.')
    records.append(dict(id=f'demo-cooked-{i}',body=msg,subreddit='synthetic',author_hash=f'person-{i%5}',created_utc=1700000000+i*86400,url='',source='demo'))
for i in range(9):
    records.append(dict(id=f'demo-new-{i}',body=f'That gaming clip was glorpmaxxing and the whole lobby went glorpmaxxing tonight! {i}',subreddit='synthetic',author_hash=f'gamer-{i%4}',created_utc=1701000000+i*86400,url='',source='demo'))
with p.open('w',encoding='utf8') as f:
    for r in records:f.write(json.dumps(r)+'\n')
print('Generated',len(records),'synthetic records')
