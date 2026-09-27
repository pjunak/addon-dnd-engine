package rules

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/pjunak/addon-dnd-engine/character"
)

func handsFixture(t *testing.T) (character.Inputs, memoryRecords, Ruleset) {
	t.Helper()
	input, source, profile := characterFixture(t)
	records := source.(memoryRecords)
	for kind, entries := range newMemoryRecords([]Object{
		{"kind": "weapon", "id": "blade", "name": "Blade", "category": "simple", "damage": "1d8", "damageType": "slashing", "properties": []any{"versatile"}, "versatileDamage": "1d10"},
		{"kind": "weapon", "id": "heavy", "name": "Heavy", "category": "martial", "damage": "2d6", "damageType": "slashing", "properties": []any{"two-handed"}},
		{"kind": "weapon", "id": "light", "name": "Light", "category": "simple", "damage": "1d4", "damageType": "piercing"},
		{"kind": "magic-item", "id": "guard", "name": "Guard", "armorType": "shield", "acBonus": 2, "attunement": true, "characterMechanics": "complete", "characterEffects": []any{Object{"target": "speed", "mode": "add", "value": 5}}},
	}).byKind {
		records.byKind[kind] = entries
	}
	input.Play.Inventory = []character.Item{
		{ID: "main-copy", Reference: &character.Reference{Kind: "weapon", ID: "blade"}, Name: "Authored blade", Quantity: 1, Location: "equipped", Notes: "Keep the etching"},
		{ID: "off-copy", Reference: &character.Reference{Kind: "magic-item", ID: "guard"}, Name: "Authored shield", Quantity: 1, Location: "equipped", Attuned: true, Acquisition: "Reward", Notes: "Keep the crest"},
		{ID: "spare-copy", Reference: &character.Reference{Kind: "weapon", ID: "blade"}, Name: "Spare blade", Quantity: 1, Location: "stored"},
	}
	input.Play.Hands = &character.Hands{Main: "main-copy", Off: "off-copy", Grip: "one"}
	return input, records, profile
}

func TestHandsSuspendExactOffHandAndRestoreWithoutChangingAttunement(t *testing.T) {
	input, records, profile := handsFixture(t)
	before := cloneCharacter(input)
	start := EvaluateCharacter(input, records, profile)
	if !start.Ready {
		t.Fatal(start.Issues)
	}
	two, err := ApplyCharacterPlay(input, Object{"operation": "set-grip", "grip": "two"}, records, profile)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(input, before) || two.Inputs.Play.Hands.Off != "" || two.Inputs.Play.Hands.SuspendedOff.ItemID != "off-copy" || !two.Inputs.Play.Inventory[1].Attuned || two.Inputs.Play.Inventory[1].Location != "carried" {
		t.Fatal("suspension changed ownership or caller", two.Inputs)
	}
	if integer(object(two.Sheet["derived"])["armorClass"], 0) != integer(object(start.Sheet["derived"])["armorClass"], 0)-2 || integer(object(two.Sheet["derived"])["speed"], 0) != integer(object(start.Sheet["derived"])["speed"], 0)-5 {
		t.Fatal("suspended effects remain", start.Sheet["derived"], two.Sheet["derived"])
	}
	weapon := objects(two.Sheet["weapons"])[0]
	if text(weapon["itemId"]) != "main-copy" || text(weapon["damage"]) != "1d10 +2" || two.Explanations["weapons.0.damage"].Value != weapon["damage"] {
		t.Fatal("grip and explanation differ", weapon)
	}
	if !reflect.DeepEqual(two.Inputs, EvaluateCharacter(two.Inputs, records, profile).Inputs) {
		t.Fatal("recalculation changed hands")
	}
	one, err := ApplyCharacterPlay(two.Inputs, Object{"operation": "set-grip", "grip": "one"}, records, profile)
	if err != nil || !reflect.DeepEqual(one.Inputs, input) {
		t.Fatal("did not restore exact off-hand instance", one.Inputs, err)
	}
	if !reflect.DeepEqual(one.Sheet["derived"], start.Sheet["derived"]) {
		t.Fatal("restored effects differ")
	}
}

func TestHandsNeverRestoreChangedMissingConsumedOrIneligibleInstances(t *testing.T) {
	for _, scenario := range []string{"removed", "consumed", "moved", "edited", "source", "occupied"} {
		t.Run(scenario, func(t *testing.T) {
			input, records, profile := handsFixture(t)
			two, err := ApplyCharacterPlay(input, Object{"operation": "set-grip", "grip": "two"}, records, profile)
			if err != nil {
				t.Fatal(err)
			}
			input = two.Inputs
			switch scenario {
			case "removed":
				input.Play.Inventory = append(input.Play.Inventory[:1], input.Play.Inventory[2:]...)
			case "consumed":
				input.Play.Inventory[1].Quantity = 0
				input.Play.Inventory[1].Attuned = false
			case "moved":
				input.Play.Inventory[1].Location = "stored"
			case "edited":
				input.Play.Inventory[1].Notes = "Changed elsewhere"
			case "source":
				record := recordByID(records, "magic-item", "guard")
				record["armorType"], record["characterEffects"] = "", []any{}
				records.byKind["magic-item"]["guard"], _ = json.Marshal(record)
			case "occupied":
				duplicate := input.Play.Inventory[1]
				duplicate.ID = "other-guard"
				duplicate.Location = "equipped"
				duplicate.Attuned = false
				input.Play.Inventory = append(input.Play.Inventory, duplicate)
			}
			before := cloneCharacter(input)
			one, err := ApplyCharacterPlay(input, Object{"operation": "set-grip", "grip": "one"}, records, profile)
			if err != nil {
				t.Fatal(err)
			}
			if one.Inputs.Play.Hands.Off != "" || one.Inputs.Play.Hands.SuspendedOff != nil || text(object(one.Sheet["hands"])["restoreReason"]) == "" || !reflect.DeepEqual(one.Inputs.Play.Inventory, before.Play.Inventory) {
				t.Fatal("changed instance was restored or lost", one.Inputs, one.Sheet["hands"])
			}
		})
	}
}

func TestHandSelectionAndCapacityUseSourceFactsAndOwnedInstances(t *testing.T) {
	input, records, profile := handsFixture(t)
	input.Play.Hands = nil
	legacy := cloneCharacter(input)
	if result := EvaluateCharacter(input, records, profile); !reflect.DeepEqual(result.Inputs, legacy) {
		t.Fatal("legacy state rewritten")
	}
	input.Play.Hands = &character.Hands{Main: "main-copy", Off: "off-copy", Grip: "one"}
	for _, command := range []Object{
		{"operation": "set-hand", "hand": "main", "itemId": "off-copy"},
		{"operation": "set-hand", "hand": "off", "itemId": "missing"},
		{"operation": "set-grip", "grip": "many"},
		{"operation": "set-grip", "grip": "two", "extra": true},
	} {
		if _, err := ApplyCharacterPlay(input, command, records, profile); err == nil {
			t.Fatal("invalid command accepted", command)
		}
	}
	input.Play.Inventory[0].Reference.ID = "heavy"
	result := EvaluateCharacter(input, records, profile)
	if result.Ready || truth(result.Guidance["canSave"]) {
		t.Fatal("required two-handed weapon accepted with shield", result.Issues)
	}
	result, err := ApplyCharacterPlay(input, Object{"operation": "set-hand", "hand": "main", "itemId": "main-copy"}, records, profile)
	if err != nil || result.Inputs.Play.Hands.Grip != "two" || result.Inputs.Play.Hands.SuspendedOff == nil {
		t.Fatal("required two-handed selection", err, result.Issues)
	}
	if _, err = ApplyCharacterPlay(result.Inputs, Object{"operation": "set-grip", "grip": "one"}, records, profile); err == nil {
		t.Fatal("one hand permitted for a two-handed source")
	}
	result, err = ApplyCharacterPlay(result.Inputs, Object{"operation": "set-hand", "hand": "main", "itemId": "spare-copy"}, records, profile)
	if err != nil || result.Inputs.Play.Hands.Main != "spare-copy" || result.Inputs.Play.Hands.Off != "off-copy" || result.Inputs.Play.Inventory[0].Location != "carried" {
		t.Fatal("copy replacement lost instance identity", err, result.Inputs)
	}
}

func TestSuspendedHandEffectsRemainInactiveWhenMovedOutsideHandCommands(t *testing.T) {
	input, records, profile := handsFixture(t)
	two, err := ApplyCharacterPlay(input, Object{"operation": "set-grip", "grip": "two"}, records, profile)
	if err != nil {
		t.Fatal(err)
	}
	two.Inputs.Play.Inventory[1].Location = "equipped"
	moved := EvaluateCharacter(two.Inputs, records, profile)
	if !reflect.DeepEqual(moved.Sheet["derived"], two.Sheet["derived"]) || text(object(moved.Sheet["hands"])["restoreReason"]) != "changed" {
		t.Fatal("suspended effects returned before the hand was resolved", moved.Sheet["derived"])
	}
}

func TestSavedHandsRejectInvalidIdentityAndGrip(t *testing.T) {
	for _, hands := range []*character.Hands{
		{Grip: "many"}, {Main: "main-copy", Off: "main-copy", Grip: "one"},
		{Main: "main-copy", Off: "off-copy", Grip: "two"},
		{Main: "main-copy", Grip: "two", SuspendedOff: &character.SuspendedHand{ItemID: "off-copy", ExpectedItemSHA256: "invalid"}},
	} {
		input, records, profile := handsFixture(t)
		input.Play.Hands = hands
		result := EvaluateCharacter(input, records, profile)
		if result.Ready || truth(result.Guidance["canSave"]) {
			t.Fatal("invalid saved hand state accepted", hands)
		}
	}
}
