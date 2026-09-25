package rules

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/pjunak/addon-dnd-engine/character"
)

func quickUseItems() []character.Item {
	return []character.Item{
		{ID: "one", Name: "Supplies", Quantity: 2, Location: "carried", Notes: "Keep notes", Acquisition: "Keep provenance"},
		{ID: "duplicate-copy", Name: "Supplies", Quantity: 3, Location: "stored", Notes: "Other copy"},
		{ID: "empty", Name: "Depleted supplies", Location: "carried"},
	}
}

func TestQuickUsePreservesInstanceIdentityAndSpentState(t *testing.T) {
	input, records, profile := characterFixture(t)
	input.Play.Inventory = quickUseItems()
	input.Play.QuickUse = []string{"duplicate-copy", "one", "empty"}
	before := cloneCharacter(input)
	caller := input
	result := EvaluateCharacter(input, records, profile)
	if !result.Ready || !reflect.DeepEqual(result.Inputs, input) || object(result.Guidance["authoredPlay"])["quickUse"] != true {
		t.Fatal("pins were not accepted as authored state", result.Issues)
	}
	pins := objects(result.Sheet["quickUse"])
	if text(pins[0]["itemId"]) != "duplicate-copy" || text(pins[0]["reason"]) != "stored" || text(pins[2]["reason"]) != "empty" || !truth(pins[1]["available"]) {
		t.Fatal("lost order or unavailable pins", pins)
	}
	for _, quantity := range []int{1, 0} {
		used, err := ApplyCharacterPlay(input, Object{"operation": "consume-item", "itemId": "one"}, records, profile)
		if err != nil || !used.Ready {
			t.Fatal(err, used.Issues)
		}
		expected := cloneCharacter(input)
		expected.Play.Inventory[0].Quantity = quantity
		if !reflect.DeepEqual(used.Inputs, expected) {
			t.Fatal("use changed another field or instance", used.Inputs)
		}
		input = used.Inputs
	}
	if !reflect.DeepEqual(result.Inputs, before) {
		t.Fatal("play mutated the previous evaluation")
	}
	for _, change := range []Object{{"operation": "rest", "rest": "short"}, {"operation": "rest", "rest": "long"}, {"operation": "damage", "amount": float64(1)}} {
		next, err := ApplyCharacterPlay(input, change, records, profile)
		if err != nil || !reflect.DeepEqual(next.Inputs.Play.Inventory, input.Play.Inventory) || !reflect.DeepEqual(next.Inputs.Play.QuickUse, input.Play.QuickUse) {
			t.Fatal("play recreated spent items or pins", err)
		}
	}
	if truth(object(object(EvaluateCharacter(input, records, profile).Guidance["quickUse"])["one"])["canUse"]) {
		t.Fatal("depleted item remains usable")
	}
	result.Inputs.Play.QuickUse[0] = "changed"
	if !reflect.DeepEqual(caller, before) {
		t.Fatal("quick-use result aliases caller state")
	}
}

func TestQuickUseRejectsInvalidReferencesAndCommandsWithoutMutation(t *testing.T) {
	input, records, profile := characterFixture(t)
	input.Play.Inventory = quickUseItems()
	for _, pins := range [][]string{{""}, {"missing"}, {"one", "one"}} {
		candidate := cloneCharacter(input)
		candidate.Play.QuickUse = pins
		result := EvaluateCharacter(candidate, records, profile)
		if result.Ready || truth(result.Guidance["canSave"]) || !reflect.DeepEqual(result.Inputs.Play.QuickUse, pins) {
			t.Fatal("invalid pins were normalized or accepted", pins, result.Issues)
		}
	}
	input.Play.QuickUse = []string{"one"}
	before, _ := json.Marshal(input)
	for _, change := range []Object{
		{"operation": "consume-item"}, {"operation": "consume-item", "itemId": "missing"},
		{"operation": "consume-item", "itemId": "duplicate-copy"}, {"operation": "consume-item", "itemId": "empty"},
		{"operation": "consume-item", "itemId": "one", "amount": float64(2)}, {"operation": "consume-item", "itemId": float64(1)},
	} {
		result, err := ApplyCharacterPlay(input, change, records, profile)
		after, _ := json.Marshal(input)
		if err == nil || string(after) != string(before) || !reflect.DeepEqual(result.Inputs, input) {
			t.Fatal("invalid command mutated input", change, err)
		}
	}
}

func TestQuickUseSupportsIncompleteAndOlderCharacters(t *testing.T) {
	_, records, profile := characterFixture(t)
	input := character.Blank()
	result := EvaluateCharacter(input, records, profile)
	body, _ := json.Marshal(result.Inputs.Play)
	var play map[string]any
	json.Unmarshal(body, &play)
	if _, added := play["quickUse"]; added {
		t.Fatal("evaluation rewrote old characters with a default")
	}
	input.Play.Inventory = quickUseItems()
	input.Play.QuickUse = []string{"one", "empty"}
	result = EvaluateCharacter(input, records, profile)
	if result.Ready || !truth(result.Guidance["canSave"]) || len(objects(result.Sheet["quickUse"])) != 2 || truth(object(object(result.Guidance["quickUse"])["one"])["canUse"]) {
		t.Fatal("incomplete character cannot retain pins or exposes play actions", result.Issues)
	}
}

func TestUsingLastEquippedItemReleasesOnlyItsAllocation(t *testing.T) {
	input, records, profile := characterFixture(t)
	input.Play.Inventory = []character.Item{{ID: "last", Name: "Token", Quantity: 1, Location: "equipped", Attuned: true, GrantID: "token", Notes: "Retain me"}}
	input.Play.QuickUse = []string{"last"}
	input.Grants = []character.Grant{{ID: "token", Name: "Token", ActorID: "dm", Reason: "Reward", Active: true, EffectiveLevel: 1, Condition: "equipped", ItemID: "last", Effects: []character.Effect{{Target: "speed", Mode: "add", Value: 5}}}}
	before := EvaluateCharacter(input, records, profile)
	result, err := ApplyCharacterPlay(input, Object{"operation": "consume-item", "itemId": "last"}, records, profile)
	if err != nil || !result.Ready {
		t.Fatal(err, result.Issues)
	}
	expected := cloneCharacter(input)
	expected.Play.Inventory[0].Quantity, expected.Play.Inventory[0].Location, expected.Play.Inventory[0].Attuned = 0, "carried", false
	if !reflect.DeepEqual(result.Inputs, expected) || integer(object(before.Sheet["derived"])["speed"], 0)-integer(object(result.Sheet["derived"])["speed"], 0) != 5 {
		t.Fatal("last-use cleanup lost identity or retained equipped effects")
	}
}
