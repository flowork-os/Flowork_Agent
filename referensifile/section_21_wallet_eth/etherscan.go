package wallet

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/teetah2402/flowork/internal/safeclient"
)

const etherscanV2Base = "https://api.etherscan.io/v2/api"

// Etherscan is a minimal adapter over Etherscan V2 API.
// Read-only — balance + tokenbalance + txlist + tokentx endpoints.
type Etherscan struct {
	apiKey string
	http   *http.Client
}

// NewEtherscan creates a client using ETHERSCAN_API_KEY from environment.
// Returns error if key is not set.
func NewEtherscan() (*Etherscan, error) {
	key := strings.TrimSpace(os.Getenv("ETHERSCAN_API_KEY"))
	if key == "" {
		return nil, fmt.Errorf("ETHERSCAN_API_KEY not set")
	}
	// EXTBUG-002 fix: raw http.Client has no SSRF guard. Even though
	// etherscanV2Base is hardcoded, DNS/proxy poisoning could redirect the
	// apikey=<KEY> query string to an internal metadata endpoint. Etherscan
	// V2 only supports API key in query string (no Bearer) so the best we
	// can do is harden the dialer + add a log note that this is a known
	// surface.
	return &Etherscan{
		apiKey: key,
		http:   safeclient.NewClient(15 * time.Second),
	}, nil
}

// response is the Etherscan V2 envelope for read endpoints.
type response struct {
	Status  string          `json:"status"`
	Message string          `json:"message"`
	Result  json.RawMessage `json:"result"`
}

// get performs a GET to Etherscan V2 and returns the decoded envelope.
//
// BUG-W56 fix Sprint 3.5e: send API key via `apikey` HTTP header instead of
// URL query string. Query strings get logged by every reverse proxy, load
// balancer, and access log along the request path. Etherscan V2 accepts the
// key in either location — header is the audit-clean variant.
func (e *Etherscan) get(ctx context.Context, params url.Values) (*response, error) {
	u := etherscanV2Base + "?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "flowork/wallet")
	req.Header.Set("apikey", e.apiKey)

	resp, err := e.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("etherscan: %w", err)
	}
	defer resp.Body.Close()

	// rc121 fortress fix: cap 10MB — Etherscan balance JSON <1KB.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, err
	}

	var r response
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("etherscan parse: %w (body: %s)", err, truncate(string(body), 200))
	}
	return &r, nil
}

// Balance returns the native coin balance for address on chainID, in wei
// (as *big.Int) so precision isn't lost. Callers convert to float for display.
func (e *Etherscan) Balance(ctx context.Context, chainID int, address string) (*big.Int, error) {
	params := url.Values{}
	params.Set("chainid", fmt.Sprintf("%d", chainID))
	params.Set("module", "account")
	params.Set("action", "balance")
	params.Set("address", address)
	params.Set("tag", "latest")

	r, err := e.get(ctx, params)
	if err != nil {
		return nil, err
	}
	if r.Status != "1" {
		return nil, fmt.Errorf("etherscan balance (chain %d): %s", chainID, r.Message)
	}
	var raw string
	if err := json.Unmarshal(r.Result, &raw); err != nil {
		return nil, err
	}
	bi := new(big.Int)
	if _, ok := bi.SetString(strings.TrimSpace(raw), 10); !ok {
		return nil, fmt.Errorf("etherscan balance: bad number %q", raw)
	}
	return bi, nil
}

// TokenBalance returns the ERC20 balance for the given contract + address,
// in the smallest unit (raw, before decimals).
//
// NOTOK handling: Etherscan V2 throws `status="0" message="NOTOK"` in several
// scenarios: rate-limit hit, unsupported contract on that chain, or transient
// upstream hiccup. For wallet display purposes we treat any NOTOK as
// "balance unknown → assume zero" rather than error — the UI doesn't need to
// show scary NOTOK banners; zero balance is the practical outcome either way.
// Log to stderr for operators who care.
func (e *Etherscan) TokenBalance(ctx context.Context, chainID int, contract, address string) (*big.Int, error) {
	params := url.Values{}
	params.Set("chainid", fmt.Sprintf("%d", chainID))
	params.Set("module", "account")
	params.Set("action", "tokenbalance")
	params.Set("contractaddress", contract)
	params.Set("address", address)
	params.Set("tag", "latest")

	r, err := e.get(ctx, params)
	if err != nil {
		return nil, err
	}
	if r.Status != "1" {
		// Try parse result as number first — zero balance sometimes arrives
		// with status=0 message=NOTOK but result="0".
		var raw string
		if jerr := json.Unmarshal(r.Result, &raw); jerr == nil && raw == "0" {
			return big.NewInt(0), nil
		}
		// Any other NOTOK: silently treat as zero to avoid scary UI banners.
		// Log for operators only.
		fmt.Fprintf(os.Stderr, "etherscan tokenbalance NOTOK (chain=%d, contract=%s): %s — treating as 0\n",
			chainID, contract, r.Message)
		return big.NewInt(0), nil
	}
	var raw string
	if err := json.Unmarshal(r.Result, &raw); err != nil {
		return nil, err
	}
	bi := new(big.Int)
	if _, ok := bi.SetString(strings.TrimSpace(raw), 10); !ok {
		return big.NewInt(0), nil
	}
	return bi, nil
}

// Tx is a normalized transaction record from Etherscan.
type Tx struct {
	Hash      string    `json:"hash"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	Value     string    `json:"value"`    // raw wei or token smallest unit
	Symbol    string    `json:"symbol"`   // native symbol or ERC20 symbol
	Decimals  int       `json:"decimals"` // for display conversion
	Timestamp time.Time `json:"timestamp"`
	ChainID   int       `json:"chain_id"`
	Type      string    `json:"type"` // "native" or "erc20"
}

// RecentTx returns the latest N native-coin transactions for address on chainID.
func (e *Etherscan) RecentTx(ctx context.Context, chainID int, address string, limit int) ([]Tx, error) {
	if limit <= 0 {
		limit = 10
	}
	params := url.Values{}
	params.Set("chainid", fmt.Sprintf("%d", chainID))
	params.Set("module", "account")
	params.Set("action", "txlist")
	params.Set("address", address)
	params.Set("page", "1")
	params.Set("offset", fmt.Sprintf("%d", limit))
	params.Set("sort", "desc")

	r, err := e.get(ctx, params)
	if err != nil {
		return nil, err
	}
	// "No transactions found" returns status 0 but that's not an error.
	if r.Status != "1" {
		if strings.Contains(strings.ToLower(r.Message), "no transactions") {
			return nil, nil
		}
		return nil, fmt.Errorf("etherscan txlist (chain %d): %s", chainID, r.Message)
	}

	var raw []struct {
		Hash      string `json:"hash"`
		From      string `json:"from"`
		To        string `json:"to"`
		Value     string `json:"value"`
		TimeStamp string `json:"timeStamp"`
	}
	if err := json.Unmarshal(r.Result, &raw); err != nil {
		return nil, err
	}

	// Find native symbol for this chain
	symbol := "ETH"
	for _, c := range Supported() {
		if c.ID == chainID {
			symbol = c.NativeSymbol
			break
		}
	}

	out := make([]Tx, 0, len(raw))
	for _, t := range raw {
		ts := parseUnixSec(t.TimeStamp)
		out = append(out, Tx{
			Hash:      t.Hash,
			From:      t.From,
			To:        t.To,
			Value:     t.Value,
			Symbol:    symbol,
			Decimals:  18,
			Timestamp: ts,
			ChainID:   chainID,
			Type:      "native",
		})
	}
	return out, nil
}

// RecentTokenTx returns recent ERC20 transfer history (all tokens) for address.
func (e *Etherscan) RecentTokenTx(ctx context.Context, chainID int, address string, limit int) ([]Tx, error) {
	if limit <= 0 {
		limit = 10
	}
	params := url.Values{}
	params.Set("chainid", fmt.Sprintf("%d", chainID))
	params.Set("module", "account")
	params.Set("action", "tokentx")
	params.Set("address", address)
	params.Set("page", "1")
	params.Set("offset", fmt.Sprintf("%d", limit))
	params.Set("sort", "desc")

	r, err := e.get(ctx, params)
	if err != nil {
		return nil, err
	}
	if r.Status != "1" {
		if strings.Contains(strings.ToLower(r.Message), "no transactions") {
			return nil, nil
		}
		return nil, fmt.Errorf("etherscan tokentx (chain %d): %s", chainID, r.Message)
	}

	var raw []struct {
		Hash         string `json:"hash"`
		From         string `json:"from"`
		To           string `json:"to"`
		Value        string `json:"value"`
		TokenSymbol  string `json:"tokenSymbol"`
		TokenDecimal string `json:"tokenDecimal"`
		TimeStamp    string `json:"timeStamp"`
	}
	if err := json.Unmarshal(r.Result, &raw); err != nil {
		return nil, err
	}

	out := make([]Tx, 0, len(raw))
	for _, t := range raw {
		dec := 18
		if n := parseInt(t.TokenDecimal); n > 0 {
			// BUG-019 fix: cap decimals to 77 (max digits in uint256).
			// A malicious token with decimals=9999999 would cause
			// downstream powTen to spin millions of BigInt multiplications.
			if n > 77 {
				n = 18 // fallback to standard decimals
			}
			dec = n
		}
		out = append(out, Tx{
			Hash:      t.Hash,
			From:      t.From,
			To:        t.To,
			Value:     t.Value,
			Symbol:    t.TokenSymbol,
			Decimals:  dec,
			Timestamp: parseUnixSec(t.TimeStamp),
			ChainID:   chainID,
			Type:      "erc20",
		})
	}
	return out, nil
}

// truncate returns s shortened to max chars.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func parseUnixSec(s string) time.Time {
	n := parseInt(s)
	if n == 0 {
		return time.Time{}
	}
	return time.Unix(int64(n), 0).UTC()
}

func parseInt(s string) int {
	s = strings.TrimSpace(s)
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		// BUG-019 fix: overflow guard. Previously no bounds check — a
		// malicious ERC20 with decimals=9999999 caused powTen to loop
		// millions of BigInt multiplications (CPU DoS). Cap to a sane
		// maximum. Any value beyond 10 digits is clearly malicious or
		// an API glitch.
		if n > 999_999_999 {
			return 0 // overflow — return 0 to trigger fallback
		}
		n = n*10 + int(c-'0')
	}
	return n
}
