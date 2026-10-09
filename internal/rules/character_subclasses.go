package rules

import "github.com/pjunak/addon-dnd-engine/character"

// classSubclassLevel is the class level at which a class's subclass is chosen,
// from the class record. Records without the field use the common third level.
func classSubclassLevel(class Object) int {
	return max(1, integer(class["subclassLevel"], 3))
}

// validateCharacterSubclasses rejects a subclass recorded for a class the
// character lacks, before that class's subclass level, or from another class.
// Removing a class level can leave such a subclass behind; the issue lets the
// consumer withdraw it instead of guessing from level arithmetic.
func validateCharacterSubclasses(input character.Inputs, records Records, result *character.Result) {
	levels := map[string]int{}
	for _, level := range input.Build.Levels {
		levels[level.ClassID]++
	}
	for _, classID := range sortedKeys(input.Build.Subclasses) {
		subclassID := input.Build.Subclasses[classID]
		if subclassID == "" {
			continue
		}
		class := recordView(records, "class", classID)
		subclass := recordView(records, "subclass", subclassID)
		if class == nil || subclass == nil || text(subclass["classId"]) != classID || levels[classID] < classSubclassLevel(class) {
			addCharacterIssue(result, "subclass-level:"+classID, "levels", "This subclass is not available at the recorded class level.", "blocker", &character.Reference{Kind: "subclass", ID: subclassID})
		}
	}
}

// annotateSpellbookCapacity publishes how many spells each spellbook may hold:
// the spells its class levels grant plus copied spells recorded as paid
// acquisitions. Consumers and validation use this one figure.
func annotateSpellbookCapacity(input character.Inputs, sheet Object) {
	copied := map[string]map[string]bool{}
	for _, acquisition := range input.Build.Spells.Acquisitions {
		if copied[acquisition.ClassID] == nil {
			copied[acquisition.ClassID] = map[string]bool{}
		}
		copied[acquisition.ClassID][acquisition.SpellID] = true
	}
	for _, caster := range objects(object(sheet["spellcasting"])["perClass"]) {
		classID := text(caster["classId"])
		if text(caster["prepares"]) != "spellbook" {
			continue
		}
		capacity := integer(caster["spellbookKnown"], 0)
		for _, id := range input.Build.Spells.Spellbook[classID] {
			if copied[classID][id] {
				capacity++
			}
		}
		caster["spellbookCapacity"] = capacity
	}
}
