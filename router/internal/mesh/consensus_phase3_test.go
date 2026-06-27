package mesh

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func meshTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.TempDir()+"/mesh.db?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	stmts := []string{
		`CREATE TABLE mesh_knowledge_inbox (id INTEGER PRIMARY KEY AUTOINCREMENT, packet_id TEXT NOT NULL UNIQUE,
		  origin_pubkey TEXT NOT NULL, drawer_content TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'shadow',
		  arrived_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE TABLE mesh_peer_karma (pubkey_hex TEXT PRIMARY KEY, karma REAL NOT NULL DEFAULT 0.5,
		  packets_promoted INTEGER NOT NULL DEFAULT 0, packets_dropped INTEGER NOT NULL DEFAULT 0, last_event_at TEXT)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func pkt(pid, pub, content string) Packet {
	return Packet{
		PacketID:     pid,
		OriginPubkey: pub,
		PayloadJSON:  fmt.Sprintf(`{"drawer_content":%q}`, content),
		TimestampNS:  time.Now().UnixNano(),
	}
}

// F1/L7: near-duplicate dari knowledge yg udah promoted → DROP (Duplicate), TANPA penalti.
func TestL7_NearDuplicate(t *testing.T) {
	db := meshTestDB(t)
	// fixture: 1 knowledge udah promoted.
	_, _ = db.Exec(`INSERT INTO mesh_knowledge_inbox (packet_id, origin_pubkey, drawer_content, status)
	                VALUES ('p0','peerA','Go is a statically typed compiled programming language','promoted')`)
	// near-same dari peer lain → L7 reject (dup).
	res := ProcessKnowledgePacket(db, pkt("p1", "peerB", "Go is a statically-typed, compiled programming language!"))
	if res.Status != StatusDropped || !res.Duplicate {
		t.Fatalf("near-dup harus Dropped+Duplicate, dapat status=%s dup=%v reason=%q", res.Status, res.Duplicate, res.Reason)
	}
}

// F2/L8: 1 peer untrusted (karma default) konten baru → FLAG → quarantine (nunggu consensus).
func TestL8_SinglePeerHeld(t *testing.T) {
	t.Setenv("FLOWORK_MESH_APPROVE", "auto") // L8 cuma kejangkau di mode auto; manual = L0 owner-queue (diuji terpisah)
	db := meshTestDB(t)
	res := ProcessKnowledgePacket(db, pkt("q1", "peerX", "the capital of France is Paris and it is in Europe"))
	if res.Status != StatusQuarantine {
		t.Fatalf("1 peer untrusted harus Quarantine (nunggu consensus), dapat %s reason=%q", res.Status, res.Reason)
	}
}

// F2/L8: 2 peer DISTINCT kirim near-same → consensus N-of-M tercapai → PROMOTED.
func TestL8_QuorumPromotes(t *testing.T) {
	t.Setenv("FLOWORK_MESH_APPROVE", "auto") // L8 cuma kejangkau di mode auto; manual = L0 owner-queue (diuji terpisah)
	db := meshTestDB(t)
	c := "binary search runs in logarithmic time on a sorted array"
	r1 := ProcessKnowledgePacket(db, pkt("k1", "peer1", c))
	if r1.Status != StatusQuarantine {
		t.Fatalf("peer ke-1 harus quarantine (1/2), dapat %s", r1.Status)
	}
	r2 := ProcessKnowledgePacket(db, pkt("k2", "peer2", c+" indeed")) // peer beda, near-same
	if r2.Status != StatusPromoted {
		t.Fatalf("peer ke-2 harus PROMOTED (consensus 2/2), dapat %s reason=%q", r2.Status, r2.Reason)
	}
}

// F2/L8: 1 peer TRUSTED (karma tinggi) → fast-path promote tanpa quorum penuh.
func TestL8_TrustedFastPath(t *testing.T) {
	t.Setenv("FLOWORK_MESH_APPROVE", "auto") // L8 cuma kejangkau di mode auto; manual = L0 owner-queue (diuji terpisah)
	db := meshTestDB(t)
	_, _ = db.Exec(`INSERT INTO mesh_peer_karma (pubkey_hex, karma) VALUES ('vip', 0.9)`)
	res := ProcessKnowledgePacket(db, pkt("t1", "vip", "merge sort is a stable divide and conquer sorting algorithm"))
	if res.Status != StatusPromoted {
		t.Fatalf("peer trusted (karma 0.9) harus PROMOTED fast-path, dapat %s reason=%q", res.Status, res.Reason)
	}
}

// Sybil-resist sederhana: 1 peer spam konten sama 2x ≠ consensus (DISTINCT pubkey).
func TestL8_SamePeerNoSelfQuorum(t *testing.T) {
	t.Setenv("FLOWORK_MESH_APPROVE", "auto") // L8 cuma kejangkau di mode auto; manual = L0 owner-queue (diuji terpisah)
	db := meshTestDB(t)
	c := "http uses tcp on port 80 by default for plaintext web traffic"
	_ = ProcessKnowledgePacket(db, pkt("s1", "spammer", c))
	res := ProcessKnowledgePacket(db, pkt("s2", "spammer", c+" yes")) // peer SAMA
	if res.Status == StatusPromoted {
		t.Fatalf("peer sama spam 2x JANGAN promote (bukan consensus), dapat %s", res.Status)
	}
}
