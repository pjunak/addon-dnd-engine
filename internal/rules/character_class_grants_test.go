package rules

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/pjunak/addon-dnd-engine/character"
)

func classGrantFixture(t *testing.T) (character.Inputs, memoryRecords, Ruleset) {
	t.Helper()
	input, source, profile := characterFixture(t)
	records := source.(memoryRecords)
	input.Build.Levels = append(input.Build.Levels, character.Level{ID: "level-two", ClassID: "fighter"})
	records.byKind["feature"] = map[string]json.RawMessage{
		"martial-training": mustJSON(Object{"id": "martial-training", "kind": "feature", "classId": "fighter", "level": 2,
			"grants": Object{
				"choices": []any{Object{"id": "training-path", "type": "enumerated", "count": 1, "from": []any{"style", "cantrips"}}},
				"choicePackages": []any{Object{"choiceId": "training-path", "options": Object{
					"style": Object{"choices": []any{Object{"id": "training-feat", "type": "feat", "count": 1, "category": "training"}}},
					"cantrips": Object{"castingAbility": Object{"fixed": "CHA"}, "spells": []any{
						Object{"id": "training-spells", "choose": 2, "spellLevel": 0, "from": Object{"class": []any{"priest"}}, "classId": "fighter", "alwaysPrepared": true},
					}},
				}}},
			}}),
		"second-training": mustJSON(Object{"id": "second-training", "kind": "feature", "classId": "fighter", "subclassId": "veteran", "level": 3,
			"grants": Object{"choices": []any{Object{"id": "second-feat", "type": "feat", "count": 1, "category": "training"}}}}),
	}
	records.byKind["subclass"] = map[string]json.RawMessage{"veteran": mustJSON(Object{"id": "veteran", "kind": "subclass", "classId": "fighter", "subclassLevel": 3})}
	records.byKind["feat"] = map[string]json.RawMessage{
		"guard":    mustJSON(Object{"id": "guard", "kind": "feat", "name": "Guard", "category": "training", "prerequisites": Object{"feature": "martial-training"}, "grants": Object{"senses": Object{"blindsight": 10}}}),
		"aim":      mustJSON(Object{"id": "aim", "kind": "feat", "name": "Aim", "category": "training", "prerequisites": Object{"feature": "martial-training"}}),
		"ordinary": mustJSON(Object{"id": "ordinary", "kind": "feat", "category": "general"}),
	}
	records.byKind["spell"] = map[string]json.RawMessage{}
	for _, spell := range []Object{
		{"id": "spark", "kind": "spell", "level": 0, "classes": []any{"priest"}},
		{"id": "light", "kind": "spell", "level": 0, "classes": []any{"priest"}},
		{"id": "higher", "kind": "spell", "level": 1, "classes": []any{"priest"}},
		{"id": "other", "kind": "spell", "level": 0, "classes": []any{"mage"}},
	} {
		records.byKind["spell"][text(spell["id"])] = mustJSON(spell)
	}
	return input, records, profile
}

func TestClassChoicePackagesUseTheSameBranchForPlanAndGrants(t *testing.T) {
	input, records, profile := classGrantFixture(t)
	input.Build.Choices = []character.Choice{{ID: "training-path", Value: mustJSON("style")}, {ID: "training-feat", Value: mustJSON("guard")}}
	input.Play.HP = 1
	input.Play.TemporaryHP = 2
	input.Play.Currency["gp"] = 17
	input.Notes = "Preserve authored state"
	before := cloneCharacter(input)
	result := EvaluateCharacter(input, records, profile)
	if !result.Ready || !classGrantOffers(result, "training-feat", "guard") || !classGrantOffers(result, "training-feat", "aim") || classGrantOffers(result, "training-feat", "ordinary") {
		t.Fatal("class-granted feat options disagree with validation", result.Issues)
	}
	if integer(object(result.Sheet["senses"])["blindsight"], 0) != 10 || len(objects(result.Sheet["feats"])) != 1 || !reflect.DeepEqual(input, before) || !reflect.DeepEqual(result.Inputs, before) {
		t.Fatal("choice grant was omitted or authored input changed", result.Sheet["feats"])
	}
	branch := cloneCharacter(input)
	branch.Build.Choices[0].Value = mustJSON("cantrips")
	result = EvaluateCharacter(branch, records, profile)
	if truth(result.Guidance["canSave"]) || !hasChoiceIssue(result, "unavailable-choice:training-feat#0") || len(objects(result.Sheet["feats"])) != 0 || integer(object(result.Sheet["senses"])["blindsight"], 0) != 0 {
		t.Fatal("an inactive package still grants its old feat", result.Issues)
	}
	branch.Build.Choices = branch.Build.Choices[:1]
	branch.Build.Spells.GrantChoices = map[string][]string{"feature:martial-training:training-spells": {"spark", "light"}}
	result = EvaluateCharacter(branch, records, profile)
	if !result.Ready || !reflect.DeepEqual(result.Inputs.Play, before.Play) {
		t.Fatal("class cantrip alternative did not become ready", result.Issues)
	}
	for _, spell := range objects(object(result.Sheet["spellcasting"])["granted"]) {
		if text(spell["castingAbility"]) != "CHA" || text(object(spell["source"])["classId"]) != "fighter" || text(object(spell["source"])["id"]) != "martial-training" {
			t.Fatal("lost casting ability, class membership or grant identity", spell)
		}
	}
	options := objects(result.SpellOptions["pendingChoices"])[0]
	if !reflect.DeepEqual(stringsOf(options["eligibleSpellIds"]), []string{"light", "spark"}) {
		t.Fatal("spell level or source list escaped its restriction", options)
	}
	branch.Build.Choices[0].Value = mustJSON("style")
	result = EvaluateCharacter(branch, records, profile)
	if !hasChoiceIssue(result, "spell-grant:feature:martial-training:training-spells") || len(objects(object(result.Sheet["spellcasting"])["granted"])) != 0 {
		t.Fatal("inactive alternative retained its spells", result.Issues)
	}
	branch.Build.Levels = branch.Build.Levels[:1]
	result = EvaluateCharacter(branch, records, profile)
	if !hasChoiceIssue(result, "unavailable-choice:training-path#0") {
		t.Fatal("feature survived withdrawal of its class level", result.Issues)
	}
}

func TestClassFeatGrantsKeepSubclassAcquisitionsDistinct(t *testing.T) {
	input, records, profile := classGrantFixture(t)
	input.Build.Levels = append(input.Build.Levels, character.Level{ID: "level-three", ClassID: "fighter"})
	input.Build.Subclasses = map[string]string{"fighter": "veteran"}
	input.Build.Choices = []character.Choice{
		{ID: "training-path", Value: mustJSON("style")}, {ID: "training-feat", Value: mustJSON("guard")}, {ID: "second-feat", Value: mustJSON("aim")},
	}
	result := EvaluateCharacter(input, records, profile)
	if !result.Ready || len(objects(result.Sheet["feats"])) != 2 || classGrantOffers(result, "second-feat", "guard") {
		t.Fatal("separate acquired feats were lost or offered twice", result.Issues)
	}
	input.Build.Choices[2].Value = mustJSON("guard")
	result = EvaluateCharacter(input, records, profile)
	if !hasFeatBlock(result, "guard") || truth(result.Guidance["canSave"]) {
		t.Fatal("duplicate nonrepeatable style was accepted", result.Issues)
	}
	delete(input.Build.Subclasses, "fighter")
	result = EvaluateCharacter(input, records, profile)
	if !hasChoiceIssue(result, "unavailable-choice:second-feat#0") {
		t.Fatal("subclass change retained the extra acquisition", result.Issues)
	}
}

func classGrantOffers(result character.Result, choice, id string) bool {
	for _, option := range objects(object(object(result.Guidance["choices"])[choice])["options"]) {
		if text(option["id"]) == id {
			return true
		}
	}
	return false
}
