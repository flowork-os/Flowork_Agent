package main
import ("encoding/json";"io";"os")
func main(){ var req struct{ Args struct{ Text string `json:"text"`; Mode string `json:"mode"` } `json:"args"` }
 in,_:=io.ReadAll(os.Stdin); _=json.Unmarshal(in,&req)
 if req.Args.Text==""{emit(nil,"param 'text' wajib");return}
 var v any; if err:=json.Unmarshal([]byte(req.Args.Text),&v);err!=nil{emit(map[string]any{"valid":false},"JSON invalid: "+err.Error());return}
 var out []byte; if req.Args.Mode=="minify"{out,_=json.Marshal(v)}else{out,_=json.MarshalIndent(v,"","  ")}
 emit(map[string]any{"result":string(out),"valid":true},"") }
func emit(o any,e string){x,_:=json.Marshal(map[string]any{"output":o,"error":e}); _,_=os.Stdout.Write(x)}
