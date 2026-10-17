package rules

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/pjunak/addon-dnd-engine/character"
)

func recordChoiceFixture(t *testing.T) (character.Inputs, memoryRecords, Ruleset) {
	t.Helper()
	input, source, profile := characterFixture(t)
	records := source.(memoryRecords)
	records.byKind["monster"] = map[string]json.RawMessage{
		"wolf":   mustJSON(Object{"kind": "monster", "id": "wolf", "name": "Wolf", "creatureType": "Beast", "crValue": 0.25, "speeds": Object{"walk": 40}}),
		"hawk":   mustJSON(Object{"kind": "monster", "id": "hawk", "name": "Hawk", "creatureType": "Beast", "crValue": 0, "speeds": Object{"walk": 10, "fly": 60}}),
		"bear":   mustJSON(Object{"kind": "monster", "id": "bear", "name": "Bear", "creatureType": "Beast", "crValue": 0.5, "speeds": Object{"walk": 30}}),
		"goblin": mustJSON(Object{"kind": "monster", "id": "goblin", "name": "Goblin", "creatureType": "Humanoid", "crValue": 0.25}),
	}
	records.byKind["magic-item"] = map[string]json.RawMessage{
		"lamp":  mustJSON(Object{"kind": "magic-item", "id": "lamp", "name": "Lamp", "rarity": "Common", "itemType": "Wondrous Item"}),
		"vial":  mustJSON(Object{"kind": "magic-item", "id": "vial", "name": "Vial", "rarity": "Common", "itemType": "Potion"}),
		"cloak": mustJSON(Object{"kind": "magic-item", "id": "cloak", "name": "Cloak", "rarity": "Uncommon", "itemType": "Wondrous Item"}),
	}
	records.byKind["feature"] = map[string]json.RawMessage{
		"fighter-forms": mustJSON(Object{"id": "fighter-forms", "kind": "feature", "name": "Forms", "classId": "fighter", "level": 1,
			"grants": Object{"choices": []any{
				Object{"id": "forms", "type": "enumerated", "optionKind": "monster", "count": 1, "countByLevel": Object{"2": 2},
					"filter":        Object{"creatureType": "Beast", "maxCr": 0.25, "fly": false},
					"filterByLevel": Object{"2": Object{"maxCr": 0.5}, "3": Object{"fly": true}}},
				Object{"id": "plans", "type": "enumerated", "optionKind": "magic-item", "count": 1,
					"filter": Object{"rarity": []any{"common"}, "notItemTypes": []any{"potion"}}, "fromByLevel": Object{"2": []any{"cloak"}}},
				Object{"id": "order", "type": "enumerated", "count": 1, "prompt": "Order", "from": []any{"warden", "magician"}},
			}}}),
	}
	return input, records, profile
}

func choiceOptionIDs(result character.Result, id string) []string {
	ids := []string{}
	for _, option := range objects(object(object(result.Guidance["choices"])[id])["options"]) {
		ids = append(ids, text(option["id"]))
	}
	return ids
}

func TestRecordChoicesOfferFilteredRecordsByClassLevel(t *testing.T) {
	input, records, profile := recordChoiceFixture(t)
	result := EvaluateCharacter(input, records, profile)
	if forms := choiceOptionIDs(result, "forms"); !reflect.DeepEqual(forms, []string{"wolf"}) {
		t.Fatal("level 1 forms ignored the type, challenge or flight filter", forms)
	}
	if plans := choiceOptionIDs(result, "plans"); !reflect.DeepEqual(plans, []string{"lamp"}) {
		t.Fatal("level 1 plans ignored the rarity or item type filter", plans)
	}
	input.Build.Levels = append(input.Build.Levels, character.Level{ID: "two", ClassID: "fighter"}, character.Level{ID: "three", ClassID: "fighter"})
	result = EvaluateCharacter(input, records, profile)
	if forms := choiceOptionIDs(result, "forms"); !reflect.DeepEqual(forms, []string{"bear", "hawk", "wolf"}) {
		t.Fatal("level steps did not raise the challenge rating or allow flight", forms)
	}
	if plans := choiceOptionIDs(result, "plans"); !reflect.DeepEqual(plans, []string{"cloak", "lamp"}) {
		t.Fatal("a level's added plans were not offered", plans)
	}
}

func TestSelectionsShowChosenOptionsWithoutFeatures(t *testing.T) {
	input, records, profile := recordChoiceFixture(t)
	input.Build.Choices = []character.Choice{{ID: "forms", Value: mustJSON("wolf")}, {ID: "plans", Value: mustJSON("lamp")}, {ID: "order", Value: mustJSON("warden")}}
	result := EvaluateCharacter(input, records, profile)
	if !result.Ready {
		t.Fatal(result.Issues)
	}
	shown := map[string]Object{}
	for _, row := range objects(result.Sheet["selections"]) {
		shown[text(row["id"])] = objects(row["values"])[0]
	}
	if text(shown["forms"]["kind"]) != "monster" || text(shown["forms"]["label"]) != "Wolf" || text(shown["order"]["label"]) != "Warden" || shown["order"]["kind"] != nil {
		t.Fatal("chosen options were not listed with their labels and kinds", shown)
	}
	if _, retained := evidenceFor(result, "monster", "wolf"); !retained {
		t.Fatal("a chosen stat block was not retained for offline reading")
	}
	if _, retained := evidenceFor(result, "monster", "bear"); retained {
		t.Fatal("an unchosen stat block was retained")
	}
}
