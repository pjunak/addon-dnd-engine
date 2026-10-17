package rules

import (
	"encoding/json"
	"testing"

	"github.com/pjunak/addon-dnd-engine/character"
)

// A level 3 singer (a full caster) whose features grant at-will, ritual-only
// and resource-cast spells, choices limited by the class's castable level and
// by the ritual tag, and the "priest" list for preparation from level 2.
func spellGrantFixture(t *testing.T) (character.Inputs, memoryRecords, Ruleset) {
	t.Helper()
	input, source, profile := characterFixture(t)
	records := source.(memoryRecords)
	progression := []any{}
	for level := 1; level <= 3; level++ {
		progression = append(progression, Object{"level": level, "cantripsKnown": 0, "preparedSpells": 1})
	}
	records.byKind["class"]["singer"] = mustJSON(Object{"kind": "class", "id": "singer", "name": "Singer", "hitDie": "d8",
		"spellcasting": Object{"ability": "CHA", "type": "full", "prepares": "list"}, "progression": progression})
	records.byKind["class"]["mage"] = mustJSON(Object{"kind": "class", "id": "mage", "name": "Mage", "hitDie": "d6",
		"spellcasting": Object{"ability": "INT", "type": "full", "prepares": "spellbook"}, "progression": progression})
	input.Build.Levels = []character.Level{{ID: "one", ClassID: "singer"}, {ID: "two", ClassID: "singer"}, {ID: "three", ClassID: "singer"}}
	records.byKind["feature"] = map[string]json.RawMessage{
		"singer-secrets": mustJSON(Object{"id": "singer-secrets", "kind": "feature", "name": "Secrets", "classId": "singer", "level": 2,
			"grants": Object{"spellLists": []any{"priest"}}}),
		"singer-gifts": mustJSON(Object{"id": "singer-gifts", "kind": "feature", "name": "Gifts", "classId": "singer", "level": 1,
			"grants": Object{
				"resources": []any{Object{"key": "steps", "name": "Steps", "fixed": 2, "recharge": "long"}},
				"spells": []any{
					Object{"ids": []any{"ward"}, "atWill": true, "alwaysPrepared": true},
					Object{"ids": []any{"listen"}, "ritualOnly": true},
					Object{"ids": []any{"step"}, "resource": "steps", "alwaysPrepared": true},
					Object{"id": "discoveries", "choose": 1, "maxSpellLevel": "class", "from": Object{"class": []any{"priest"}}, "classId": "singer", "alwaysPrepared": true},
					Object{"id": "rites", "choose": 1, "spellLevel": 1, "from": Object{"class": []any{"priest"}, "ritual": true}},
				},
			}}),
		"mage-savant": mustJSON(Object{"id": "mage-savant", "kind": "feature", "name": "Savant", "classId": "mage", "level": 1,
			"grants": Object{"spellbookBonus": []any{Object{"level": 1, "count": 2}, Object{"level": 3, "count": 3}}}}),
	}
	records.byKind["spell"] = map[string]json.RawMessage{}
	for _, spell := range []Object{
		{"id": "ward", "level": 1, "classes": []any{"singer"}},
		{"id": "listen", "level": 1, "ritual": true, "classes": []any{"singer"}},
		{"id": "step", "level": 2, "classes": []any{"singer"}},
		{"id": "blessing", "level": 1, "classes": []any{"priest"}},
		{"id": "rite", "level": 1, "ritual": true, "classes": []any{"priest"}},
		{"id": "miracle", "level": 3, "classes": []any{"priest"}},
	} {
		spell["kind"] = "spell"
		records.byKind["spell"][text(spell["id"])] = mustJSON(spell)
	}
	input.Build.Spells.GrantChoices = map[string][]string{"feature:singer-gifts:discoveries": {"blessing"}, "feature:singer-gifts:rites": {"rite"}}
	input.Play.PreparedSpells = map[string][]string{"singer": {"blessing"}}
	input.Play.HP = 1
	return input, records, profile
}

func grantedOption(result character.Result, ref string) Object {
	for _, grant := range objects(result.SpellOptions["granted"]) {
		if text(grant["ref"]) == ref {
			return grant
		}
	}
	return nil
}

func TestGrantedSpellsCastAtWillAsRitualsOrFromTheirResource(t *testing.T) {
	input, records, profile := spellGrantFixture(t)
	result := EvaluateCharacter(input, records, profile)
	if !result.Ready {
		t.Fatal(result.Issues)
	}
	ward, listen, step := grantedOption(result, "ward"), grantedOption(result, "listen"), grantedOption(result, "step")
	if slots := stringsOf(ward["slots"]); len(slots) < 2 || slots[0] != "" {
		t.Fatal("an at-will spell lost its free cast or its upcasting slots", ward)
	}
	if slots := stringsOf(listen["slots"]); len(slots) != 1 || slots[0] != "" {
		t.Fatal("a ritual-only spell offered spell slots", listen)
	}
	if slots := stringsOf(step["slots"]); len(slots) < 2 || slots[0] != "steps" {
		t.Fatal("a resource-cast spell did not offer its resource first", step)
	}
	next, err := ApplyCharacterPlay(result.Inputs, Object{"operation": "cast-granted-spell", "key": ward["key"], "slot": ""}, records, profile)
	if err != nil {
		t.Fatal(err)
	}
	for key, spent := range next.Inputs.Play.ResourceUses {
		if spent != 0 {
			t.Fatal("an at-will cast spent", key)
		}
	}
	if _, err = ApplyCharacterPlay(result.Inputs, Object{"operation": "cast-granted-spell", "key": listen["key"], "slot": "slot-1"}, records, profile); err == nil {
		t.Fatal("a ritual-only spell was cast with a slot")
	}
	next, err = ApplyCharacterPlay(result.Inputs, Object{"operation": "cast-granted-spell", "key": step["key"], "slot": "steps"}, records, profile)
	if err != nil || next.Inputs.Play.ResourceUses["steps"] != 1 {
		t.Fatal("a resource-cast spell did not spend its resource", err, next.Inputs.Play.ResourceUses)
	}
}

func TestSpellChoicesAndListsFollowTheGrantingClass(t *testing.T) {
	input, records, profile := spellGrantFixture(t)
	result := EvaluateCharacter(input, records, profile)
	for _, choice := range objects(result.SpellOptions["pendingChoices"]) {
		eligible := stringsOf(choice["eligibleSpellIds"])
		switch text(choice["key"]) {
		case "feature:singer-gifts:discoveries":
			if !contains(eligible, "blessing") || contains(eligible, "miracle") {
				t.Fatal("a class-level spell choice escaped the castable level", eligible)
			}
		case "feature:singer-gifts:rites":
			if len(eligible) != 1 || eligible[0] != "rite" {
				t.Fatal("a ritual choice offered a spell without the ritual tag", eligible)
			}
		}
	}
	singer := objects(result.SpellOptions["classes"])[0]
	if !contains(stringsOf(singer["spellIds"]), "blessing") || contains(stringsOf(singer["spellIds"]), "miracle") || hasChoiceIssue(result, "spell-eligibility:prepared:singer:blessing") {
		t.Fatal("an added class list did not reach preparation", singer["spellIds"], result.Issues)
	}
	input.Build.Levels = input.Build.Levels[:1]
	input.Build.Spells.GrantChoices = map[string][]string{}
	result = EvaluateCharacter(input, records, profile)
	if !hasChoiceIssue(result, "spell-eligibility:prepared:singer:blessing") {
		t.Fatal("an added list applied before its feature level", result.Issues)
	}
	input.Build.Levels = []character.Level{{ID: "one", ClassID: "mage"}, {ID: "two", ClassID: "mage"}, {ID: "three", ClassID: "mage"}}
	input.Play.PreparedSpells = nil
	result = EvaluateCharacter(input, records, profile)
	for _, caster := range objects(object(result.Sheet["spellcasting"])["perClass"]) {
		if integer(caster["spellbookKnown"], 0) != 6+2*2+3 || integer(caster["spellbookBonus"], 0) != 3 {
			t.Fatal("the spellbook bonus did not follow its class level", caster)
		}
	}
}
