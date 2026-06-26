// proxy_targets_ext.go — target deploy built-in (cloudflare/deno/vercel) via registry
// (NON-frozen, deletable). Reuse jsString + proxyDeployBody dari handler frozen. Nambah target
// BARU (netlify/aws/fly) = tambah di sini atau file sibling proxy_<x>_ext.go + RegisterProxyDeployTarget.
// Hapus file ini → 3 target ilang dari registry, build TETAP OK (endpoint khusus lama tetap jalan).
//
// Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
package main

func init() {
	RegisterProxyDeployTarget(ProxyDeployTarget{Name: "cloudflare", CLIBin: "wrangler", Build: func(b proxyDeployBody) map[string]any {
		if b.TargetURL == "" {
			b.TargetURL = "https://your-tunnel.trycloudflare.com"
		}
		if b.Name == "" {
			b.Name = "flow-router-proxy"
		}
		script := "// Cloudflare Worker — flow_router edge proxy\nexport default {\n  async fetch(request, env, ctx) {\n    const target = " + jsString(b.TargetURL) + ";\n    const url = new URL(request.url);\n    const upstream = new URL(target);\n    upstream.pathname = url.pathname; upstream.search = url.search;\n    return fetch(upstream.toString(), { method: request.method, headers: request.headers, body: ['GET','HEAD'].includes(request.method) ? null : request.body });\n  }\n}"
		return map[string]any{
			"platform":      "cloudflare-workers",
			"wranglerToml":  "name = \"" + b.Name + "\"\nmain = \"src/worker.js\"\ncompatibility_date = \"2026-01-01\"\n",
			"workerScript":  script,
			"deployCommand": "wrangler deploy",
			"setupSteps":    []string{"1. mkdir -p " + b.Name + "/src && cd " + b.Name, "2. tulis wrangler.toml", "3. tulis src/worker.js", "4. wrangler deploy"},
		}
	}})

	RegisterProxyDeployTarget(ProxyDeployTarget{Name: "deno", CLIBin: "deployctl", Build: func(b proxyDeployBody) map[string]any {
		if b.TargetURL == "" {
			b.TargetURL = "https://your-tunnel.trycloudflare.com"
		}
		if b.Project == "" {
			b.Project = "flow-router-proxy"
		}
		script := "// Deno Deploy proxy — flow_router edge\nconst TARGET = " + jsString(b.TargetURL) + ";\nDeno.serve(async (req) => {\n  const url = new URL(req.url); const upstream = new URL(TARGET);\n  upstream.pathname = url.pathname; upstream.search = url.search;\n  return await fetch(upstream.toString(), { method: req.method, headers: req.headers, body: ['GET','HEAD'].includes(req.method) ? null : req.body });\n});"
		return map[string]any{
			"platform":      "deno-deploy",
			"script":        script,
			"deployCommand": "deployctl deploy --project=" + b.Project + " server.ts",
			"setupSteps":    []string{"1. deno install -A jsr:@deno/deployctl", "2. tulis server.ts", "3. deployctl deploy --project=" + b.Project + " server.ts"},
		}
	}})

	RegisterProxyDeployTarget(ProxyDeployTarget{Name: "vercel", CLIBin: "vercel", Build: func(b proxyDeployBody) map[string]any {
		if b.TargetURL == "" {
			b.TargetURL = "https://your-tunnel.trycloudflare.com"
		}
		if b.Project == "" {
			b.Project = "flow-router-proxy"
		}
		script := "// Vercel Edge Function — flow_router proxy\nexport const config = { runtime: 'edge' };\nexport default async function handler(req) {\n  const TARGET = " + jsString(b.TargetURL) + ";\n  const url = new URL(req.url); const upstream = new URL(TARGET);\n  upstream.pathname = url.pathname; upstream.search = url.search;\n  return fetch(upstream.toString(), { method: req.method, headers: req.headers, body: ['GET','HEAD'].includes(req.method) ? null : req.body });\n}"
		return map[string]any{
			"platform":      "vercel-edge",
			"script":        script,
			"deployCommand": "vercel deploy --prod",
			"setupSteps":    []string{"1. npm i -g vercel", "2. tulis api/proxy.ts", "3. vercel deploy --prod"},
		}
	}})
}
