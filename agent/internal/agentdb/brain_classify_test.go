package agentdb

import (
	"testing"
)

func TestMemTypeClassifierHook_AutoClassify(t *testing.T) {
	s := openTestStore(t)

	// Test case 1: User info classification
	content1 := "Aola Sahidin adalah pencipta dari Flowork OS"
	id1, added1, err := s.AddBrainDrawer(content1, "", "", "", "agent")
	if err != nil {
		t.Fatalf("AddBrainDrawer error: %v", err)
	}
	if !added1 {
		t.Fatalf("Expected drawer1 to be added")
	}

	drawer1, found1, err := s.GetBrainDrawer(id1)
	if err != nil || !found1 {
		t.Fatalf("Failed to get drawer1: %v", err)
	}
	if drawer1.MemType != memTypeUser {
		t.Errorf("Expected mem_type '%s', got '%s'", memTypeUser, drawer1.MemType)
	}

	// Test case 2: Project info classification
	content2 := "bug: fix panic in go-routine crash"
	id2, added2, err := s.AddBrainDrawer(content2, "", "", "", "agent")
	if err != nil {
		t.Fatalf("AddBrainDrawer error: %v", err)
	}
	if !added2 {
		t.Fatalf("Expected drawer2 to be added")
	}

	drawer2, found2, err := s.GetBrainDrawer(id2)
	if err != nil || !found2 {
		t.Fatalf("Failed to get drawer2: %v", err)
	}
	if drawer2.MemType != memTypeProject {
		t.Errorf("Expected mem_type '%s', got '%s'", memTypeProject, drawer2.MemType)
	}
}
