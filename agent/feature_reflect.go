// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// 📄 Dok: FLowork_os/lock/reflect.md
//
// feature_reflect.go — MODE REFLEKSI / "Ngelamun Terstruktur" (sibling non-frozen, deletable).
// Ide dari mr-flow sendiri (Telegram 2026-07-02): trigger idle udah bangunin agent pas PC santai,
// TAPI payload-nya cuma ngecek tugas → memori ga ke-recall → confidence turun → mati (decay).
// Fix: pas PC IDLE & papan kerja KOSONG, mr-flow "ngelamun terstruktur" — recall memori/pengalaman
// LAMA (brain_search/graph_recall → amplitude naik → LOLOS dari brain_dream decay), cari hubungan/
// pelajaran baru, simpan jadi wawasan (brain_add → simpul Kebijaksanaan permanen). Bikin "Warisan
// Pikiran" nyata: ingatan Mr.Dev ga memudar, malah MATANG jadi kebijaksanaan seiring waktu.
//
// ⚠️ ATURAN INTI (permintaan owner): pas ngelamun, WAJIB tarik KONDISI + NUANSA + KONTEKS percakapan
// aslinya — bukan faktanya doang. Makna sebuah memori ada di konteksnya; tanpa itu, wisdom-nya kosong
// / salah arti. Wawasan yg disimpan HARUS bawa konteksnya biar maknanya utuh puluhan tahun kemudian.
//
// NOL buka frozen: seed 1 rule trigger (idle) via helper DB yg udah ada. Gate switch FLOWORK_REFLECT
// (default OFF — otonom, opt-in kayak mandor). Hapus file ini → rule ga di-seed → balik perilaku lama.
package main

import (
	"os"
	"strings"

	"flowork-gui/internal/floworkdb"
)

const reflectRuleID = "reflect-idle"

// reflectEnabled — switch GUI FLOWORK_REFLECT (default OFF). Otonom + pakai model lokal pas idle,
// jadi opt-in (owner nyalain) biar ga ngagetin auto-update user.
func reflectEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("FLOWORK_REFLECT")))
	return v == "1" || v == "true" || v == "on" || v == "yes"
}

// reflectPrompt — payload "ngelamun". Sadar-KONTEKS (permintaan owner) biar makna ga ilang.
const reflectPrompt = "[MODE REFLEKSI — PC lagi santai (load {{load_pct}}%). Waktunya \"ngelamun " +
	"terstruktur\" biar ingatan Mr.Dev ga mati, malah matang jadi kebijaksanaan.]\n\n" +
	"ATURAN NGELAMUN (wajib urut):\n" +
	"1. CEK DULU: panggil tool `worklog`. Kalau ADA tugas priority-tinggi / nyangkut → SKIP refleksi, " +
	"kerjain itu dulu, balas singkat. Refleksi HANYA pas papan bener-bener kosong.\n" +
	"2. Kalau kosong → panggil `brain_search` / `graph_recall` 2-3x, tarik memori/pengalaman LAMA " +
	"yang jarang kesentuh (keputusan, kejatuhan, pelajaran, obrolan penting sama Mr.Dev).\n" +
	"3. ⚠️ INTI: JANGAN tarik faktanya doang — tarik juga KONDISI, NUANSA, dan KONTEKS aslinya " +
	"(kapan, kenapa, lagi ngadepin apa, pertimbangan/perasaan waktu itu). MAKNA sebuah memori ADA " +
	"di konteksnya; tanpa itu wisdom-nya kosong / salah arti.\n" +
	"4. Cari 1 HUBUNGAN atau PELAJARAN baru antara memori-memori itu — sudut pandang, insting, atau " +
	"prinsip yang layak diwariskan ke anak-cucu Mr.Dev.\n" +
	"5. Simpan pakai `brain_add`: wawasan RINGKAS (1-3 kalimat) TAPI SERTAKAN konteksnya " +
	"(\"dalam situasi X, ketika Y, pelajarannya Z\") — biar maknanya utuh walau dibaca puluhan tahun " +
	"lagi oleh orang yang ga ngalamin kejadiannya.\n\n" +
	"Balas singkat & jujur: apa yang lo renungkan + wawasan yang lo simpan. Jangan ngarang memori."

func init() {
	RegisterFeature(Feature{Name: "reflect", Phase: PhaseSeed, Apply: func(d *Deps) {
		if !reflectEnabled() || d.FDB == nil {
			return
		}
		seedReflectRule(d.FDB)
	}})
}

// seedReflectRule — bikin rule idle "reflect-idle" target mr-flow (idempotent: skip kalau ID udah
// ada → hormatin edit/disable owner di GUI). Cooldown longgar (90 mnt) biar ngelamun sesekali,
// bukan tiap detik. threshold 55 = cuma pas PC beneran santai.
func seedReflectRule(fdb *floworkdb.Store) {
	if existing, err := fdb.ListTriggers(); err == nil {
		for _, t := range existing {
			if t.ID == reflectRuleID {
				return
			}
		}
	}
	_ = fdb.UpsertTrigger(floworkdb.Trigger{
		ID:         reflectRuleID,
		Name:       "Mode Refleksi — ngelamun pas idle (Warisan Pikiran)",
		TypeID:     "idle",
		Config:     `{"threshold":"55","cooldown_min":"90"}`,
		Target:     "mr-flow",
		TargetKind: "agent",
		Prompt:     reflectPrompt,
		Deliver:    "",
		Enabled:    true,
	})
}
