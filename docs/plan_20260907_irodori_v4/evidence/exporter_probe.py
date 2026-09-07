import json,pathlib,sys,traceback
ROOT=pathlib.Path(__file__).resolve().parents[3];W=ROOT/'third_party/irodori-v4-research'
sys.path.insert(0,str(W/'Irodori-TTS-8224dafb46d0aba89209a8f905f1cb7e3299d9c1'))
sys.path.insert(0,str(W/'Irodori-TTS-ONNX-5df35d8720f810902971745a5ad961ff436bd73c/onnx_exporter'))
from irodori_tts.inference_runtime import _load_checkpoint_for_inference
from irodori_tts.config import ModelConfig,merge_dataclass_overrides
from irodori_tts.model import TextToLatentRFDiT
from irodori_tts_onnx.wrappers import TextEncoderModule,export_specs_for
state,cfg,train,backbone=_load_checkpoint_for_inference(W/'Irodori-TTS-v4.1-Small/model.safetensors')
model=TextToLatentRFDiT(merge_dataclass_overrides(ModelConfig(),cfg,section='checkpoint model_config'),pretrained_backbone_config=backbone,load_pretrained_backbone_weights=False)
model.load_state_dict(state,assign=True);model.eval()
report={'exporter_revision':'5df35d8720f810902971745a5ad961ff436bd73c','upstream_revision':'8224dafb46d0aba89209a8f905f1cb7e3299d9c1','kind':'current exporter wrappers against official v4.1 loaded model; not full exporter CLI','text_encoder_class':type(model.text_encoder).__name__,'spec_keys':list(export_specs_for(model))}
try:
    wrapper=TextEncoderModule(model)
    report['wrapper_construct']='success'
except Exception as exc:
    report['wrapper_construct']='failed';report['error']=repr(exc);traceback.print_exc()
print(json.dumps(report,ensure_ascii=False,indent=2))
(pathlib.Path(__file__).parent/'exporter-compat.json').write_text(json.dumps(report,ensure_ascii=False,indent=2),encoding='utf-8')
# This probe passes only if it reproduces both known migration blockers.
assert report['wrapper_construct']=='failed' and 'text_embedding' in report['error']
assert 'speaker_encoder' not in report['spec_keys'] and 'duration_predictor' not in report['spec_keys']

