package rules

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/pjunak/addon-dnd-engine/character"
)

func replacementFixture(t *testing.T, spells bool) (character.Inputs, memoryRecords, Ruleset, Object) {
	t.Helper()
	input, source, profile := characterFixture(t)
	records := source.(memoryRecords)
	input.Build.Levels = append(input.Build.Levels, character.Level{ID: "level-two", ClassID: "fighter"})
	grants := Object{"choices": []any{Object{"id": "training-pick", "type": "feat", "count": 1, "category": "style", "changeOn": "classLevel", "levelReplacements": 1}}}
	change := Object{"operation": "replace-class-choice", "kind": "feat", "key": "training-pick", "out": "first", "ref": "second"}
	input.Build.Choices = []character.Choice{{ID: "training-pick", Value: mustJSON("first")}}
	records.byKind["feat"] = map[string]json.RawMessage{}
	for _, id := range []string{"first", "second", "later"} {
		record := Object{"id": id, "kind": "feat", "name": id, "category": "style", "repeatable": false}
		if id == "later" {
			record["prerequisites"] = Object{"level": 3}
		}
		records.byKind["feat"][id] = mustJSON(record)
	}
	if spells {
		input.Build.Choices = []character.Choice{}
		grants = Object{"castingAbility": Object{"fixed": "WIS"}, "spells": []any{Object{"id": "chants", "choose": 2, "spellLevel": 0, "from": Object{"class": []any{"other"}}, "classId": "fighter", "alwaysPrepared": true, "levelReplacements": 1}}}
		input.Build.Spells.GrantChoices["feature:training:chants"] = []string{"first", "second"}
		records.byKind["spell"] = map[string]json.RawMessage{}
		for _, id := range []string{"first", "second", "third", "wrong"} {
			list := "other"
			if id == "wrong" {
				list = "unrelated"
			}
			records.byKind["spell"][id] = mustJSON(Object{"id": id, "kind": "spell", "name": id, "level": 0, "classes": []any{list}})
		}
		change = Object{"operation": "replace-class-choice", "kind": "spell", "key": "feature:training:chants", "out": "first", "ref": "third"}
	}
	records.byKind["feature"] = map[string]json.RawMessage{"training": mustJSON(Object{"id": "training", "kind": "feature", "name": "Training", "classId": "fighter", "level": 1, "grants": grants})}
	return input, records, profile, change
}

func TestClassReplacementIsBoundedAndPreservesAuthoredState(t *testing.T) {
	for _, spells := range []bool{false, true} {
		t.Run(map[bool]string{false: "feat", true: "spell"}[spells], func(t *testing.T) {
			input, records, profile, change := replacementFixture(t, spells)
			original := cloneCharacter(input)
			first, err := ApplyCharacterPlay(input, change, records, profile)
			if err != nil || !first.Ready {
				t.Fatalf("replace: %v %+v", err, first.Issues)
			}
			if !reflect.DeepEqual(input, original) || !reflect.DeepEqual(first.Inputs.Play, input.Play) {
				t.Fatal("replacement mutated original or play state")
			}
			if len(first.Inputs.Build.Replacements) != 1 || first.Inputs.Build.Replacements[0].ClassLevel != 2 {
				t.Fatal("missing consumed allowance")
			}
			if spells && !reflect.DeepEqual(first.Inputs.Build.Spells.GrantChoices["feature:training:chants"], []string{"third", "second"}) {
				t.Fatal("lost sibling cantrip")
			}
			if _, err = ApplyCharacterPlay(first.Inputs, change, records, profile); err == nil {
				t.Fatal("allowed repeated replacement")
			}
			rested, err := ApplyCharacterPlay(first.Inputs, Object{"operation": "rest", "rest": "long"}, records, profile)
			if err != nil || !reflect.DeepEqual(rested.Inputs.Build.Replacements, first.Inputs.Build.Replacements) {
				t.Fatal("rest lost ledger", err)
			}
			if integer(objects(rested.Guidance["classReplacements"])[0]["remaining"], -1) != 0 {
				t.Fatal("rest refreshed allowance")
			}
			imported := cloneCharacter(rested.Inputs)
			imported.Build.Replacements[0].Origin = "import"
			if integer(objects(EvaluateCharacter(imported, records, profile).Guidance["classReplacements"])[0]["remaining"], -1) != 0 {
				t.Fatal("import refreshed allowance")
			}
			records.byKind["class"]["other"] = mustJSON(Object{"id": "other", "kind": "class", "hitDie": "d8", "multiclassPrerequisites": Object{}})
			multiclass := cloneCharacter(first.Inputs)
			multiclass.Build.Levels = append(multiclass.Build.Levels, character.Level{ID: "other-level", ClassID: "other"})
			if integer(objects(EvaluateCharacter(multiclass, records, profile).Guidance["classReplacements"])[0]["remaining"], -1) != 0 {
				t.Fatal("other class refreshed allowance")
			}
			lower := cloneCharacter(first.Inputs)
			lower.Build.Levels = lower.Build.Levels[:1]
			if len(objects(EvaluateCharacter(lower, records, profile).Guidance["classReplacements"])) != 0 {
				t.Fatal("initial level granted replacement")
			}
			lower.Build.Levels = append(lower.Build.Levels, character.Level{ID: "different-id", ClassID: "fighter"})
			if integer(objects(EvaluateCharacter(lower, records, profile).Guidance["classReplacements"])[0]["remaining"], -1) != 0 {
				t.Fatal("recreating level refreshed allowance")
			}
			first.Inputs.Build.Levels = append(first.Inputs.Build.Levels, character.Level{ID: "level-three", ClassID: "fighter"})
			change["out"], change["ref"] = change["ref"], change["out"]
			next, err := ApplyCharacterPlay(first.Inputs, change, records, profile)
			if err != nil || !next.Ready || len(next.Inputs.Build.Replacements) != 2 {
				t.Fatal("next class level did not allow return to earlier selection", err, next.Issues)
			}
		})
	}
}

func TestClassReplacementRejectsWrongOwnerAndIneligibleSelections(t *testing.T) {
	for _, spells := range []bool{false, true} {
		input, records, profile, valid := replacementFixture(t, spells)
		for _, patch := range []Object{{"key": "other-choice"}, {"kind": "unknown"}, {"out": "not-selected"}, {"ref": "first"}, {"ref": "wrong"}, {"extra": true}} {
			change := cloneObjectDeep(valid)
			for key, value := range patch {
				change[key] = value
			}
			if _, err := ApplyCharacterPlay(input, change, records, profile); err == nil {
				t.Fatalf("accepted %v", change)
			}
		}
		feature := recordByID(records, "feature", "training")
		declaration := objects(object(feature["grants"])["choices"])
		if spells {
			declaration = objects(object(feature["grants"])["spells"])
		}
		delete(declaration[0], "levelReplacements")
		records.byKind["feature"]["training"] = mustJSON(feature)
		if _, err := ApplyCharacterPlay(input, valid, records, profile); err == nil {
			t.Fatal("undeclared source allowance was inferred")
		}
	}
}

func TestReplacementPrerequisitesUseTheReplacementLevel(t *testing.T) {
	input, records, profile, change := replacementFixture(t, false)
	change["ref"] = "later"
	if _, err := ApplyCharacterPlay(input, change, records, profile); err == nil {
		t.Fatal("early feat allowed")
	}
	input.Build.Levels = append(input.Build.Levels, character.Level{ID: "third", ClassID: "fighter"})
	next, err := ApplyCharacterPlay(input, change, records, profile)
	if err != nil || !next.Ready {
		t.Fatal("eligible later acquisition rejected", err, next.Issues)
	}
	if !EvaluateCharacter(next.Inputs, records, profile).Ready {
		t.Fatal("replacement cannot be reloaded")
	}
	for _, option := range objects(object(object(next.Guidance["choices"])["training-pick"])["options"]) {
		if text(option["id"]) == "later" {
			return
		}
	}
	t.Fatal("saved replacement missing from Builder eligibility")
}

func TestClassReplacementHistorySurvivesSourceWithdrawalAndBuilderCorrection(t *testing.T) {
	input, records, profile, change := replacementFixture(t, false)
	next, err := ApplyCharacterPlay(input, change, records, profile)
	if err != nil {
		t.Fatal(err)
	}
	feature := records.byKind["feature"]["training"]
	delete(records.byKind["feature"], "training")
	unavailable := EvaluateCharacter(next.Inputs, records, profile)
	if len(objects(unavailable.Guidance["classReplacements"])) != 0 || !reflect.DeepEqual(unavailable.Inputs.Build.Replacements, next.Inputs.Build.Replacements) {
		t.Fatal("missing source erased or reissued a replacement")
	}
	records.byKind["feature"]["training"] = feature
	corrected := cloneCharacter(next.Inputs)
	corrected.Build.Choices[0].Value = mustJSON("first")
	result := EvaluateCharacter(corrected, records, profile)
	if !result.Ready || integer(objects(result.Guidance["classReplacements"])[0]["remaining"], -1) != 0 {
		t.Fatal("ordinary correction should remain possible without resetting the allowance", result.Issues)
	}
}

func TestMalformedClassReplacementHistoryBlocksSaving(t *testing.T) {
	input, records, profile, change := replacementFixture(t, false)
	next, err := ApplyCharacterPlay(input, change, records, profile)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*character.ClassReplacement){
		"origin":      func(e *character.ClassReplacement) { e.Origin = "claimed" },
		"source":      func(e *character.ClassReplacement) { e.Source.Kind = "spell" },
		"owner":       func(e *character.ClassReplacement) { e.ClassID = "" },
		"key":         func(e *character.ClassReplacement) { e.Key = "" },
		"level":       func(e *character.ClassReplacement) { e.ClassLevel = 1 },
		"slot":        func(e *character.ClassReplacement) { e.Slot = -1 },
		"same choice": func(e *character.ClassReplacement) { e.Out = e.In },
	} {
		t.Run(name, func(t *testing.T) {
			invalid := cloneCharacter(next.Inputs)
			mutate(&invalid.Build.Replacements[0])
			if EvaluateCharacter(invalid, records, profile).Guidance["canSave"] != false {
				t.Fatal("invalid ledger was saveable")
			}
		})
	}
	next.Inputs.Build.Replacements = append(next.Inputs.Build.Replacements, next.Inputs.Build.Replacements[0])
	if EvaluateCharacter(next.Inputs, records, profile).Guidance["canSave"] != false {
		t.Fatal("duplicate allowance was saveable")
	}
}
