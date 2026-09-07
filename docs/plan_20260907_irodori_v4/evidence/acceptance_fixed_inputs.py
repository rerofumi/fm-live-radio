import pathlib,sys,json,tempfile,os
sys.path.insert(0,str(pathlib.Path.cwd()/'tools/irodori_export'))
from export import _validate_fixed_inputs,_resolve_codec_weights,CODEC_REPO,SOURCE_REVISION,sha256
root=pathlib.Path.cwd(); work=root/'third_party/irodori-v4-research'; source=work/f'Irodori-TTS-{SOURCE_REVISION}'; checkpoint=work/'Irodori-TTS-v4.1-Small/model.safetensors'; tokenizer=work/'Irodori-TTS-v4.1-Small/tokenizer/tokenizer.json'; archive=work/'upstream.zip'
codec=_resolve_codec_weights(CODEC_REPO); result={'valid':str(_validate_fixed_inputs(source,checkpoint,tokenizer,str(codec),archive)), 'codec_file_sha256':sha256(codec),'negative':{}}
with tempfile.TemporaryDirectory(prefix='irodori-acceptance-') as temp:
 p=pathlib.Path(temp); bad=p/'bad.bin'; bad.write_bytes(b'corrupt'); bs=p/f'Irodori-TTS-{SOURCE_REVISION}'; bs.mkdir(); (bs/'changed.py').write_text('tampered=True')
 for label,args,expected in [('source',(bs,checkpoint,tokenizer,str(codec),archive),'source content SHA256 mismatch'),('archive',(source,checkpoint,tokenizer,str(codec),bad),'source archive SHA256 mismatch'),('checkpoint',(source,bad,tokenizer,str(codec),archive),'checkpoint SHA256 mismatch'),('tokenizer',(source,checkpoint,bad,str(codec),archive),'tokenizer SHA256 mismatch'),('codec',(source,checkpoint,tokenizer,str(bad),archive),'codec weights SHA256 mismatch')]:
  try: _validate_fixed_inputs(*args); result['negative'][label]={'ok':False,'error':'accepted'}
  except ValueError as e: result['negative'][label]={'ok':expected in str(e),'error':str(e)}
print(json.dumps(result,ensure_ascii=False,indent=2)); assert all(x['ok'] for x in result['negative'].values())
