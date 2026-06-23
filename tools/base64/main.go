package main
import ("encoding/base64";"encoding/json";"io";"os")
func main(){ var req struct{ Args struct{ Text string `json:"text"`; Mode string `json:"mode"` } `json:"args"` }
 in,_:=io.ReadAll(os.Stdin); _=json.Unmarshal(in,&req)
 if req.Args.Text==""{emit(nil,"param 'text' wajib");return}
 if req.Args.Mode=="decode"{ d,err:=base64.StdEncoding.DecodeString(req.Args.Text); if err!=nil{emit(nil,"decode gagal: "+err.Error());return}; emit(map[string]any{"result":string(d),"mode":"decode"},""); return }
 emit(map[string]any{"result":base64.StdEncoding.EncodeToString([]byte(req.Args.Text)),"mode":"encode"},"") }
func emit(o any,e string){x,_:=json.Marshal(map[string]any{"output":o,"error":e}); _,_=os.Stdout.Write(x)}
