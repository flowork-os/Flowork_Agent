// FROZEN brain-core — desain abadi Flowork (Mesh/federation). Kalau ini bikin lo "nyasar": ini BY-DESIGN, baca lock/brain.md dulu. Jangan edit tanpa unfreeze owner.
// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flow_router
// Locked at: 2026-05-30
// Reason: Section 13 phase 2 — mDNS multicast discovery. Pure Go UDP
//   (no CGO). Announce every 30s + listen for peer announcements.
//   INVARIANT 2 cloud metadata IP block. Whitelist callback siap untuk
//   karma engine (Section 19) integration phase 3.
//
// discovery.go — Section 13 phase 2: mDNS LAN peer discovery.

package mesh

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"
)

const (
	mdnsIPv4Group     = "224.0.0.251"
	mdnsPort          = 5353
	floworkServiceTag = "_flowork._tcp"
	announceInterval  = 30 * time.Second
)

// AnnouncePacket — broadcast yang router pancarkan.
type AnnouncePacket struct {
	Service   string `json:"service"`
	PubKeyHex string `json:"pubkey"`
	Port      int    `json:"port"`
	Version   string `json:"version"`
	IsVirt    bool   `json:"virt"`
	Hostname  string `json:"hostname"`
	TS        int64  `json:"ts"`
}

// Discovery — manages mDNS multicast announce + listen + INVARIANT 2 block.
type Discovery struct {
	mu             sync.Mutex
	pubkey         []byte
	port           int
	version        string
	db             *sql.DB
	conn           *net.UDPConn
	stopped        bool
	WhitelistCheck func(pubkeyHex string) bool // optional gate
}

// NewDiscovery — caller wajib supply pubkey + db (peer registry).
func NewDiscovery(pubkey []byte, port int, version string, db *sql.DB) *Discovery {
	return &Discovery{
		pubkey:  pubkey,
		port:    port,
		version: version,
		db:      db,
	}
}

// Start — bind listener + spawn announce + listen goroutines. ctx cancel
// → close conn + return. Best-effort: kalau mDNS port busy, log + skip.
func (d *Discovery) Start(ctx context.Context) error {
	addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%d", mdnsIPv4Group, mdnsPort))
	if err != nil {
		return fmt.Errorf("discovery: resolve multicast: %w", err)
	}
	conn, err := net.ListenMulticastUDP("udp4", nil, addr)
	if err != nil {
		// Port busy — try announce-only via ephemeral port.
		log.Printf("[mesh-discovery] mDNS port %d busy — announce-only mode", mdnsPort)
		localAddr, _ := net.ResolveUDPAddr("udp4", ":0")
		conn, err = net.ListenUDP("udp4", localAddr)
		if err != nil {
			return fmt.Errorf("discovery: listen UDP: %w", err)
		}
	}
	d.conn = conn
	_ = conn.SetReadBuffer(65536)
	log.Printf("[mesh-discovery] started on %s (pubkey=%s, port=%d)",
		conn.LocalAddr(), hex.EncodeToString(d.pubkey)[:8], d.port)

	go d.announceLoop(ctx, addr)
	go d.listenLoop(ctx)
	return nil
}

// Stop — close conn + flag.
func (d *Discovery) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.stopped = true
	if d.conn != nil {
		_ = d.conn.Close()
	}
}

// TriggerAnnounce — POST /api/mesh/discover panggil ini buat manual broadcast.
func (d *Discovery) TriggerAnnounce() {
	addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%d", mdnsIPv4Group, mdnsPort))
	if err != nil {
		return
	}
	d.announce(addr)
}

func (d *Discovery) announceLoop(ctx context.Context, addr *net.UDPAddr) {
	d.announce(addr) // immediate first
	ticker := time.NewTicker(announceInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.announce(addr)
		}
	}
}

func (d *Discovery) announce(addr *net.UDPAddr) {
	d.mu.Lock()
	if d.stopped || d.conn == nil {
		d.mu.Unlock()
		return
	}
	d.mu.Unlock()

	hostname, _ := os.Hostname()
	pkt := AnnouncePacket{
		Service:   floworkServiceTag,
		PubKeyHex: hex.EncodeToString(d.pubkey),
		Port:      d.port,
		Version:   d.version,
		Hostname:  hostname,
		TS:        time.Now().Unix(),
	}
	data, err := json.Marshal(pkt)
	if err != nil {
		return
	}
	sendConn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		return
	}
	defer sendConn.Close()
	_, _ = sendConn.Write(data)
}

func (d *Discovery) listenLoop(ctx context.Context) {
	buf := make([]byte, 4096)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		_ = d.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, remote, err := d.conn.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			d.mu.Lock()
			stopped := d.stopped
			d.mu.Unlock()
			if stopped {
				return
			}
			continue
		}
		var pkt AnnouncePacket
		if err := json.Unmarshal(buf[:n], &pkt); err != nil {
			continue
		}
		if pkt.Service != floworkServiceTag {
			continue
		}
		// Skip self.
		if pkt.PubKeyHex == hex.EncodeToString(d.pubkey) {
			continue
		}
		peerIP := remote.IP.String()
		// INVARIANT 2 — block cloud metadata IPs.
		if IsCloudMetadataIP(peerIP) {
			log.Printf("[mesh-discovery] REJECT cloud metadata IP %s (INVARIANT 2)", peerIP)
			continue
		}
		// Whitelist callback gate (optional).
		if d.WhitelistCheck != nil && !d.WhitelistCheck(pkt.PubKeyHex) {
			log.Printf("[mesh-discovery] peer %s REJECTED by whitelist", pkt.PubKeyHex[:8])
			continue
		}
		// Persist to mesh_peers.
		_ = UpsertPeer(d.db, Peer{
			PubKeyHex: pkt.PubKeyHex,
			Hostname:  pkt.Hostname,
			IP:        peerIP,
			Port:      pkt.Port,
			Version:   pkt.Version,
			IsVirt:    pkt.IsVirt,
		})
		log.Printf("[mesh-discovery] peer found: %s@%s:%d",
			pkt.PubKeyHex[:8], peerIP, pkt.Port)
	}
}
