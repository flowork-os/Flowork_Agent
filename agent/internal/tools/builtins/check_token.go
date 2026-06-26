// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

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
