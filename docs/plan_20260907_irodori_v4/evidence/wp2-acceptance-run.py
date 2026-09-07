import sys, pathlib, json, hashlib, subprocess, random, importlib.metadata
root=pathlib.Path(__file__).resolve().parents[3]
out=root/'docs/plan_20260907_irodori_v4/evidence'
probe=out/'wp2-acceptance-probe'
(probe/'legacy').mkdir(parents=True,exist_ok=True)
old=subprocess.check_output(['jj','file','show','-r','4573c430','internal/localtts/irodori/tokenizer/tokenizer.go'],cwd=root)
(probe/'legacy/tokenizer.go').write_bytes(old)
go=r'''package main
import ("encoding/json";"os";"fmt"; current "fm-live-radio/internal/localtts/irodori/tokenizer"; legacy "fm-live-radio/docs/plan_20260907_irodori_v4/evidence/wp2-acceptance-probe/legacy")
type Case struct{Name string;Text string;BOS bool;Length int}
func main(){var cases []Case;if err:=json.NewDecoder(os.Stdin).Decode(&cases);err!=nil{panic(err)};rows:=[]map[string]any{};for _,version:=range []string{"v4","v3"}{path:="model/irodori-v4.1/tokenizer/tokenizer.json";if version=="v3"{path="model/irodori-v3/tokenizer.json"};for _,bos:=range []bool{false,true}{tok,err:=current.FromFile(path,bos);if err!=nil{panic(err)};old,err:=legacy.FromFile(path,bos);if err!=nil{panic(err)};for _,c:=range cases{if c.BOS!=bos{continue};ids,mask,err:=tok.EncodePaddedChecked(c.Text,c.Length);r:=map[string]any{"version":version,"name":c.Name,"bos":bos,"length":c.Length,"ids":ids,"mask":mask,"raw":tok.Encode(c.Text),"pad":tok.PadID()};if err!=nil{r["error"]=fmt.Sprint(err)};if version=="v3"&&c.Length>0{o,m:=old.EncodePadded(c.Text,c.Length);r["legacy_ids"]=o;r["legacy_mask"]=m;r["legacy_raw"]=old.Encode(c.Text)};rows=append(rows,r)}}};json.NewEncoder(os.Stdout).Encode(rows)}
'''
(probe/'main.go').write_text(go,encoding='utf-8')
source=root/'third_party/irodori-v4-research/Irodori-TTS-8224dafb46d0aba89209a8f905f1cb7e3299d9c1'
sys.path.insert(0,str(source))
from irodori_tts.tokenizer import PretrainedTextTokenizer
model=root/'third_party/irodori-v4-research/Irodori-TTS-v4.1-Small/tokenizer'
oldrows=json.loads((out/'tokenizer-parity.json').read_text(encoding='utf-8-sig'))['rows']
texts=[(f'original_{i}',x['text']) for i,x in enumerate(oldrows)]
texts += [('empty',''),('space',' '),('spaces','    '),('tab','\t'),('crlf','\r\nhello\t world\n'),('newline','\nこんにちは'),('unicode_space','\u00a0hello\u3000world\u2009'),('combining','é e\u0301 が か\u3099'),('width','ＡＢＣ１２３ ｶﾞｯﾂ'),('control','a\0b\u200b\u200d\ufeff'),('fallback','🤭𠮷\U0010ffff\ue000🫠'),('digits','0 1.234 -9e10 １２ ① Ⅳ'),('symbols','！？!? +-*/= ©™🚀'),('specials','<unk><s></s><pad><sep><mask><cls><|assistant|><|prefix|>'),('specials_spaces',' hello <s> world <pad> '),('metaspace','▁hello  world▁')]
for n in [1,63,64,65,253,254,255,256,257,258,511,512,513]:
 texts += [(f'special_count_{n}','<|assistant|>'*n),(f'newline_count_{n}','\n'*n)]
for n in [254,255,256,257,258,259,260,511,512]:
 texts.append((f'ascii_{n}','a'*n))
rng=random.Random(20260908)
atoms=['あ','今日','音楽','AI','x','a',' ', '\n','\t','🤭','𠮷','é','\u0301','<pad>','<s>','漢字','123','▁','\u3000','🫠']
for i in range(100): texts.append((f'mixed_{i}',''.join(rng.choice(atoms) for _ in range(rng.randint(1,35)))))
cases=[dict(Name=name,Text=t,BOS=bos,Length=length) for name,t in texts for bos in [False,True] for length in ([1,64,256,512] if not name.startswith('mixed_') else [256])]
cases += [dict(Name='invalid',Text='hello',BOS=bos,Length=n) for bos in [False,True] for n in [0,-1]]
(out/'wp2-acceptance-cases.json').write_text(json.dumps(cases,ensure_ascii=False,indent=2),encoding='utf-8')
proc=subprocess.run(['mise','x','--','go','run',str(probe)],cwd=root,input=json.dumps(cases,ensure_ascii=False),encoding='utf-8',capture_output=True,check=True)
actual=json.loads(proc.stdout)
actualmap={(r['version'],r['name'],r['bos'],r['length']):r for r in actual}
results=[]
for bos in [False,True]:
 tok=PretrainedTextTokenizer.from_pretrained(str(model),add_bos=bos,local_files_only=True)
 for c in cases:
  if c['BOS']!=bos:continue
  r=actualmap[('v4',c['Name'],bos,c['Length'])]
  try:
   ids,mask=tok.batch_encode([c['Text']],max_length=c['Length'])
   expected_ids=ids[0].tolist(); expected_mask=mask[0].tolist()
   raw=tok.encode(c['Text'],add_bos=False).tolist()
   match=r['ids']==expected_ids and r['mask']==expected_mask and r['raw']==raw
   results.append(dict(name=c['Name'],bos=bos,length=c['Length'],match=match,raw_length=len(raw),expected_ids=expected_ids,expected_mask=expected_mask,expected_raw=raw,actual=r))
  except ValueError as e:
   results.append(dict(name=c['Name'],bos=bos,length=c['Length'],match=bool(r.get('error')),expected_error=str(e),actual=r))
v3=[dict(name=r['name'],bos=r['bos'],length=r['length'],match=(r['ids']==r.get('legacy_ids') and r['mask']==r.get('legacy_mask') and r['raw']==r.get('legacy_raw')),actual=r) for r in actual if r['version']=='v3' and r['length']>0]
paths=['internal/localtts/irodori/tokenizer/tokenizer.go','internal/localtts/irodori/tokenizer/tokenizer_test.go','internal/localtts/irodori/pipeline/pipeline.go','model/irodori-v4.1/tokenizer/tokenizer.json','model/irodori-v3/tokenizer.json','tools/irodori_export/uv.lock']
paths += [str(p.relative_to(root)) for p in sorted(model.glob('*.json'))]+[str((source/'irodori_tts/tokenizer.py').relative_to(root))]
meta={'executor':'WP-2 independent acceptance','baseline_snapshot':'4573c430','snapshot':subprocess.check_output(['jj','log','-r','@','--no-graph','-T','commit_id'],cwd=root,text=True).strip(),'python':sys.version,'versions':{p:importlib.metadata.version(p) for p in ['transformers','tokenizers','torch']},'hashes':{p:hashlib.sha256((root/p).read_bytes()).hexdigest() for p in paths},'baseline_extracted_sha256':hashlib.sha256(old).hexdigest(),'v4_count':len(results),'v4_failures':sum(not r['match'] for r in results),'v3_count':len(v3),'v3_failures':sum(not r['match'] for r in v3)}
(out/'wp2-acceptance-parity.json').write_text(json.dumps({'meta':meta,'v4':results,'v3':v3},ensure_ascii=False,indent=2),encoding='utf-8')
print(json.dumps(meta,indent=2))
for r in results:
 if not r['match']:print('MISMATCH',r['name'],r['bos'],r['length'],r.get('expected_raw'),r['actual']['raw'])

