// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Tab GUI: Proxy Pools → dok lock/gui/Proxy Pools.md  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"strings"

	"github.com/flowork-os/flowork_Router/internal/store"
)

type proxyDeployBody struct {
	Name      string `json:"name"`
	TargetURL string `json:"targetUrl"`
	APIKeyEnv string `json:"apiKeyEnv"`
	Project   string `json:"project"`
}

func cloudflareDeployHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body proxyDeployBody
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.TargetURL == "" {
		body.TargetURL = "https://your-tunnel.trycloudflare.com"
	}
	if body.Name == "" {
		body.Name = "flow-router-proxy"
	}
	script := `// Cloudflare Worker — flow_router edge proxy
export default {
  async fetch(request, env, ctx) {
    const target = ` + jsString(body.TargetURL) + `;
    const url = new URL(request.url);
    const upstream = new URL(target);
    upstream.pathname = url.pathname;
    upstream.search = url.search;
    const init = {
      method: request.method,
      headers: request.headers,
      body: ['GET','HEAD'].includes(request.method) ? null : request.body
    };
    return fetch(upstream.toString(), init);
  }
}`
	wranglerToml := `name = "` + body.Name + `"
main = "src/worker.js"
compatibility_date = "2026-01-01"
`
	cliAvailable := false
	if _, err := exec.LookPath("wrangler"); err == nil {
		cliAvailable = true
	}

	d, _ := store.Open()
	pool := &store.ProxyPool{
		Name:     body.Name,
		Rotation: "single",
		IsActive: false,
	}
	_ = store.UpsertProxyPool(d, pool)
	writeJSON(w, http.StatusOK, map[string]any{
		"platform":      "cloudflare-workers",
		"wranglerToml":  wranglerToml,
		"workerScript":  script,
		"cliAvailable":  cliAvailable,
		"deployCommand": "wrangler deploy",
		"setupSteps": []string{
			"1. mkdir -p " + body.Name + "/src && cd " + body.Name,
			"2. echo 'wrangler.toml' content above > wrangler.toml",
			"3. echo 'worker.js' content above > src/worker.js",
			"4. wrangler deploy",
		},
		"poolId": pool.ID,
	})
}

func denoDeployHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body proxyDeployBody
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.TargetURL == "" {
		body.TargetURL = "https://your-tunnel.trycloudflare.com"
	}
	if body.Project == "" {
		body.Project = "flow-router-proxy"
	}
	script := `// Deno Deploy proxy — flow_router edge
const TARGET = ` + jsString(body.TargetURL) + `;
Deno.serve(async (req) => {
  const url = new URL(req.url);
  const upstream = new URL(TARGET);
  upstream.pathname = url.pathname;
  upstream.search = url.search;
  return await fetch(upstream.toString(), {
    method: req.method,
    headers: req.headers,
    body: ['GET','HEAD'].includes(req.method) ? null : req.body,
  });
});`
	cliAvailable := false
	if _, err := exec.LookPath("deployctl"); err == nil {
		cliAvailable = true
	}
	d, _ := store.Open()
	pool := &store.ProxyPool{Name: body.Project, Rotation: "single"}
	_ = store.UpsertProxyPool(d, pool)
	writeJSON(w, http.StatusOK, map[string]any{
		"platform":      "deno-deploy",
		"script":        script,
		"cliAvailable":  cliAvailable,
		"deployCommand": "deployctl deploy --project=" + body.Project + " server.ts",
		"setupSteps": []string{
			"1. deno install -A jsr:@deno/deployctl  (if not installed)",
			"2. echo 'script' above > server.ts",
			"3. deployctl deploy --project=" + body.Project + " server.ts",
		},
		"poolId": pool.ID,
	})
}

func vercelDeployHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body proxyDeployBody
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.TargetURL == "" {
		body.TargetURL = "https://your-tunnel.trycloudflare.com"
	}
	if body.Project == "" {
		body.Project = "flow-router-proxy"
	}
	script := `// Vercel Edge Function — flow_router proxy
export const config = { runtime: 'edge' };
export default async function handler(req) {
  const TARGET = ` + jsString(body.TargetURL) + `;
  const url = new URL(req.url);
  const upstream = new URL(TARGET);
  upstream.pathname = url.pathname;
  upstream.search = url.search;
  return fetch(upstream.toString(), {
    method: req.method,
    headers: req.headers,
    body: ['GET','HEAD'].includes(req.method) ? null : req.body,
  });
}`
	cliAvailable := false
	if _, err := exec.LookPath("vercel"); err == nil {
		cliAvailable = true
	}
	d, _ := store.Open()
	pool := &store.ProxyPool{Name: body.Project, Rotation: "single"}
	_ = store.UpsertProxyPool(d, pool)
	writeJSON(w, http.StatusOK, map[string]any{
		"platform":      "vercel-edge",
		"script":        script,
		"cliAvailable":  cliAvailable,
		"deployCommand": "vercel deploy --prod",
		"setupSteps": []string{
			"1. npm i -g vercel  (if not installed)",
			"2. mkdir " + body.Project + "/api && cd " + body.Project,
			"3. echo 'handler' above > api/proxy.ts",
			"4. vercel deploy --prod",
		},
		"poolId": pool.ID,
	})
}

func jsString(s string) string {
	out := strings.ReplaceAll(s, `\`, `\\`)
	out = strings.ReplaceAll(out, `"`, `\"`)
	return `"` + out + `"`
}
