package main
import ("encoding/json";"io";"os";"strings")
func main(){ var req struct{ Args struct{ Text string `json:"text"` } `json:"args"` }
 in,_:=io.ReadAll(os.Stdin); _=json.Unmarshal(in,&req)
 if req.Args.Text==""{emit(nil,"param 'text' wajib");return}
 var b strings.Builder; prev:=false
 for _,r:=range strings.ToLower(req.Args.Text){ if (r>='a'&&r<='z')||(r>='0'&&r<='9'){b.WriteRune(r);prev=false}else if !prev{b.WriteByte('-');prev=true} }
 emit(map[string]any{"slug":strings.Trim(b.String(),"-")},"") }
func emit(o any,e string){x,_:=json.Marshal(map[string]any{"output":o,"error":e}); _,_=os.Stdout.Write(x)}
