package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/pjunak/addon-dnd-engine/character"
	"github.com/pjunak/addon-dnd-engine/internal/rules"
	"github.com/pjunak/ttrpg-codex/sdk/go/workerrpc"
	"strings"
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
	response := value.(characterResponse)
	if response.Identity != engineIdentity() || data.rulesetCalls != 1 {
		t.Fatal("evaluation lost identity or loaded rules twice")
	}
	result := response.Evaluation
	if body, _ := json.Marshal(response); strings.Contains(string(body), "null") && !strings.Contains(string(body), `"policy":null`) {
		if !nullsAllowed(body) {
			t.Fatalf("response has null lists: %s", body)
		}
	}
	if result.Ready || len(result.Issues) == 0 {
		t.Fatal("incomplete creation was advertised as valid")
	}
	data.rulesetError = workerrpc.NewRPCError(workerrpc.JSONRPCApplication, workerrpc.KindUnavailable, "Rules are unavailable", true, nil)
	_, err = handler.HandleRPC(context.Background(), rpcRequest("evaluate-character", string(params)))
	assertRPCError(t, err, workerrpc.KindUnavailable)
}
func TestCharacterBoundaryNamesAnOversizedRequest(t *testing.T) {
	profile := engineRuleset(t)
	profile.Constants.Character = &rules.CharacterPolicy{MaximumLevel: 20, MinimumHPGain: 1, StandardArray: []int{15, 14, 13, 12, 10, 8}, RollDice: 4, RollSides: 6, RollKeep: 3, DistanceUnit: "ft"}
	handler, _ := New(&engineProvider{ruleset: profile, records: engineRecords{}})
	request := func(notes int) []byte {
		inputs := character.Blank()
		inputs.Play.Inventory = []character.Item{{ID: "journal", Name: "Journal", Quantity: 1, Location: "carried", Notes: strings.Repeat("x", notes)}}
		params, _ := json.Marshal(map[string]any{"contractVersion": character.ContractVersion, "inputs": inputs})
		return params
	}
	exact := request(character.MaxRequestBytes - len(request(0)))
	if len(exact) != character.MaxRequestBytes {
		t.Fatalf("fixture is %d bytes", len(exact))
	}
	if _, err := handler.HandleRPC(context.Background(), rpcRequest("evaluate-character", string(exact))); err != nil {
		t.Fatalf("a request at the limit was rejected: %v", err)
	}
	over := request(character.MaxRequestBytes - len(request(0)) + 1)
	_, err := handler.HandleRPC(context.Background(), rpcRequest("character-play", string(over)))
	assertRPCError(t, err, workerrpc.KindInvalidRequest)
	var failure *workerrpc.RPCError
	_ = errors.As(err, &failure)
	if want := fmt.Sprintf("character request is %d bytes; the limit is %d bytes", len(over), character.MaxRequestBytes); failure.Data.Message != want {
		t.Fatalf("oversized request answered %q, want %q", failure.Data.Message, want)
	}
}
func TestCharacterBoundaryRejectsUnknownInputs(t *testing.T) {
	handler, _ := New(&engineProvider{ruleset: engineRuleset(t)})
	_, err := handler.HandleRPC(context.Background(), rpcRequest("evaluate-character", `{"contractVersion":"rules-character.v2","inputs":{"manualStats":{"STR":99}}}`))
	assertRPCError(t, err, workerrpc.KindInvalidRequest)
	for _, value := range []string{`"one"`, `1`, `[1]`, `{}`} {
		_, err := handler.HandleRPC(context.Background(), rpcRequest("evaluate-character", `{"contractVersion":"rules-character.v2","inputs":{"play":{"quickUse":`+value+`}}}`))
		assertRPCError(t, err, workerrpc.KindInvalidRequest)
	}
	for _, play := range []string{
		`{"containers":"pack"}`, `{"containers":[1]}`, `{"containers":[{"id":"pack","name":"Pack","capacity":99}]}`,
		`{"containers":[{"id":"pack","name":"Pack","parentId":"pouch"}]}`, `{"inventory":[{"containerId":5}]}`,
	} {
		_, err := handler.HandleRPC(context.Background(), rpcRequest("evaluate-character", `{"contractVersion":"rules-character.v2","inputs":{"play":`+play+`}}`))
		assertRPCError(t, err, workerrpc.KindInvalidRequest)
	}
	for _, value := range []string{`"true"`, `1`, `[]`, `{}`} {
		_, err := handler.HandleRPC(context.Background(), rpcRequest("evaluate-character", `{"contractVersion":"rules-character.v2","inputs":{"play":{"inspiration":`+value+`}}}`))
		assertRPCError(t, err, workerrpc.KindInvalidRequest)
	}
	for _, value := range []string{`true`, `"held"`, `[1]`, `[{"id":1,"level":1}]`, `[{"id":"held","level":1.5}]`, `[{"id":"held","level":1,"expiresAt":"tomorrow"}]`} {
		_, err := handler.HandleRPC(context.Background(), rpcRequest("evaluate-character", `{"contractVersion":"rules-character.v2","inputs":{"play":{"conditions":`+value+`}}}`))
		assertRPCError(t, err, workerrpc.KindInvalidRequest)
	}
	for _, value := range []string{
		`"blade"`, `1`, `[]`, `{"main":1}`, `{"off":[]}`, `{"grip":true}`, `{"extra":true}`,
		`{"suspendedOff":"shield"}`, `{"suspendedOff":{"itemId":2}}`, `{"suspendedOff":{"expectedItemSha256":[]}}`,
		`{"suspendedOff":{"bodyPlacement":false}}`, `{"suspendedOff":{"extra":true}}`,
	} {
		_, err := handler.HandleRPC(context.Background(), rpcRequest("evaluate-character", `{"contractVersion":"rules-character.v2","inputs":{"play":{"hands":`+value+`}}}`))
		assertRPCError(t, err, workerrpc.KindInvalidRequest)
	}
}

// nullsAllowed reports whether every null in a response is a nullable field.
func nullsAllowed(body []byte) bool {
	for _, field := range []string{"armorRestrictions", "restrictions", "hitDie", "pact", "versatileDamage", "exclusiveGroup", "resource", "level", "free", "castingAbility", "castAtLevel", "spellLevel", "maxSpellLevel", "default", "selected", "requires", "value", "ability"} {
		body = []byte(strings.ReplaceAll(string(body), `"`+field+`":null`, ""))
	}
	return !strings.Contains(string(body), "null")
}
