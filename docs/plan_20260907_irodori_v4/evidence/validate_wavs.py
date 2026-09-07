import hashlib,json,pathlib
import numpy as np
import soundfile as sf
root=pathlib.Path(__file__).resolve().parents[3];work=root/'third_party/irodori-v4-research';evidence=pathlib.Path(__file__).parent
rows=[]
for f in sorted(work.glob('*.wav')):
    a,sr=sf.read(f,dtype='float64',always_2d=True); info=sf.info(f)
    row={'file':str(f.relative_to(root)),'sample_rate':sr,'channels':a.shape[1],'frames':len(a),'seconds':len(a)/sr,'subtype':info.subtype,'finite':bool(np.isfinite(a).all()),'peak':float(np.abs(a).max()),'rms':float(np.sqrt(np.mean(a*a))),'sha256':hashlib.sha256(f.read_bytes()).hexdigest()}
    assert row['sample_rate']==48000 and row['channels']==1 and row['subtype']=='PCM_16' and row['finite'] and row['peak']>0 and row['rms']>0
    rows.append(row)
(evidence/'wav-validation.json').write_text(json.dumps(rows,indent=2),encoding='utf-8')
print(json.dumps(rows,indent=2))
assert len(rows)==7, 'Expected v3, v4 and five v4.1 outputs'
