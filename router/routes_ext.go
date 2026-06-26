// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah fitur TANPA buka frozen: file sibling baru + registry (RegisterMeshFilter/
// RegisterExtraRoute/RegisterGraphProjection) + SWITCH fwswitch. Pola: lock/frozen-core.md

package main

import (
	"net/http"
	"sync"
)

var (
	extraRouteMu         sync.Mutex
	extraRouteRegistrars []func(*http.ServeMux)
)

func RegisterExtraRoute(reg func(*http.ServeMux)) {
	if reg == nil {
		return
	}
	extraRouteMu.Lock()
	extraRouteRegistrars = append(extraRouteRegistrars, reg)
	extraRouteMu.Unlock()
}

func registerExtraRoutes(mux *http.ServeMux) {
	extraRouteMu.Lock()
	regs := make([]func(*http.ServeMux), len(extraRouteRegistrars))
	copy(regs, extraRouteRegistrars)
	extraRouteMu.Unlock()
	for _, reg := range regs {
		reg(mux)
	}
}
