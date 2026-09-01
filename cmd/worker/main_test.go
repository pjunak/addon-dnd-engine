package main

import (
	"testing"

	"github.com/pjunak/addon-dnd-engine/internal/engine"
)

func TestAdvertisedMethodsMatchPublicService(t *testing.T) {
	want := []string{
		"context",
		"get-record",
		"query-records",
		"derive",
		"hydrate",
		"builder-plan",
		"apply-builder-choice",
		"reconcile-builder-decisions",
	}
	methods := advertisedMethods()
	if len(methods) != len(want) {
		t.Fatalf("advertised methods = %#v", methods)
	}
	for _, method := range want {
		name := "service/dnd5e.rules-engine/" + method
		if methods[name] != engine.ContractVersion {
			t.Fatalf("%s version = %q", name, methods[name])
		}
	}
}
