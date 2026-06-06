package scanapi

import (
	"reflect"
	"testing"
)

func TestNucleiExclusionArgs(t *testing.T) {
	disabled := map[string]bool{"nuclei:dns": true, "nuclei:file": true, "auditor:x": true}
	got := nucleiExclusionArgs(disabled, "/tpl")
	want := []string{"-exclude-templates", "/tpl/dns", "-exclude-templates", "/tpl/file"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("exclusion args: got %v want %v", got, want)
	}
	if nucleiExclusionArgs(disabled, "") != nil {
		t.Fatal("empty dir → harus nil")
	}
	if nucleiExclusionArgs(map[string]bool{}, "/tpl") != nil {
		t.Fatal("disabled kosong → harus nil")
	}
	if nucleiExclusionArgs(map[string]bool{"auditor:y": true}, "/tpl") != nil {
		t.Fatal("non-nuclei disabled → harus nil (auditor core ga punya bentuk dir)")
	}
}
