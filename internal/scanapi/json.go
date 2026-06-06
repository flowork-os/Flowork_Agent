// json.go — helper response JSON internal package scanapi (copy tfWriteJSON main,
// biar handler scanner ga gantung ke package main pas dipindah ke folder sendiri).

package scanapi

import (
	"encoding/json"
	"net/http"
)

// tfWriteJSON — tulis JSON. code==0 → biarin default 200; selainnya set status.
func tfWriteJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	if code != 0 {
		w.WriteHeader(code)
	}
	_ = json.NewEncoder(w).Encode(body)
}
