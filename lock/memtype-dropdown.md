# MEM_TYPE DROPDOWN — satu sumber kanonik (GUI=truth)

> Tab Brain dashboard :2402. Owner: Mr.Dev. 2026-06-26.

## MASALAH
Dua dropdown mem_type ("Add Knowledge" + "Typed Memory") di-hardcode TERPISAH & beda isi (12 vs 7) →
drawer ber-tipe non-kanonik (mis. 'knowledge') gak muncul di tab Typed → user bingung memorinya ilang.

## FIX (GUI=truth, anti hardcode)
Endpoint `GET /api/brain/mem-types` (`router/handlers_brain_memtypes.go`, FROZEN) balikin
`AllMemTypes` kanonik (`mem_type_registry.go`) ∪ mem_type yang BENERAN ada di `drawers` (dinamis,
anti-ilang). Kedua dropdown di-populate dari endpoint ini saat tab dibuka (`index.html` loadMemTypes).
Verified (CDP): dua dropdown = 16 opsi SAMA.

## FILE
- `router/handlers_brain_memtypes.go` (FROZEN) — handler.
- `router/routes.go` (non-frozen) — route.
- `router/web/static/index.html` (non-frozen GUI) — `loadMemTypes()` populate 2 dropdown.
- `router/internal/brain/mem_type_registry.go` (non-frozen) — `AllMemTypes` (sumber kanonik; nambah
  tipe = edit di sini → otomatis muncul di dropdown).
