// Package wallet reads blockchain balance via Etherscan V2 API and
// aggregates native + ERC20 + USD price so the GUI panel shows the same
// view Ayah sees in Trust Wallet.
//
// Spec: roadmap/G-8-WALLET-PARITY-SPEC.md
package wallet

// Chain identifies one blockchain network supported by Etherscan V2 API.
type Chain struct {
	ID           int    // chainid param for Etherscan V2
	Name         string // human-readable
	NativeSymbol string // ETH / POL / ARB
	NativeCGID   string // CoinGecko ID for price lookup
}

// Supported returns the chains verified to work on Etherscan V2 free tier
// (2026-04-18 audit). BSC/Optimism/Base return NOTOK and need paid tier.
func Supported() []Chain {
	return []Chain{
		{ID: 1, Name: "Ethereum", NativeSymbol: "ETH", NativeCGID: "ethereum"},
		{ID: 137, Name: "Polygon", NativeSymbol: "POL", NativeCGID: "matic-network"},
		{ID: 42161, Name: "Arbitrum", NativeSymbol: "ETH", NativeCGID: "ethereum"},
	}
}

// Token describes one ERC20 contract to monitor on a given chain.
type Token struct {
	ChainID  int
	Symbol   string
	Contract string
	Decimals int
	CGID     string // CoinGecko ID for price
}

// MonitoredTokens returns the ERC20 tokens worth polling balance for on
// each supported chain. Stablecoins first (USDT/USDC/DAI) because that's
// where freelance earnings typically land.
//
// Ayah can extend this via env var WALLET_EXTRA_TOKENS_<CHAIN_ID> if a
// new stablecoin gains traction in the future.
func MonitoredTokens() []Token {
	return []Token{
		// Ethereum (chainid=1)
		{ChainID: 1, Symbol: "USDT", Contract: "0xdAC17F958D2ee523a2206206994597C13D831ec7", Decimals: 6, CGID: "tether"},
		{ChainID: 1, Symbol: "USDC", Contract: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", Decimals: 6, CGID: "usd-coin"},
		{ChainID: 1, Symbol: "DAI", Contract: "0x6B175474E89094C44Da98b954EedeAC495271d0F", Decimals: 18, CGID: "dai"},

		// Polygon (chainid=137)
		{ChainID: 137, Symbol: "USDT", Contract: "0xc2132D05D31c914a87C6611C10748AEb04B58e8F", Decimals: 6, CGID: "tether"},
		{ChainID: 137, Symbol: "USDC", Contract: "0x3c499c542cEF5E3811e1192ce70d8cC03d5c3359", Decimals: 6, CGID: "usd-coin"},
		{ChainID: 137, Symbol: "DAI", Contract: "0x8f3Cf7ad23Cd3CaDbD9735AFf958023239c6A063", Decimals: 18, CGID: "dai"},

		// Arbitrum (chainid=42161)
		{ChainID: 42161, Symbol: "USDT", Contract: "0xFd086bC7CD5C481DCC9C85ebE478A1C0b69FCbb9", Decimals: 6, CGID: "tether"},
		{ChainID: 42161, Symbol: "USDC", Contract: "0xaf88d065e77c8cC2239327C5EDb3A432268e5831", Decimals: 6, CGID: "usd-coin"},
	}
}

// TokensFor returns monitored tokens filtered by chain ID.
func TokensFor(chainID int) []Token {
	var out []Token
	for _, t := range MonitoredTokens() {
		if t.ChainID == chainID {
			out = append(out, t)
		}
	}
	return out
}

// AllCGIDs returns unique CoinGecko IDs needed for price fetch across all
// chains + tokens (dedupe). One price call covers everything.
func AllCGIDs() []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range Supported() {
		if !seen[c.NativeCGID] {
			seen[c.NativeCGID] = true
			out = append(out, c.NativeCGID)
		}
	}
	for _, t := range MonitoredTokens() {
		if !seen[t.CGID] {
			seen[t.CGID] = true
			out = append(out, t.CGID)
		}
	}
	return out
}
