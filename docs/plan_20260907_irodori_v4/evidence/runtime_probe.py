import argparse, dataclasses, hashlib, json, pathlib, sys, time, traceback
ROOT=pathlib.Path(__file__).resolve().parents[3]
WORK=ROOT/'third_party/irodori-v4-research'
SRC=WORK/'Irodori-TTS-8224dafb46d0aba89209a8f905f1cb7e3299d9c1'
sys.path.insert(0,str(SRC))
import numpy as np
import soundfile as sf
import torch
from huggingface_hub import snapshot_download
from irodori_tts.inference_runtime import InferenceRuntime, RuntimeKey, SamplingRequest
from irodori_tts.tokenizer import PretrainedTextTokenizer
p=argparse.ArgumentParser();p.add_argument('--version',default='v4.1');args=p.parse_args()
model='Irodori-TTS-'+args.version+'-Small'
info=json.loads((WORK/(model+'-info.json')).read_text(encoding='utf-8-sig'))
local=WORK/model
print('DOWNLOAD',model,info['sha'],flush=True)
snapshot_download('Aratako/'+model,revision=info['sha'],local_dir=str(local),allow_patterns=['model.safetensors','tokenizer/*'])
expected_hashes={'v4':'5863c986345d9f6d20b7d8748fee1af02079c5161cf0c9e52557da0a0c378593','v4.1':'c85de88c01700cb53538e706f128ebcb1b8513ad21d7d0e75f58bc82cdbf89f6'}
with (local/'model.safetensors').open('rb') as fh:
    model_hash=hashlib.file_digest(fh,'sha256').hexdigest()
print('MODEL_SHA256',model_hash,flush=True)
if model_hash!=expected_hashes[args.version]:
    raise RuntimeError('Model checksum mismatch: do not run inference; redownload the pinned checkpoint')
print('ENV',torch.__version__,torch.version.cuda,torch.cuda.get_device_name(0),flush=True)
if args.version=='v4.1':
    gt=json.loads((pathlib.Path(__file__).parent/'go-tokenizer.json').read_text(encoding='utf-8-sig'))
    tok=PretrainedTextTokenizer.from_pretrained(str(local/'tokenizer'),add_bos=True,local_files_only=True)
    comp=[]
    for row in gt['rows']:
        ids,mask=tok.batch_encode([row['text']],max_length=256)
        pyids=ids[0].tolist();pymask=mask[0].tolist()
        comp.append({'text':row['text'],'match':pyids==row['ids'] and pymask==row['mask'],'go_active':row['ids'][:sum(row['mask'])],'python_active':pyids[:sum(pymask)]})
    (pathlib.Path(__file__).parent/'tokenizer-parity.json').write_text(json.dumps({'go_specials':{k:gt[k] for k in ['bos','pad','unk']},'python_bos':tok.bos_token_id,'python_pad':tok.pad_token_id,'rows':comp},ensure_ascii=False,indent=2),encoding='utf-8')
    print('TOKENIZER_PARITY',[(r['text'],r['match']) for r in comp],flush=True)
torch.cuda.reset_peak_memory_stats();t=time.perf_counter()
rt=InferenceRuntime.from_key(RuntimeKey(checkpoint=str(local/'model.safetensors'),model_device='cuda',codec_device='cuda',model_precision='fp32',codec_precision='fp32'))
load=time.perf_counter()-t
print('LOADED',load,'watermark',rt.watermarker.ready,flush=True)
ref=ROOT/'narrator/narrator_01.wav'
data,sr=sf.read(ref)
print('REFERENCE',len(data)/sr,sr,flush=True)
cases=[('reference','こんにちは。今日はラジオの音声合成を確認します。',None,True,0)]
if args.version=='v4.1':
    cases += [('reference_warm','こんにちは。今日はラジオの音声合成を確認します。',None,True,0),('news','東京都では、九月七日の午後三時から新しい催しが始まります。',None,True,1),('caption_reference','こんにちは。今日はラジオの音声合成を確認します。','落ち着いて、聞き取りやすくニュースを読む。',True,0),('caption_only','こんにちは。今日はラジオの音声合成を確認します。','落ち着いた女性の声で、聞き取りやすくニュースを読む。',False,0)]
report={'model':model,'revision':info['sha'],'torch':torch.__version__,'cuda':torch.version.cuda,'precision':'fp32','steps':40,'cfg':[3,3,5],'cfg_min_t':0.5,'load_seconds':load,'watermark_ready':rt.watermarker.ready,'ref_seconds':len(data)/sr,'model_config':dataclasses.asdict(rt.model_cfg),'cases':[]}
for name,text,caption,use_ref,seed in cases:
    t=time.perf_counter()
    try:
        result=rt.synthesize(SamplingRequest(text=text,caption=caption,ref_wav=str(ref) if use_ref else None,no_ref=not use_ref,num_steps=40,seed=seed),log_fn=lambda msg:print(msg,flush=True))
        elapsed=time.perf_counter()-t
        audio=result.audio.detach().float().cpu().numpy().reshape(-1)
        out=WORK/(args.version+'-'+name+'.wav');sf.write(out,audio,result.sample_rate,subtype='PCM_16')
        row={'name':name,'text':text,'caption':caption,'seed':seed,'wall_seconds':elapsed,'audio_seconds':len(audio)/result.sample_rate,'sample_rate':result.sample_rate,'frames':len(audio),'finite':bool(np.isfinite(audio).all()),'peak':float(np.abs(audio).max()),'rms':float(np.sqrt(np.mean(audio**2))),'peak_cuda_allocated_mib':torch.cuda.max_memory_allocated()/2**20,'peak_cuda_reserved_mib':torch.cuda.max_memory_reserved()/2**20,'file':str(out.relative_to(ROOT)),'sha256':hashlib.sha256(out.read_bytes()).hexdigest(),'stage_timings':result.stage_timings,'messages':result.messages}
        row['rtf']=elapsed/row['audio_seconds'];report['cases'].append(row);print('RESULT',json.dumps(row,ensure_ascii=False),flush=True)
    except Exception as exc:
        report['cases'].append({'name':name,'error':repr(exc)});traceback.print_exc()
    (pathlib.Path(__file__).parent/(args.version+'-runtime.json')).write_text(json.dumps(report,ensure_ascii=False,indent=2),encoding='utf-8')
if any('error' in c for c in report['cases']): sys.exit(1)

