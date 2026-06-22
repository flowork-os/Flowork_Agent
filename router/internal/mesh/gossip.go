// FROZEN brain-core — desain abadi Flowork (Mesh/federation). Kalau ini bikin lo "nyasar": ini BY-DESIGN, baca lock/brain.md dulu. Jangan edit tanpa unfreeze owner.
// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flow_router
// Locked at: 2026-05-30
// Reason: Section 15 phase 2 gossip protocol. Push to 3 random peers
//   every 10s. Dedupe via mesh_gossip_state. 2-of-3 BFT broadcast hook.
//   Phase 3 (anti-entropy pull, gossip_signed envelope) → tambah file.
//
// gossip.go — Section 15 phase 2: push gossip + dedupe + BFT broadcast.

package mesh

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	mathrand "math/rand"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	gossipFanout   = 3
	gossipInterval = 10 * time.Second
	pushTimeout    = 5 * time.Second
)

// GossipEngine — periodic push pending packets ke random peers.
type GossipEngine struct {
	db       *sql.DB
	httpCli  *http.Client
	stop     chan struct{}
	interval time.Duration
	fanout   int
}

func NewGossipEngine(db *sql.DB) *GossipEngine {
	return &GossipEngine{
		db:       db,
		httpCli:  &http.Client{Timeout: pushTimeout},
		interval: gossipInterval,
		fanout:   gossipFanout,
	}
}

func (g *GossipEngine) Start(ctx context.Context) {
	g.stop = make(chan struct{})
	log.Printf("[gossip] engine started — fanout=%d interval=%s", g.fanout, g.interval)
	go g.loop(ctx)
}

func (g *GossipEngine) Stop() {
	if g.stop != nil {
		close(g.stop)
	}
}

func (g *GossipEngine) loop(ctx context.Context) {
	timer := time.NewTimer(20 * time.Second) // warm-up
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-g.stop:
			return
		case <-timer.C:
			g.tick(ctx)
			timer.Reset(g.interval)
		}
	}
}

func (g *GossipEngine) tick(ctx context.Context) {
	peers, perr := selectRandomPeers(g.db, g.fanout)
	if perr != nil || len(peers) == 0 {
		return
	}
	pending, lerr := ListPendingPackets(g.db, 10)
	if lerr != nil {
		return
	}
	for _, pkt := range pending {
		// Dedup check via mesh_gossip_state.
		if alreadyForwarded(g.db, pkt.PacketID) {
			continue
		}
		forwarded := []string{}
		for _, peer := range peers {
			if pushToPeer(ctx, g.httpCli, peer, pkt) {
				forwarded = append(forwarded, peer.PubKeyHex)
			}
		}
		recordGossipForward(g.db, pkt.PacketID, forwarded)
		if len(forwarded) > 0 {
			_ = MarkProcessed(g.db, pkt.PacketID)
		}
	}
}

// selectRandomPeers — N random peers dari mesh_peers WHERE blocked=0.
func selectRandomPeers(db *sql.DB, n int) ([]Peer, error) {
	peers, err := ListPeers(db, false)
	if err != nil {
		return nil, err
	}
	if len(peers) <= n {
		return peers, nil
	}
	// Fisher-Yates partial.
	rng := mathrand.New(mathrand.NewSource(time.Now().UnixNano()))
	for i := 0; i < n; i++ {
		j := i + rng.Intn(len(peers)-i)
		peers[i], peers[j] = peers[j], peers[i]
	}
	return peers[:n], nil
}

func pushToPeer(ctx context.Context, cli *http.Client, peer Peer, pkt Packet) bool {
	if peer.IP == "" {
		return false
	}
	url := fmt.Sprintf("http://%s:%d/api/mesh/packet", peer.IP, peer.Port)
	body, _ := json.Marshal(pkt)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := cli.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// alreadyForwarded — check mesh_gossip_state.
func alreadyForwarded(db *sql.DB, packetID string) bool {
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM mesh_gossip_state WHERE packet_id = ?`, packetID).Scan(&n)
	return n > 0
}

// recordGossipForward — INSERT to mesh_gossip_state.
func recordGossipForward(db *sql.DB, packetID string, forwarded []string) {
	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = db.Exec(
		`INSERT OR IGNORE INTO mesh_gossip_state (packet_id, seen_at, forwarded_to)
		 VALUES (?, ?, ?)`,
		packetID, now, strings.Join(forwarded, ","))
}

// =============================================================================
// 2-of-3 BFT broadcast hook
// =============================================================================

// BFTBroadcast — emergency broadcast yang butuh consensus ≥2 trusted signer.
// Phase 2 stub: collect signature dari N=3 peer. Phase 3 (actual consensus
// state machine) → tambah file baru.
type BFTBroadcaster struct {
	mu         sync.Mutex
	pending    map[string][]string // packet_id → signature list
	threshold  int                  // 2-of-3
}

func NewBFTBroadcaster() *BFTBroadcaster {
	return &BFTBroadcaster{
		pending:   map[string][]string{},
		threshold: 2,
	}
}

// SubmitSignature — peer submit signature buat packet_id. Return true
// kalau threshold reached + caller bisa broadcast.
func (b *BFTBroadcaster) SubmitSignature(packetID, signature string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	sigs := b.pending[packetID]
	for _, s := range sigs {
		if s == signature {
			return false
		}
	}
	sigs = append(sigs, signature)
	b.pending[packetID] = sigs
	return len(sigs) >= b.threshold
}

// Status — return current signer count.
func (b *BFTBroadcaster) Status(packetID string) (int, int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.pending[packetID]), b.threshold
}
