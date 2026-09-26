package rules

import (
	"encoding/json"
	"github.com/pjunak/addon-dnd-engine/character"
	"reflect"
	"strings"
	"testing"
)

func placementFixture(t *testing.T) (character.Inputs, memoryRecords, Ruleset) {
	input, source, profile := characterFixture(t)
	records := source.(memoryRecords)
	records.byKind["gear"] = map[string]json.RawMessage{
		"lenses":    mustJSON(Object{"id": "lenses", "name": "Lenses", "bodyPlacements": []string{"face"}}),
		"greaves":   mustJSON(Object{"id": "greaves", "name": "Greaves", "bodyPlacements": []string{"legs"}}),
		"accessory": mustJSON(Object{"id": "accessory", "name": "Accessory", "bodyPlacements": []string{"other", "neck"}}),
	}
	input.Play.Inventory = []character.Item{
		{ID: "one", Name: "Personal lenses", Reference: &character.Reference{Kind: "gear", ID: "lenses"}, Quantity: 1, Location: "equipped", BodyPlacement: "face", Notes: "Keep notes"},
		{ID: "two", Name: "Greaves", Reference: &character.Reference{Kind: "gear", ID: "greaves"}, Quantity: 1, Location: "equipped", BodyPlacement: "legs"},
		{ID: "three", Name: "Accessory", Reference: &character.Reference{Kind: "gear", ID: "accessory"}, Quantity: 1, Location: "equipped", BodyPlacement: "other"},
		{ID: "copy", Name: "Accessory", Reference: &character.Reference{Kind: "gear", ID: "accessory"}, Quantity: 1, Location: "equipped", BodyPlacement: "other"},
	}
	return input, records, profile
}

func TestBodyPlacementKeepsIdentityAndDoesNotCreateMechanicalSlots(t *testing.T) {
	input, records, profile := placementFixture(t)
	unplaced := cloneCharacter(input)
	for i := range unplaced.Play.Inventory {
		unplaced.Play.Inventory[i].BodyPlacement = ""
	}
	baseline := EvaluateCharacter(unplaced, records, profile)
	result := EvaluateCharacter(input, records, profile)
	if !result.Ready || !truth(result.Guidance["canSave"]) || !reflect.DeepEqual(result.Inputs, input) {
		t.Fatal(result.Issues)
	}
	for _, key := range []string{"derived", "attunement", "weapons", "abilities"} {
		if !reflect.DeepEqual(result.Sheet[key], baseline.Sheet[key]) {
			t.Fatal("placement changed mechanics", key)
		}
	}
	if len(objects(result.Sheet["bodyPlacement"])) != 4 || len(result.Explanations["bodyPlacement"].Terms) != 4 {
		t.Fatal("saved identity/explanation lost")
	}
	if got := stringsOf(object(object(result.Guidance["equipment"])["one"])["bodyPlacements"]); !reflect.DeepEqual(got, []string{"face"}) {
		t.Fatal(got)
	}
	if !reflect.DeepEqual(object(object(result.Sheet["equipment"])["two"])["bodyPlacements"], []string{"legs"}) {
		t.Fatal("saved source facts lost")
	}
	for _, change := range []Object{{"operation": "rest", "rest": "long"}, {"operation": "rest", "rest": "short"}, {"operation": "consume-item", "itemId": "one"}} {
		next, err := ApplyCharacterPlay(input, change, records, profile)
		if err != nil {
			t.Fatal(err)
		}
		for i, item := range input.Play.Inventory {
			expected := item
			if change["operation"] == "consume-item" && item.ID == "one" {
				expected.Quantity = 0
				expected.Location = "carried"
				expected.BodyPlacement = ""
			}
			if !reflect.DeepEqual(next.Inputs.Play.Inventory[i], expected) {
				t.Fatal("play rewrote instance", next.Inputs.Play.Inventory[i], expected)
			}
		}
	}
	if !reflect.DeepEqual(result.Inputs, input) {
		t.Fatal("later play mutated saved evaluation")
	}
}

func TestBodyPlacementRejectsUnsupportedAssignmentsWithoutRepairingInputs(t *testing.T) {
	original, records, profile := placementFixture(t)
	cases := map[string]func(*character.Inputs){
		"unknown":        func(v *character.Inputs) { v.Play.Inventory[0].BodyPlacement = "tentacle" },
		"wrong source":   func(v *character.Inputs) { v.Play.Inventory[0].BodyPlacement = "legs" },
		"stored":         func(v *character.Inputs) { v.Play.Inventory[0].Location = "stored" },
		"carried":        func(v *character.Inputs) { v.Play.Inventory[0].Location = "carried" },
		"empty":          func(v *character.Inputs) { v.Play.Inventory[0].Quantity = 0 },
		"unruled custom": func(v *character.Inputs) { v.Play.Inventory[0].Reference = nil },
		"missing source": func(v *character.Inputs) { v.Play.Inventory[0].Reference.ID = "missing" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			input := cloneCharacter(original)
			mutate(&input)
			result := EvaluateCharacter(input, records, profile)
			if result.Ready || truth(result.Guidance["canSave"]) || !reflect.DeepEqual(result.Inputs, input) {
				t.Fatal("invalid assignment accepted or rewritten", result.Issues)
			}
		})
	}
	for _, declaration := range []any{[]string{"face", "face"}, []string{"face", "unknown"}, []any{"face", 42}, nil} {
		record := recordByID(records, "gear", "lenses")
		record["bodyPlacements"] = declaration
		records.byKind["gear"]["lenses"] = mustJSON(record)
		result := EvaluateCharacter(original, records, profile)
		if truth(result.Guidance["canSave"]) || len(stringsOf(object(object(result.Guidance["equipment"])["one"])["bodyPlacements"])) != 0 {
			t.Fatal("malformed source accepted")
		}
	}
}

func TestBodyPlacementPreservesOldAndIncompleteCharactersAndDMEligibility(t *testing.T) {
	input, records, profile := placementFixture(t)
	input.Build = character.Blank().Build
	result := EvaluateCharacter(input, records, profile)
	if result.Ready || !truth(result.Guidance["canSave"]) {
		t.Fatal("legal incomplete build rejected", result.Issues)
	}
	input, records, profile = placementFixture(t)
	input.Play.Inventory[0].Reference = nil
	input.Play.Inventory[0].GrantID = "custom"
	input.Grants = []character.Grant{{ID: "custom", Name: "Custom lenses", Reason: "Ruling", ActorID: "dm", GrantedAt: input.Play.AsOf, Active: true, EffectiveLevel: 1, ItemID: "one", Condition: "equipped", Effects: []character.Effect{{Target: "speed", Mode: "add", Value: 1}}}}
	if !truth(EvaluateCharacter(input, records, profile).Guidance["canSave"]) {
		t.Fatal("active DM item cannot be placed")
	}
	input.Grants[0].Active = false
	if truth(EvaluateCharacter(input, records, profile).Guidance["canSave"]) {
		t.Fatal("withdrawn grant still grants placement")
	}
	input = character.Blank()
	result = EvaluateCharacter(input, records, profile)
	body, _ := json.Marshal(result.Inputs)
	if strings.Contains(string(body), "bodyPlacement") {
		t.Fatal("default body placement added")
	}
}

func TestBodyPlacementDoesNotBypassArmorExclusivity(t *testing.T) {
	input, records, profile := placementFixture(t)
	records.byKind["gear"]["suit"] = mustJSON(Object{"id": "suit", "name": "Armor", "armorType": "heavy", "baseAC": 18, "dexCap": 0, "bodyPlacements": []string{"body", "other"}})
	input.Play.Inventory = []character.Item{
		{ID: "first", Reference: &character.Reference{Kind: "gear", ID: "suit"}, Quantity: 1, Location: "equipped", BodyPlacement: "body"},
		{ID: "second", Reference: &character.Reference{Kind: "gear", ID: "suit"}, Quantity: 1, Location: "equipped", BodyPlacement: "other"},
	}
	result := EvaluateCharacter(input, records, profile)
	for _, issue := range result.Issues {
		if issue.ID == "equipped:armor" {
			return
		}
	}
	t.Fatal("different display groups bypassed armor occupancy")
}
