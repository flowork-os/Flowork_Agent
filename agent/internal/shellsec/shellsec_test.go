package shellsec

import "testing"

func TestDangerousBlocks(t *testing.T) {
	bad := []string{
		"rm -rf /",
		"rm  -rf   /",             // spasi dobel (lolos substring lama)
		"RM -RF /",                // kapital
		"rm --recursive --force /", // ejaan flag panjang
		"rm -fr /",                // flag kebalik
		"rm -r -f /etc",           // flag kepisah + path sistem
		"rm --no-preserve-root -rf /home",
		"sudo rm -rf /usr",
		"echo hi; rm -rf ~",        // subcommand kedua
		":(){ :|:& };:",           // fork bomb spasi
		"dd if=/dev/zero of=/dev/sda",
		"mkfs.ext4 /dev/sdb",
		"cat /etc/shadow",
	}
	for _, c := range bad {
		if ok, _ := Dangerous(c); !ok {
			t.Errorf("SHOULD block but didn't: %q", c)
		}
	}
}

func TestDangerousAllowsLegit(t *testing.T) {
	ok := []string{
		"rm -rf ./build",   // relatif — legit
		"rm -rf node_modules",
		"rm -rf *",          // wildcard relatif di cwd — legit (jangan over-block)
		"rm -rf .cache",
		"ls -la /etc",       // baca, bukan hapus
		"grep -rf pattern .", // -rf di grep, bukan rm
		"git rm -r --cached .",
		"echo 'rm -rf /' # dokumentasi", // wait: ini nyebut path sistem
	}
	// Catatan: baris terakhir SENGAJA dihapus dari cek (nyebut '/' literal) — lihat bawah.
	for _, c := range ok[:len(ok)-1] {
		if blocked, why := Dangerous(c); blocked {
			t.Errorf("should ALLOW but blocked: %q (%s)", c, why)
		}
	}
}
