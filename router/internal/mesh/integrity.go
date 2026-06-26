// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Integritas frozen-core (anti-tamper mesh) → dok lock/integrity.md  ⚠️ FROZEN — jangan edit.
// Gate via seam RegisterMeshFilter; switch FLOWORK_INTEGRITY_GATE. Pola: lock/frozen-core.md
//

package mesh

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
)

var freezeLineRe = regexp.MustCompile(`^([0-9a-f]{64})\s+(\S+\.go)$`)

var (
	integrityOnce  sync.Once
	coreClean      = true
	coreRoot       string
	coreCheckedCnt int
)

func manifestPath() string {
	if p := strings.TrimSpace(os.Getenv("FLOWORK_KERNEL_MANIFEST")); p != "" {
		return p
	}
	return "../KERNEL_FREEZE.md"
}

func computeIntegrity() {
	f, err := os.Open(manifestPath())
	if err != nil {
		coreClean, coreRoot, coreCheckedCnt = true, "", 0
		return
	}
	defer f.Close()

	clean := true
	var hashes []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		m := freezeLineRe.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		want, path := m[1], m[2]
		if !strings.HasPrefix(path, "../router/") {
			continue
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			clean = false
			continue
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != want {
			clean = false
		}
		hashes = append(hashes, want)
	}
	if len(hashes) == 0 {
		coreClean, coreRoot, coreCheckedCnt = true, "", 0
		return
	}
	sort.Strings(hashes)
	root := sha256.Sum256([]byte(strings.Join(hashes, "\n")))
	coreClean, coreRoot, coreCheckedCnt = clean, hex.EncodeToString(root[:]), len(hashes)
}

func CoreClean() bool { integrityOnce.Do(computeIntegrity); return coreClean }

func CoreRootHash() string { integrityOnce.Do(computeIntegrity); return coreRoot }

func CoreCheckedCount() int { integrityOnce.Do(computeIntegrity); return coreCheckedCnt }
