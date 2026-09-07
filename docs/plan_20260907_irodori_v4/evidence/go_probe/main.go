package main
import (
 "encoding/json"
 "fmt"
 "os"
 "time"
 "fm-live-radio/internal/generation"
 "fm-live-radio/internal/localtts/irodori/pipeline"
 "fm-live-radio/internal/localtts/irodori/tokenizer"
)
func main() {
 if len(os.Args)>1 && os.Args[1]=="tokenizer" {
  tok,err:=tokenizer.FromFile(os.Args[2],true); if err!=nil { panic(err) }
  texts:=[]string{"こんにちは。今日はラジオの音声合成を確認します。","東京都では、九月七日の午後三時から新しい催しが始まります。","AIと音楽、価格は1,234円です。","あははっ🤭、楽しいですね。","𠮷野家と髙橋さん。"," hello  world "}
  rows:=[]any{}
  for _,t:=range texts { ids,mask:=tok.EncodePadded(t,256); rows=append(rows,map[string]any{"text":t,"ids":ids,"mask":mask}) }
  json.NewEncoder(os.Stdout).Encode(map[string]any{"bos":tok.BOSID(),"pad":tok.PadID(),"unk":tok.UNKID(),"rows":rows}); return
 }
 if err:=generation.ConfigureExecutionProvider("cuda",0);err!=nil {panic(err)}
 if err:=generation.Init("");err!=nil {panic(err)}
 defer generation.Shutdown()
 opt:=pipeline.DefaultOptions();opt.ModelDir="model/irodori-v3";opt.Text="こんにちは。今日はラジオの音声合成を確認します。";opt.RefWAV="narrator/narrator_01.wav";opt.OutputWAV="third_party/irodori-v4-research/v3-baseline.wav";opt.Seed=0
 start:=time.Now();rt,err:=pipeline.LoadInitialise(opt);if err!=nil{panic(err)};defer rt.Close();load:=time.Since(start)
 start=time.Now();err=rt.Synthesize();if err!=nil{panic(err)}
 fmt.Printf("V3_BASELINE load_seconds=%.3f synthesis_seconds=%.3f output=%s steps=%d seed=%d\n",load.Seconds(),time.Since(start).Seconds(),opt.OutputWAV,opt.NumSteps,opt.Seed)
}
