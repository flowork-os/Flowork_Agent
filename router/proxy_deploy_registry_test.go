package main

import "testing"

// TestProxyDeployRegistry buktiin target deploy plug-and-play: built-in (cloudflare/deno/vercel)
// via sibling terdaftar, dan target BARU bisa di-Register tanpa edit frozen.
func TestProxyDeployRegistry(t *testing.T) {
	for _, want := range []string{"cloudflare", "deno", "vercel"} {
		tg, ok := getProxyDeployTarget(want)
		if !ok {
			t.Fatalf("built-in %q harus terdaftar", want)
		}
		res := tg.Build(proxyDeployBody{TargetURL: "https://x.example", Project: "p", Name: "p"})
		if res["platform"] == nil {
			t.Fatalf("%q Build harus balik platform", want)
		}
	}
	RegisterProxyDeployTarget(ProxyDeployTarget{Name: "dummy-netlify", Build: func(b proxyDeployBody) map[string]any {
		return map[string]any{"platform": "netlify"}
	}})
	if _, ok := getProxyDeployTarget("dummy-netlify"); !ok {
		t.Fatal("target baru via Register harus muncul")
	}
}
