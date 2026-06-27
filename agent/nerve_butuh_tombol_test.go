package main

import "testing"

func TestButuhTombol_Merge(t *testing.T) {
	var list []ButuhTombol
	a := ButuhTombol{Lokasi: "main.go", Alasan: "mau ubah X", Kind: "fix"}
	list = butuhTombolMerge(list, a)
	if len(list) != 1 {
		t.Fatalf("1 laporan harus masuk, dapat %d", len(list))
	}
	// dedupe: lokasi+alasan sama → ga dobel.
	list = butuhTombolMerge(list, a)
	if len(list) != 1 {
		t.Fatalf("laporan sama harus dedupe, dapat %d", len(list))
	}
	// beda alasan → masuk.
	list = butuhTombolMerge(list, ButuhTombol{Lokasi: "main.go", Alasan: "mau ubah Y", Kind: "fix"})
	if len(list) != 2 {
		t.Fatalf("laporan beda harus masuk, dapat %d", len(list))
	}
	// kosong → diabaikan.
	list = butuhTombolMerge(list, ButuhTombol{})
	if len(list) != 2 {
		t.Fatalf("laporan kosong harus diabaikan, dapat %d", len(list))
	}
}

func TestButuhTombol_Cap(t *testing.T) {
	var list []ButuhTombol
	for i := 0; i < butuhTombolMax+50; i++ {
		list = butuhTombolMerge(list, ButuhTombol{Lokasi: "f", Alasan: string(rune('A'+i%26)) + itoaSmall(i)})
	}
	if len(list) > butuhTombolMax {
		t.Fatalf("antrian harus di-cap <= %d, dapat %d", butuhTombolMax, len(list))
	}
}
