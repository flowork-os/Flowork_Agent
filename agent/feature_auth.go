// feature_auth.go — FASE-B: fitur AUTH self-register (single-owner password). Pola template
// buat fitur lain: bikin file feature_*.go + init()→RegisterFeature, NOL sentuh main.go.
package main

func init() {
	RegisterFeature(Feature{Name: "auth", Phase: PhaseRoute, Apply: func(d *Deps) {
		d.Mux.HandleFunc("/api/auth/me", d.AuthMgr.MeHandler)
		d.Mux.HandleFunc("/api/auth/login", d.AuthMgr.LoginHandler)
		d.Mux.HandleFunc("/api/auth/register", d.AuthMgr.RegisterHandler)
		d.Mux.HandleFunc("/api/auth/logout", d.AuthMgr.LogoutHandler)
		d.Mux.HandleFunc("/api/auth/change-password", d.AuthMgr.ChangePasswordHandler)
		d.Mux.HandleFunc("/api/owner/auto-verify", ownerAutoVerify)
	}})
}
