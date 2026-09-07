import pathlib,json,gzip,hashlib,zipfile
root=pathlib.Path(__file__).resolve().parents[3]
out=root/'docs/plan_20260907_irodori_v4/evidence'
p=out/'wp2-acceptance-parity.json'
data=json.loads(p.read_text(encoding='utf-8'))
source_path='third_party/irodori-v4-research/Irodori-TTS-8224dafb46d0aba89209a8f905f1cb7e3299d9c1/irodori_tts/tokenizer.py'
archive=root/'third_party/irodori-v4-research/upstream.zip'
with zipfile.ZipFile(archive) as z:
 n=next(n for n in z.namelist() if n.endswith('/irodori_tts/tokenizer.py'))
 data['meta']['official_source_archive_match']=z.read(n)==(root/source_path).read_bytes()
data['meta']['archive_sha256']=hashlib.sha256(archive.read_bytes()).hexdigest()
raw=json.dumps(data,ensure_ascii=False,separators=(',',':')).encode()
g=gzip.compress(raw,mtime=0)
(out/'wp2-acceptance-parity-full.json.gz').write_bytes(g)
summary={'meta':data['meta'],'full_evidence':{'path':'wp2-acceptance-parity-full.json.gz','sha256':hashlib.sha256(g).hexdigest(),'uncompressed_sha256':hashlib.sha256(raw).hexdigest()},'v4':[dict(name=r['name'],bos=r['bos'],length=r['length'],match=r['match'],raw_length=r.get('raw_length'),ids_mask_sha256=hashlib.sha256(json.dumps([r['actual']['ids'],r['actual']['mask']],separators=(',',':')).encode()).hexdigest()) for r in data['v4']],'v3':[dict(name=r['name'],bos=r['bos'],length=r['length'],match=r['match']) for r in data['v3']]}
p.write_text(json.dumps(summary,ensure_ascii=False,indent=2),encoding='utf-8')
print(json.dumps({'full_gzip_bytes':len(g),'summary_bytes':p.stat().st_size,'source_archive_match':data['meta']['official_source_archive_match']}))

