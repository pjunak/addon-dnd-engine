package rules

import (
	"reflect"
	"strings"
	"testing"

	"github.com/pjunak/addon-dnd-engine/character"
)

func TestEquipmentSlotsUseDeclaredArmorTypeAcrossCatalogKinds(t *testing.T) {
	input, source, profile := characterFixture(t)
	records := source.(memoryRecords)
	extra := newMemoryRecords([]Object{
		{"kind": "armor", "id": "travel-plate", "name": "Travel plate", "armorType": "heavy", "baseAC": 18},
		{"kind": "armor", "id": "round-guard", "name": "Round guard", "armorType": "shield", "acBonus": 2},
		{"kind": "magic-item", "id": "ward", "name": "Ward", "armorType": "shield", "characterMechanics": "narrative"},
	})
	for kind, rows := range extra.byKind {
		records.byKind[kind] = rows
	}
	input.Play.Inventory = []character.Item{
		{ID: "plate", Reference: &character.Reference{Kind: "armor", ID: "travel-plate"}, Name: "Plate", Location: "stored", Quantity: 1},
		{ID: "guard", Reference: &character.Reference{Kind: "armor", ID: "round-guard"}, Name: "Guard", Location: "equipped", Quantity: 1},
		{ID: "ward", Reference: &character.Reference{Kind: "magic-item", ID: "ward"}, Name: "Ward", Location: "carried", Quantity: 1},
	}
	result := EvaluateCharacter(input, records, profile)
	for id, slot := range map[string]string{"plate": "armor", "guard": "shield", "ward": "shield"} {
		if text(object(object(result.Guidance["equipment"])[id])["slot"]) != slot || text(object(object(result.Sheet["equipment"])[id])["slot"]) != slot {
			t.Fatal("guidance and saved projection must share declared slots", id, result.Guidance["equipment"], result.Sheet["equipment"])
		}
	}
	input.Play.Inventory[2].Location = "equipped"
	original := cloneCharacter(input)
	result = EvaluateCharacter(input, records, profile)
	if !hasChoiceIssue(result, "equipped:shield") || truth(result.Guidance["canSave"]) {
		t.Fatal("duplicate shield slot was accepted", result.Issues)
	}
	if !reflect.DeepEqual(input, original) || !reflect.DeepEqual(result.Inputs, original) {
		t.Fatal("slot validation changed authored inventory")
	}
}

func TestAttunementPrerequisitesCannotBeSuppliedByTheItemBeingAttuned(t *testing.T) {
	input, source, profile := characterFixture(t)
	records := source.(memoryRecords)
	record := Object{"kind": "magic-item", "id": "test-focus", "name": "Test focus", "attunement": true, "characterMechanics": "complete",
		"attunementPrerequisites": Object{"abilities": Object{"CON": 19}},
		"characterEffects":        []any{Object{"target": "abilityScore", "key": "CON", "mode": "minimum", "value": 19}}}
	records.byKind["magic-item"] = newMemoryRecords([]Object{record}).byKind["magic-item"]
	input.Play.Inventory = []character.Item{{ID: "focus", Reference: &character.Reference{Kind: "magic-item", ID: "test-focus"}, Name: "Focus", Quantity: 1, Location: "equipped", Attuned: true}}
	original := cloneCharacter(input)
	result := EvaluateCharacter(input, records, profile)
	if !hasChoiceIssue(result, "attunement:focus") || truth(result.Guidance["canSave"]) || truth(object(object(result.Guidance["equipment"])["focus"])["canAttune"]) {
		t.Fatal("item qualified for its own attunement", result.Issues, result.Guidance["equipment"])
	}
	if !reflect.DeepEqual(input, original) || !reflect.DeepEqual(result.Inputs, original) {
		t.Fatal("prerequisite check changed input")
	}
	grant := character.Grant{ID: "independent", Name: "Independent training", ActorID: "dm", Reason: "Test training", GrantedAt: input.Play.AsOf, Active: true, EffectiveLevel: 1, Condition: "always",
		Effects: []character.Effect{{Target: "abilityScore", Key: "CON", Mode: "minimum", Value: 19}}}
	input.Grants = []character.Grant{grant}
	result = EvaluateCharacter(input, records, profile)
	if !result.Ready || !truth(object(object(result.Guidance["equipment"])["focus"])["canAttune"]) {
		t.Fatal("independently qualified item was rejected", result.Issues)
	}
	for _, state := range []string{"revoked", "expired", "dependent"} {
		candidate := cloneCharacter(input)
		switch state {
		case "revoked":
			candidate.Grants[0].Active = false
		case "expired":
			candidate.Grants[0].ExpiresAt = input.Play.AsOf
		case "dependent":
			candidate.Grants[0].Condition = "attuned"
			candidate.Grants[0].ItemID = "focus"
		}
		result = EvaluateCharacter(candidate, records, profile)
		if !hasChoiceIssue(result, "attunement:focus") {
			t.Fatal("lost or circular prerequisite remained authorized", state, result.Issues)
		}
	}
	record["attunementPrerequisites"] = Object{"text": "Requires a recorded ruling"}
	records.byKind["magic-item"]["test-focus"] = mustJSON(record)
	input.Grants = nil
	result = EvaluateCharacter(input, records, profile)
	if !hasChoiceIssue(result, "attunement:focus") {
		t.Fatal("narrative prerequisite was guessed", result.Issues)
	}
	grant.Effects = nil
	grant.Waivers = []string{"attunement:focus"}
	input.Grants = []character.Grant{grant}
	if result = EvaluateCharacter(input, records, profile); !result.Ready {
		t.Fatal("exact independent DM waiver rejected", result.Issues)
	}
	input.Grants[0].Condition = "attuned"
	input.Grants[0].ItemID = "focus"
	if result = EvaluateCharacter(input, records, profile); !hasChoiceIssue(result, "attunement:focus") {
		t.Fatal("attunement supplied its own waiver", result.Issues)
	}
}

func TestAttunementCapacityAndDuplicateGuidancePreserveAuthoredState(t *testing.T) {
	input, source, profile := characterFixture(t)
	records := source.(memoryRecords)
	entries := []Object{}
	for _, id := range []string{"one", "two", "three", "four"} {
		entries = append(entries, Object{"kind": "magic-item", "id": id, "name": id, "attunement": true, "characterMechanics": "complete", "characterEffects": []any{}})
		input.Play.Inventory = append(input.Play.Inventory, character.Item{ID: id, Name: id, Reference: &character.Reference{Kind: "magic-item", ID: id}, Location: "carried", Quantity: 1, Attuned: id != "four"})
	}
	records.byKind["magic-item"] = newMemoryRecords(entries).byKind["magic-item"]
	profile.Constants.Character.UniqueAttunement = true
	result := EvaluateCharacter(input, records, profile)
	if text(object(object(result.Guidance["equipment"])["four"])["attuneReason"]) != "capacity" {
		t.Fatal("capacity rejection needs a reason", result.Guidance["equipment"])
	}
	input.Play.Inventory[3].Attuned = true
	original := cloneCharacter(input)
	result = EvaluateCharacter(input, records, profile)
	if !hasChoiceIssue(result, "attunement-capacity") || !reflect.DeepEqual(result.Inputs, original) {
		t.Fatal("excess attunement must be rejected without withdrawing items", result.Issues)
	}
	input.Grants = []character.Grant{{ID: "capacity", Name: "DM capacity", ActorID: "dm", Reason: "Test ruling", GrantedAt: input.Play.AsOf, Active: true, EffectiveLevel: 1, Condition: "always", Effects: []character.Effect{{Target: "attunementLimit", Mode: "add", Value: 1}}}}
	if result = EvaluateCharacter(input, records, profile); !result.Ready {
		t.Fatal("active capacity grant rejected", result.Issues)
	}
	for _, issue := range result.Issues {
		if strings.Contains(issue.Message, "Attuned to more than") {
			t.Fatal("obsolete warning ignores authorized capacity", result.Issues)
		}
	}
	input.Grants[0].Active = false
	if result = EvaluateCharacter(input, records, profile); !hasChoiceIssue(result, "attunement-capacity") {
		t.Fatal("withdrawn capacity grant still authorized", result.Issues)
	}
	input.Play.Inventory[3].Attuned = false
	input.Play.Inventory[3].Reference = input.Play.Inventory[0].Reference
	input.Play.Inventory[2].Attuned = false
	result = EvaluateCharacter(input, records, profile)
	if text(object(object(result.Guidance["equipment"])["four"])["attuneReason"]) != "duplicate" {
		t.Fatal("duplicate rejection needs a reason", result.Guidance["equipment"])
	}
	input.Play.Inventory[3].Attuned = true
	if result = EvaluateCharacter(input, records, profile); !hasChoiceIssue(result, "attunement-duplicate:four") {
		t.Fatal("duplicate attunement accepted", result.Issues)
	}
}

func TestCustomAttunementUsesProspectiveGrantAndCapacity(t *testing.T) {
	input, records, profile := characterFixture(t)
	input.Play.Inventory = []character.Item{{ID: "custom", Name: "Custom charm", Quantity: 1, Location: "carried", GrantID: "mechanics"}}
	input.Grants = []character.Grant{{ID: "mechanics", ItemID: "custom", Name: "Charm", ActorID: "dm", Reason: "Authored attuned mechanics", GrantedAt: input.Play.AsOf, Active: true, EffectiveLevel: 1, Condition: "attuned", Effects: []character.Effect{{Target: "initiative", Mode: "add", Value: 1}}}}
	original := cloneCharacter(input)
	result := EvaluateCharacter(input, records, profile)
	if !truth(object(object(result.Guidance["equipment"])["custom"])["canAttune"]) || !reflect.DeepEqual(input, original) {
		t.Fatal("prospective custom attunement unavailable or mutated input", result.Guidance["equipment"])
	}
	input.Play.Inventory[0].Attuned = true
	if result = EvaluateCharacter(input, records, profile); !result.Ready {
		t.Fatal("permitted custom attunement rejected", result.Issues)
	}
	input.Grants[0].Active = false
	result = EvaluateCharacter(input, records, profile)
	if !hasChoiceIssue(result, "custom-item:custom") || truth(object(object(result.Guidance["equipment"])["custom"])["canAttune"]) {
		t.Fatal("revoked custom attunement remained authorized", result.Issues)
	}
}
