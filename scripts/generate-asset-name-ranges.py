#!/usr/bin/env python3
"""Generate the pinned asset alphabet using only vendored UCD 15.0.0 inputs."""
import hashlib
import json
from pathlib import Path
BASE = Path(__file__).resolve().parent.parent
UCD = BASE / 'scripts/unicode/15.0.0'
EXPECTED = {'UnicodeData.txt':'806e9aed65037197f1ec85e12be6e8cd870fc5608b4de0fffd990f689f376a73','DerivedCoreProperties.txt':'d367290bc0867e6b484c68370530bdd1a08b6b32404601b8c7accaf83e05628d','DerivedNormalizationProps.txt':'d5687a48c95c7d6e1ec59cb29c0f2e8b052018eb069a4371b7368d0561e12a29'}
for name, digest in EXPECTED.items():
    assert hashlib.sha256((UCD/name).read_bytes()).hexdigest() == digest, name
(UCD/'SHA256SUMS').write_text(''.join(f'{v}  {k}\n' for k,v in EXPECTED.items()))
def prop(name, field, value=None):
    out=set()
    for line in (UCD/name).read_text().splitlines():
        cols=[c.strip() for c in line.split('#')[0].split(';')]
        if len(cols)<2 or cols[1]!=field or value and cols[2]!=value: continue
        ends=cols[0].split('..'); out.update(range(int(ends[0],16),int(ends[-1],16)+1))
    return out
rows={}
for line in (UCD/'UnicodeData.txt').read_text().splitlines():
    row=line.split(';'); cp=int(row[0],16)
    if row[1].endswith(', First>'): start=cp
    elif row[1].endswith(', Last>'):
        for i in range(start,cp+1): rows[i]=row
    else: rows[cp]=row
letters={cp for cp,r in rows.items() if r[2][0] in 'LN'}
dec={cp:[int(x,16) for x in r[5].split()] for cp,r in rows.items() if r[5] and not r[5].startswith('<')}
following={v[1] for v in dec.values() if len(v)==2 and int(rows[v[1]][3])==0}
following.update(range(0x1161,0x1176)); following.update(range(0x11a8,0x11c3))
def ranges(values):
    out=[]
    for cp in sorted(values):
        if out and out[-1][1]+1==cp: out[-1][1]=cp
        else: out.append([cp,cp])
    return out
excluded={'defaultIgnorable':letters & prop('DerivedCoreProperties.txt','Default_Ignorable_Code_Point'), 'notNFC':letters & prop('DerivedNormalizationProps.txt','NFC_QC','N'), 'compositionFollowing':letters & following}
allowed=letters-set.union(*excluded.values())
maybe=prop('DerivedNormalizationProps.txt','NFC_QC','M')
assert excluded['compositionFollowing'] == letters & maybe
assert not allowed & (maybe | excluded['notNFC'])
report = {
    'unicodeVersion': '15.0.0', 'inputs': EXPECTED,
    'validation': 'passed',
    'candidateRangeCount': len(ranges(allowed | set(map(ord, '_-.')))),
    'excluded': {k: {'count': len(v), 'ranges': ranges(v)} for k, v in excluded.items()},
    'excludedTotal': len(letters - allowed),
    'hangulVCount': len(letters & set(range(0x1161, 0x1176))),
    'hangulTCount': len(letters & set(range(0x11a8, 0x11c3))),

}
(UCD/'asset-name-generation.json').write_text(json.dumps(report, indent=2)+'\n')
assert all(int(rows[cp][3]) == 0 for cp in allowed)
assert set(range(0xac00, 0xd7a4)) <= allowed
assert not (set(range(0x1161, 0x1176)) | set(range(0x11a8, 0x11c3))) & allowed
allowed.update(map(ord,'_-.'))
rules_path=BASE/'internal/sources/html_rules.json'
rules=json.loads(rules_path.read_text()); rules['version']=4
a=rules['asset']; a.pop('segmentPattern',None)
a.update(unicodeVersion='15.0.0',lengthUnit='utf8Bytes',maxSegmentLength=128,maxPathLength=512,maxSegments=4,allowedRanges=ranges(allowed),reservedNames=['con','prn','aux','nul']+[p+str(n) for p in ['com','lpt'] for n in range(1,10)]+[p+n for p in ['com','lpt'] for n in '¹²³'])
rules_path.write_text(json.dumps(rules,ensure_ascii=False,indent=2)+'\n')
