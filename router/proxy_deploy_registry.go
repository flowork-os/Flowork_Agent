// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Tab GUI: Proxy Pools (registry deploy plug-and-play) → dok lock/gui/Proxy Pools.md  ⚠️ FROZEN.
// Nambah target deploy: sibling proxy_<x>_ext.go + RegisterProxyDeployTarget. Cara: CARAFREEZE.MD
// (POLA A) + lock/plug-and-play.md. Pola freeze: lock/frozen-core.md

package main

import "sort"

type ProxyDeployTarget struct {
	Name   string
	CLIBin string
	Build  func(body proxyDeployBody) map[string]any
}

var proxyDeployTargets = map[string]ProxyDeployTarget{}

func RegisterProxyDeployTarget(t ProxyDeployTarget) {
	if t.Name == "" || t.Build == nil {
		return
	}
	proxyDeployTargets[t.Name] = t
}

func proxyDeployTargetNames() []string {
	names := make([]string, 0, len(proxyDeployTargets))
	for n := range proxyDeployTargets {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func getProxyDeployTarget(name string) (ProxyDeployTarget, bool) {
	t, ok := proxyDeployTargets[name]
	return t, ok
}
