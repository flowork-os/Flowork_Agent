// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Capability broker (cap gate). Audit pass — RWMutex thread-safe,
//   Approve copies caps slice, Approved returns copy (no leak), prefix match
//   dengan boundary char ':', '/', '?' (anti subdomain hijack).
//
// Package broker — capability gate. Phase 5 menambah dev-mode auto-approve
// dari manifest.capabilities_required. Phase 11 nanti diganti modal
// permission Android-style; sementara di dev kita trust manifest.

package broker

import (
	"strings"
	"sync"
)

// Broker — capability gate. Approval per-plugin disimpan in-memory
// (Phase 11 akan persist ke kv). Thread-safe.
type Broker struct {
	mu        sync.RWMutex
	approvals map[string][]string
}

// New return broker kosong.
func New() *Broker {
	return &Broker{approvals: map[string][]string{}}
}

// Approve overwrite approved capabilities untuk satu plugin. Dipanggil
// saat boot dengan capabilities_required (dev mode) atau dari modal
// owner approval (Phase 11+).
func (b *Broker) Approve(pluginID string, caps []string) {
	cp := make([]string, len(caps))
	copy(cp, caps)
	b.mu.Lock()
	b.approvals[pluginID] = cp
	b.mu.Unlock()
}

// Revoke hapus approved caps satu plugin.
func (b *Broker) Revoke(pluginID string) {
	b.mu.Lock()
	delete(b.approvals, pluginID)
	b.mu.Unlock()
}

// IsApproved cek apakah capability request cocok dengan salah satu
// approved. Match rules:
//
//   exact:          cap == approved
//   sub continue:   approved + ":" prefix (kv:get cover kv:get:my.key)
//   URL path:       approved + "/" prefix (net:fetch:https://host cover .../path)
//   URL query:      approved + "?" prefix
//
// Boundary char penting biar approval `kv:get` ngga nge-cover `kv:getx`,
// dan `net:fetch:https://api.foo.com` ngga nge-cover `net:fetch:https://api.foo.com.evil/`.
func (b *Broker) IsApproved(pluginID, capability string) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, approved := range b.approvals[pluginID] {
		if approved == capability {
			return true
		}
		if len(capability) > len(approved) && strings.HasPrefix(capability, approved) {
			switch capability[len(approved)] {
			case ':', '/', '?':
				return true
			}
		}
	}
	return false
}

// Approved return daftar caps yang sedang aktif untuk satu plugin.
// Dipakai oleh UI status.
func (b *Broker) Approved(pluginID string) []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	src := b.approvals[pluginID]
	out := make([]string, len(src))
	copy(out, src)
	return out
}
