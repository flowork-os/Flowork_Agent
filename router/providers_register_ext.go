// providers_register_ext.go — PROVIDER PLUG-AND-PLAY (kayak plugin WordPress).
//
// Provider di Flowork = copot-pasang TANPA buka frozen:
//  1. KONEKSI (baseURL/key/model) = DATA murni di DB → tambah/aktif-nonaktif/hapus dari GUI.
//  2. PROTOKOL/dialect baru = file sibling internal/translator/{request,response}/<x>.go
//     + init(){ translator.Register(...) } → auto-load.
//  3. PROVIDER MEDIA baru (embedding/image/tts/stt) = file sibling
//     internal/providers/<kat>/<x>.go implement interface + init(){ Register(...) }.
//
// Tiap kategori provider "dipasang" cukup dengan blank-import paket-nya SEKALI di sini
// (file NON-frozen) → init() tiap provider jalan, daftar ke registry. Provider mati/ganti
// teknologi = hapus file sibling-nya / nonaktifin koneksi DB. Nol sentuh core.
//
// Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
package main

// embedding/image/tts udah di-import di main.go (frozen). STT ke-skip di sana →
// dipasang di sini (gap diperbaiki tanpa buka main.go).
import _ "github.com/flowork-os/flowork_Router/internal/providers/stt"
