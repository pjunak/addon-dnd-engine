package rules

import (
	"testing"

	"github.com/pjunak/addon-dnd-engine/character"
)

func characterIssue(result character.Result, id string) bool {
	for _, issue := range result.Issues {
		if issue.ID == id {
			return true
		}
	}
	return false
}

func TestSubclassesAndSpellListsLeftByARemovedLevelAreReported(t *testing.T) {
	input, _, profile := characterFixture(t)
	records := newMemoryRecords([]Object{
		{"kind": "class", "id": "fighter", "name": "Fighter", "hitDie": "d10", "subclassLevel": 3},
		{"kind": "class", "id": "rogue", "name": "Rogue", "hitDie": "d8"},
		{"kind": "subclass", "id": "champion", "name": "Champion", "classId": "fighter"},
		{"kind": "subclass", "id": "thief", "name": "Thief", "classId": "rogue"},
		{"kind": "species", "id": "dwarf", "name": "Dwarf", "speeds": Object{"walk": 30}},
		{"kind": "background", "id": "artisan", "name": "Artisan"},
	})
	input.Build.Levels = []character.Level{{ID: "one", ClassID: "fighter"}, {ID: "two", ClassID: "fighter"}, {ID: "three", ClassID: "fighter"}}
	input.Build.Subclasses = map[string]string{"fighter": "champion"}
	if result := EvaluateCharacter(input, records, profile); characterIssue(result, "subclass-level:fighter") {
		t.Fatal("rejected a subclass at its subclass level")
	}
	// Removing the third level leaves the subclass below its level.
	input.Build.Levels = input.Build.Levels[:2]
	if result := EvaluateCharacter(input, records, profile); !characterIssue(result, "subclass-level:fighter") {
		t.Fatal("accepted a subclass below its subclass level")
	}
	// A subclass of a class the character lacks, or of another class, is rejected.
	input.Build.Levels = append(input.Build.Levels, character.Level{ID: "three", ClassID: "fighter"})
	input.Build.Subclasses = map[string]string{"fighter": "thief", "rogue": "thief"}
	result := EvaluateCharacter(input, records, profile)
	if !characterIssue(result, "subclass-level:fighter") || !characterIssue(result, "subclass-level:rogue") {
		t.Fatal("accepted a mismatched or orphaned subclass", result.Issues)
	}
	// Spells of a class the character no longer has get their own issue, so a
	// consumer can withdraw them without confusing them with over-capacity.
	input.Build.Subclasses = map[string]string{}
	input.Build.Spells.Cantrips = map[string][]string{"wizard": {"spark"}}
	result = EvaluateCharacter(input, records, profile)
	if !characterIssue(result, "spell-class:cantrips:wizard") || characterIssue(result, "spell-count:cantrips:wizard") {
		t.Fatal("orphaned spell list not reported as such", result.Issues)
	}
}

func TestSpellbookCapacityCountsGrantedAndCopiedSpells(t *testing.T) {
	input := character.Blank()
	input.Build.Spells.Spellbook = map[string][]string{"wizard": {"ward", "copied", "other"}}
	input.Build.Spells.Acquisitions = []character.SpellAcquisition{{ID: "copy", ClassID: "wizard", SpellID: "copied", Level: 1}}
	sheet := Object{"spellcasting": Object{"perClass": []any{
		Object{"classId": "wizard", "prepares": "spellbook", "spellbookKnown": 6},
		Object{"classId": "cleric", "prepares": "list"},
	}}}
	annotateSpellbookCapacity(input, sheet)
	casters := objects(object(sheet["spellcasting"])["perClass"])
	if integer(casters[0]["spellbookCapacity"], 0) != 7 {
		t.Fatal("capacity ignores copied spells", casters[0])
	}
	if _, exists := casters[1]["spellbookCapacity"]; exists {
		t.Fatal("non-spellbook caster got a spellbook capacity")
	}
}

func TestMalformedHitDieIsReportedNotAssumed(t *testing.T) {
	input, records, profile := characterFixture(t)
	broken := newMemoryRecords([]Object{
		{"kind": "class", "id": "fighter", "name": "Fighter", "hitDie": "ten"},
		{"kind": "species", "id": "dwarf", "name": "Dwarf", "speeds": Object{"walk": 30}},
		{"kind": "background", "id": "artisan", "name": "Artisan"},
	})
	if result := EvaluateCharacter(input, records, profile); characterIssue(result, "class-hit-die:fighter") {
		t.Fatal("rejected a valid hit die")
	}
	if result := EvaluateCharacter(input, broken, profile); !characterIssue(result, "class-hit-die:fighter") {
		t.Fatal("assumed a hit die for a malformed record", result.Issues)
	}
}
