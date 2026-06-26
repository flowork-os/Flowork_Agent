// routes_ext.go — NON-frozen GROWTH POINT for HTTP routes (seam evolusi).
//
// routes.go (wiring rute) + semua handler yang ditunjuknya = FROZEN (kramat). Untuk
// MENGEKSPOS endpoint BARU TANPA membuka satu pun file frozen: JANGAN edit routes.go —
// buat file baru `handlers_<fitur>_ext.go` lalu daftar lewat switch di bawah:
//
//	func init() {
//	    RegisterExtraRoute(func(mux *http.ServeMux) {
//	        mux.HandleFunc("/api/<fitur>", myHandler)
//	    })
//	}
//
// registerRoutes (frozen) memanggil registerExtraRoutes(mux) PALING AKHIR, jadi tiap
// registrar yang di-append di sini otomatis ter-wire saat startup. Nol file frozen
// disentuh → freeze tetap kramat, Flowork bisa tumbuh endpoint selamanya.
//
// Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
package main

import (
	"net/http"
	"sync"
)

var (
	extraRouteMu         sync.Mutex
	extraRouteRegistrars []func(*http.ServeMux)
)

// RegisterExtraRoute mendaftarkan satu fungsi registrasi-rute. Panggil dari init()
// di file *_ext.go baru. Inilah seam yang membuat Flowork bisa menambah endpoint
// abadi tanpa membuka file frozen.
func RegisterExtraRoute(reg func(*http.ServeMux)) {
	if reg == nil {
		return
	}
	extraRouteMu.Lock()
	extraRouteRegistrars = append(extraRouteRegistrars, reg)
	extraRouteMu.Unlock()
}

// registerExtraRoutes dipanggil registerRoutes (routes.go) setelah grup rute inti.
// Mewire tiap registrar yang di-append lewat RegisterExtraRoute.
func registerExtraRoutes(mux *http.ServeMux) {
	extraRouteMu.Lock()
	regs := make([]func(*http.ServeMux), len(extraRouteRegistrars))
	copy(regs, extraRouteRegistrars)
	extraRouteMu.Unlock()
	for _, reg := range regs {
		reg(mux)
	}
}
