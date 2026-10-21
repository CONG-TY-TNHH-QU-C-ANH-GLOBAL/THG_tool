"""Prepare only the eight owner-reviewed facts for the existing model preview."""

import hashlib
import json
from pathlib import Path
import sys


REVIEW_HASH = 'ed882debb485e5e55eeb3829ad9aaf05c548d1cfb4fc318bd92ae966c2aa4410'


def prepare(review_path, pack_path, destination):
    raw = Path(review_path).read_bytes().replace(b'\r\n', b'\n')
    if hashlib.sha256(raw).hexdigest() != REVIEW_HASH:
        raise ValueError('review changed')
    review = json.loads(raw)
    authored = {r['id']: r for r in json.loads(Path(pack_path).read_text(encoding='utf-8'))['records']}
    records, sources = [], {}
    for item in review['records']:
        payload = item['payload']
        records.append({
            'id': item['id'], 'title': item['title'], 'topic': 'company',
            'asset_type': item['asset_type'], 'audience': payload['audience'],
            'verification': payload['verification'], 'facts': payload['facts'],
            'source_ids': payload['source_ids'],
            'search_terms': authored[item['id']].get('search_terms', []),
        })
        for source in item['sources']:
            sources[source['id']] = dict(source, title=source['id'], coverage='reviewed public fact')
    target = Path(destination)
    target.mkdir(mode=0o700, parents=True, exist_ok=False)
    # Preview simulates approval in memory; it never mutates the authored pack.
    pack = dict(schema_version=1, review_state='pending', runtime_state='not_loaded', records=records)
    registry = dict(schema_version=1, sources=list(sources.values()))
    for name, data in (('knowledge.json', pack), ('sources.json', registry)):
        (target / name).write_text(json.dumps(data, ensure_ascii=False), encoding='utf-8')


if __name__ == '__main__':
    prepare(*sys.argv[1:])
