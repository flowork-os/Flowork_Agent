// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval. Owner: Aola Sahidin (Mr.Dev).
// Locked 2026-06-17 · P3 Scam Detector tool, owner-approved. Tested via real tool-dispatch
//   (capability net:fetch enforced correctly). Engine: internal/cryptocheck.
//
// check_token.go — P3 Scam Detector tool: vet a crypto token for scam/rug risk.
//
// Tipis (pola market.go): parse args → cryptocheck.CheckToken → Result. Capability
// net:fetch (SandboxRunV3 enforce). Data FAKTUAL dari GoPlus (EVM) / RugCheck (Solana),
// bukan model nebak → anti-halu. Read-only (TIDAK transaksi). Dipakai langsung atau oleh
// Scam Detector group (P3). Engine: internal/cryptocheck.
package builtins

import (
	"context"

	"flowork-gui/internal/cryptocheck"
	"flowork-gui/internal/tools"
)

func init() { tools.Register(&checkTokenTool{}) }

type checkTokenTool struct{}

func (checkTokenTool) Name() string       { return "check_token" }
func (checkTokenTool) Capability() string { return "net:fetch:*" }
func (checkTokenTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Crypto scam/rug safety check for ONE token. Pulls FACTUAL on-chain security data (GoPlus for EVM chains, RugCheck for Solana — no API key) and returns red-flags + a risk verdict (SCAM / HIGH-RISK / CAUTION / LOOKS-OK). Read-only. Use to vet a token before trusting or recommending it. NEVER asserts a token is 'safe' — it only surfaces risks; absence of flags is not a guarantee.",
		Params: []tools.Param{
			{Name: "chain", Type: tools.ParamString, Description: "chain: eth, bsc, polygon, base, arbitrum, optimism, avalanche, fantom, or solana", Required: true},
			{Name: "address", Type: tools.ParamString, Description: "token contract address (EVM 0x… or Solana mint address)", Required: true},
		},
		Returns: "{chain, address, source, risk_level, score, flags:[{severity,label,detail}], summary, token}",
	}
}

func (checkTokenTool) Run(_ context.Context, args map[string]any) (tools.Result, error) {
	chain, _ := args["chain"].(string)
	addr, _ := args["address"].(string)
	rep, err := cryptocheck.CheckToken(chain, addr)
	if err != nil {
		return tools.Result{}, err
	}
	return tools.Result{Output: rep}, nil
}
