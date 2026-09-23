package rules

import (
	"reflect"
	"testing"

	"github.com/pjunak/addon-dnd-engine/character"
)

func TestSourceClassAndSpellcasterPrerequisites(t *testing.T) {
	for _, test := range []struct {
		name             string
		predicate, sheet Object
		match, known     bool
	}{
		{"class membership", Object{"classes": Object{"mage": 1}}, Object{"classes": []any{Object{"classId": "fighter", "level": 3}, Object{"classId": "mage", "level": 1}}}, true, true},
		{"class level", Object{"classes": Object{"mage": 2}}, Object{"classes": []any{Object{"classId": "mage", "level": 1}}}, false, true},
		{"missing class", Object{"classes": Object{"mage": 1}}, Object{}, false, true},
		{"invalid class level", Object{"classes": Object{"mage": 0}}, Object{}, false, false},
		{"class cantrip", Object{"spellcaster": true}, Object{"spellcasting": Object{"perClass": []any{Object{"cantripsKnown": 1}}}}, true, true},
		{"class spell", Object{"spellcaster": true}, Object{"spellcasting": Object{"perClass": []any{Object{"preparedLimit": 1, "maxSpellLevel": 1}}}}, true, true},
		{"not yet unlocked", Object{"spellcaster": true}, Object{"spellcasting": Object{"perClass": []any{Object{"preparedLimit": 0, "maxSpellLevel": 0}}}}, false, true},
		{"trait cantrip", Object{"spellcaster": true}, Object{"spellcasting": Object{"granted": []any{Object{"ref": "spark", "level": 0, "source": Object{"type": "species"}}}}}, true, true},
		{"feat free spell", Object{"spellcaster": true}, Object{"spellcasting": Object{"granted": []any{Object{"ref": "ward", "level": 1, "free": "1/long", "source": Object{"type": "feat"}}}}}, true, true},
		{"unusable prepared grant", Object{"spellcaster": true}, Object{"spellcasting": Object{"granted": []any{Object{"ref": "ward", "level": 1, "source": Object{"type": "feat"}}}}}, false, true},
		{"item does not qualify", Object{"spellcaster": true}, Object{"spellcasting": Object{"granted": []any{Object{"ref": "spark", "level": 0, "source": Object{"type": "magic-item"}}}}}, false, true},
		{"unknown predicate", Object{"spellcaster": "yes"}, Object{}, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			match, known := prerequisiteMatches(test.predicate, test.sheet)
			if match != test.match || known != test.known {
				t.Fatalf("got %v/%v want %v/%v", match, known, test.match, test.known)
			}
		})
	}
}

func TestAttunementClassWithdrawalRequiresExplicitRepair(t *testing.T) {
	input, source, profile := characterFixture(t)
	records := source.(memoryRecords)
	records.byKind["magic-item"] = newMemoryRecords([]Object{{"kind": "magic-item", "id": "training-focus", "attunement": true, "attunementPrerequisites": Object{"classes": Object{"fighter": 1}}, "characterMechanics": "complete", "characterEffects": []any{}}}).byKind["magic-item"]
	input.Play.Inventory = []character.Item{{ID: "focus", Name: "Focus", Quantity: 1, Location: "carried", Attuned: true, Reference: &character.Reference{Kind: "magic-item", ID: "training-focus"}}}
	if result := EvaluateCharacter(input, records, profile); !result.Ready {
		t.Fatal("qualified attunement rejected", result.Issues)
	}
	records.byKind["class"]["scout"] = mustJSON(Object{"kind": "class", "id": "scout", "hitDie": "d8"})
	input.Build.Levels[0].ClassID = "scout"
	before := cloneCharacter(input)
	result := EvaluateCharacter(input, records, profile)
	if !hasChoiceIssue(result, "attunement:focus") || truth(result.Guidance["canSave"]) {
		t.Fatal("lost prerequisite accepted", result.Issues)
	}
	if !reflect.DeepEqual(before, input) || !reflect.DeepEqual(before, result.Inputs) {
		t.Fatal("silently changed authored attunement")
	}
	input.Play.Inventory[0].Attuned = false
	if result = EvaluateCharacter(input, records, profile); !result.Ready {
		t.Fatal("explicit unattunement could not repair", result.Issues)
	}
}
