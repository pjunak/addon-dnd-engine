package engine

import (
	"context"
	"encoding/json"
	"github.com/pjunak/addon-dnd-engine/character"
	"github.com/pjunak/addon-dnd-engine/internal/provider"
	"github.com/pjunak/addon-dnd-engine/internal/rules"
	"github.com/pjunak/ttrpg-codex/sdk/go/workerrpc"
	"testing"
)

func TestCharacterBoundaryUsesOneIdentifiedEvaluation(t *testing.T) {
	profile := engineRuleset(t)
	profile.Constants.Character = &rules.CharacterPolicy{MaximumLevel: 20, MinimumHPGain: 1, StandardArray: []int{15, 14, 13, 12, 10, 8}, RollDice: 4, RollSides: 6, RollKeep: 3, DistanceUnit: "ft"}
	data := &engineProvider{ruleset: profile, records: engineRecords{}}
	handler, _ := New(data)
	params, _ := json.Marshal(map[string]any{"contractVersion": character.ContractVersion, "inputs": character.Blank()})
	value, err := handler.HandleRPC(context.Background(), rpcRequest("evaluate-character", string(params)))
	if err != nil {
		t.Fatal(err)
	}
	response := value.(map[string]any)
	if response["identity"].(provider.Identity) != engineIdentity() || data.rulesetCalls != 1 {
		t.Fatal("evaluation lost identity or loaded rules twice")
	}
	result := response["evaluation"].(character.Result)
	if result.Ready || len(result.Issues) == 0 {
		t.Fatal("incomplete creation was advertised as valid")
	}
	data.rulesetError = workerrpc.NewRPCError(workerrpc.JSONRPCApplication, workerrpc.KindUnavailable, "Rules are unavailable", true, nil)
	_, err = handler.HandleRPC(context.Background(), rpcRequest("evaluate-character", string(params)))
	assertRPCError(t, err, workerrpc.KindUnavailable)
}
func TestCharacterBoundaryRejectsRetiredHandlersAndUnknownInputs(t *testing.T) {
	handler, _ := New(&engineProvider{ruleset: engineRuleset(t)})
	for _, method := range []string{"hydrate", "builder-plan", "apply-builder-choice", "reconcile-builder-decisions", "apply-play-change", "spell-options"} {
		_, err := handler.HandleRPC(context.Background(), rpcRequest(method, `{}`))
		if err == nil {
			t.Fatalf("retired handler %s remains callable", method)
		}
	}
	_, err := handler.HandleRPC(context.Background(), rpcRequest("evaluate-character", `{"contractVersion":"rules-character.v1","inputs":{"manualStats":{"STR":99}}}`))
	assertRPCError(t, err, workerrpc.KindInvalidRequest)
}
