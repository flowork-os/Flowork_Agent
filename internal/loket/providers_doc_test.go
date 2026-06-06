package loket
import ("context";"encoding/json";"path/filepath";"testing")
func TestDocPutGetRoundTrip(t *testing.T) {
	dir := t.TempDir()
	sp := &storeProviders{deps: Deps{StorePath: func(m string)(string,error){return filepath.Join(dir,m+".db"),nil}}}
	if _, err := sp.docPut(context.Background(),"m",json.RawMessage(`{"collection":"interactions","id":"x1","body":{"text":"hi"}}`)); err != nil {
		t.Fatalf("docPut: %v", err)
	}
	r,err := sp.docGet(context.Background(),"m",json.RawMessage(`{"collection":"interactions","id":"x1"}`))
	if err != nil { t.Fatalf("docGet: %v", err) }
	var s struct{ Found bool `json:"found"`; Body json.RawMessage `json:"body"` }
	_ = json.Unmarshal(r,&s)
	if !s.Found { t.Errorf("not found: %s", r) }
}
