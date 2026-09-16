package rules

import (
	"reflect"
	"testing"

	"github.com/pjunak/addon-dnd-engine/character"
)

func TestEquipmentSaveRejectsEmptyEquippedInstances(t *testing.T) {
	input, source, profile := characterFixture(t)
	records := source.(memoryRecords)
	records.byKind["weapon"] = newMemoryRecords([]Object{{"kind": "weapon", "id": "blade", "name": "Blade"}}).byKind["weapon"]
	input.Play.Inventory = []character.Item{{ID: "empty", Reference: &character.Reference{Kind: "weapon", ID: "blade"}, Name: "Blade", Location: "equipped", Quantity: 0}}
	original := cloneCharacter(input)
	result := EvaluateCharacter(input, records, profile)
	if result.Ready || truth(result.Guidance["canSave"]) {
		t.Fatal("empty equipped instance was accepted", result.Issues)
	}
	if !reflect.DeepEqual(input, original) || !reflect.DeepEqual(result.Inputs, original) {
		t.Fatal("invalid equipment was silently normalized")
	}
	input.Play.Inventory[0].Location = "carried"
	if result := EvaluateCharacter(input, records, profile); !truth(result.Guidance["canSave"]) {
		t.Fatal("depleted carried inventory must remain saveable", result.Issues)
	}
}

func TestCustomEquipmentUsesCurrentGrantAuthorityAndProspectiveCondition(t *testing.T) {
	for _, condition := range []string{"always", "equipped"} {
		t.Run(condition, func(t *testing.T) {
			input, records, profile := characterFixture(t)
			input.Play.Inventory = []character.Item{{ID: "custom", Name: "Custom charm", GrantID: "mechanics", Location: "carried", Quantity: 1}}
			input.Grants = []character.Grant{{ID: "mechanics", ItemID: "custom", Name: "Charm", ActorID: "dm", Reason: "Authored mechanics", GrantedAt: input.Play.AsOf, Active: true, EffectiveLevel: 1, Condition: condition, Effects: []character.Effect{{Target: "initiative", Mode: "add", Value: 1}}}}
			original := cloneCharacter(input)
			result := EvaluateCharacter(input, records, profile)
			if !truth(object(object(result.Guidance["equipment"])["custom"])["canEquip"]) {
				t.Fatal("eligible item cannot be equipped", result.Guidance["equipment"])
			}
			if !reflect.DeepEqual(input, original) {
				t.Fatal("prospective eligibility mutated input")
			}
			input.Play.Inventory[0].Location = "equipped"
			if result := EvaluateCharacter(input, records, profile); !result.Ready {
				t.Fatal("valid equipment rejected", result.Issues)
			}
			for _, kind := range []string{"expired", "future", "revoked"} {
				t.Run(kind, func(t *testing.T) {
					candidate := cloneCharacter(input)
					switch kind {
					case "expired":
						candidate.Grants[0].ExpiresAt = candidate.Play.AsOf
					case "future":
						candidate.Grants[0].EffectiveLevel = 2
					case "revoked":
						candidate.Grants[0].Active = false
					}
					result := EvaluateCharacter(candidate, records, profile)
					if result.Ready || truth(result.Guidance["canSave"]) || truth(object(object(result.Guidance["equipment"])["custom"])["canEquip"]) {
						t.Fatal("inactive grant authorized equipment", result.Issues, result.Guidance["equipment"])
					}
				})
			}
		})
	}
}
