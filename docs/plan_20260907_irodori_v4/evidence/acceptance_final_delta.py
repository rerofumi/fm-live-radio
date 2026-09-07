import sys,pathlib,json
r=pathlib.Path.cwd(); sys.path.insert(0,str(r/'tools/irodori_export'))
from export import sha256
from parity import _validate_manifest
p=r/'model/irodori-v4.1'; m=json.loads((p/'manifest.json').read_text()); old=json.loads((r/'docs/plan_20260907_irodori_v4/evidence/wp1-final-snapshot.json').read_text(encoding='utf-8-sig'))
errors=_validate_manifest(m,p)
result={'manifest_sha256':sha256(p/'manifest.json'),'manifest_errors':errors,'codec_license':m['licenses']['codec_code']['spdx'],'parity_unchanged':sha256(r/'tools/irodori_export/parity.py')=='fd9fb75305869511c9c38c11b188bd8bf17e281d038657c8794ab61ba149c153','graph_retained':{n:sha256(p/g['path'])==old['graphs'][n]['sha256'] for n,g in m['graphs'].items()},'external_retained':{x['path']:sha256(p/x['path'])==x['sha256'] for x in old['external_data']},'exporter_hashes':{n:sha256(r/'tools/irodori_export'/n)==h for n,h in m['exporter']['files_sha256'].items()},'notice_hashes':{x['path']:sha256(r/x['path'])==x['sha256'] for x in m['licenses']['notice_files']}}
print(json.dumps(result,indent=2)); assert not errors and result['codec_license']=='Apache-2.0' and result['parity_unchanged'] and all(all(result[k].values()) for k in ('graph_retained','external_retained','exporter_hashes','notice_hashes'))
