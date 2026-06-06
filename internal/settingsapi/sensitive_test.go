package settingsapi

import "testing"

func TestIsSensitiveEnvKey(t *testing.T) {
	block := []string{"PATH", "LD_PRELOAD", "DYLD_INSERT_LIBRARIES", "FLOWORK_LOOPBACK_SECRET", "HOME", "IFS", "GIT_SSH", "NODE_OPTIONS"}
	for _, k := range block {
		if !IsSensitiveEnvKey(k) {
			t.Errorf("%q should be blocked", k)
		}
	}
	allow := []string{"ETHERSCAN_API_KEY", "OPENAI_API_KEY", "TELEGRAM_BOT_TOKEN", "MY_TOKEN"}
	for _, k := range allow {
		if IsSensitiveEnvKey(k) {
			t.Errorf("%q should be allowed", k)
		}
	}
}
