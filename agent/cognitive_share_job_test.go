package main

import "testing"

func TestCleanForShare(t *testing.T) {
	if !cleanForShare("Go is a statically typed programming language", nil) {
		t.Error("fakta umum bersih harus boleh share")
	}
	bad := map[string]string{
		"path":  "config at /home/user1/secret.txt",
		"brand": "this uses the claude model",
		"empty": "   ",
		"token": "key ghp_AbCd1234EfGh5678IjKl",
	}
	for name, b := range bad {
		if cleanForShare(b, nil) {
			t.Errorf("%s: konten ga-aman lolos gerbang: %q", name, b)
		}
	}
	// Nama owner ke-redaksi → konten berubah → KETAT: jangan share.
	if cleanForShare("Alpha's private decision rule", []string{"Alpha"}) {
		t.Error("konten dgn nama owner harus di-block (D8)")
	}
}
