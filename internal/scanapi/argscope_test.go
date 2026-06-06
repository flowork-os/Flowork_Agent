package scanapi

import (
	"reflect"
	"sort"
	"testing"
)

func TestHostsInArg(t *testing.T) {
	cases := map[string][]string{
		"-u":                        nil,
		"-silent":                   nil,
		"https://victim.com/path":   {"victim.com"},
		"http://10.0.0.1:8080/x":    {"10.0.0.1"},
		"victim.com":                {"victim.com"},
		"sub.victim.com:443":        {"sub.victim.com"},
		"192.168.0.0/16":            {"192.168.0.0/16"},
		"8.8.8.8":                   {"8.8.8.8"},
		"User-Agent:":               nil,
		"json":                      nil,
		"-H":                        nil,
	}
	for in, want := range cases {
		got := hostsInArg(in)
		sort.Strings(got)
		if want == nil && len(got) == 0 {
			continue
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("hostsInArg(%q) = %v, want %v", in, got, want)
		}
	}
}
