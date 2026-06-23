package main
import ("encoding/json";"io";"os";"regexp")
func main(){ var req struct{ Args struct{ Text string `json:"text"`; Pattern string `json:"pattern"` } `json:"args"` }
 in,_:=io.ReadAll(os.Stdin); _=json.Unmarshal(in,&req)
 if req.Args.Pattern==""{emit(nil,"param 'pattern' wajib");return}
 re,err:=regexp.Compile(req.Args.Pattern); if err!=nil{emit(nil,"regex invalid: "+err.Error());return}
 m:=re.FindAllString(req.Args.Text,-1); if m==nil{m=[]string{}}
 emit(map[string]any{"matches":m,"count":len(m)},"") }
func emit(o any,e string){x,_:=json.Marshal(map[string]any{"output":o,"error":e}); _,_=os.Stdout.Write(x)}
