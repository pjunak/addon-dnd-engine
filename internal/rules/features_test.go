package rules

import (
	"encoding/json"
	"testing"
)

func TestChosenSubclassFillsItsTableSlot(t *testing.T) {
	t.Parallel()
	records, profile := syntheticRecords(), syntheticRuleset(t)
	wizard := recordByID(records, "class", "wizard")
	wizard["progression"] = append(values(wizard["progression"]),
		Object{"level": 3, "features": []any{"Wizard Subclass"}},
		Object{"level": 6, "features": []any{"Subclass Feature"}})
	records.byKind["class"]["wizard"] = mustJSON(wizard)
	records.byKind["feature"]["wizard-arcane-tradition"] = mustJSON(Object{
		"kind": "feature", "id": "wizard-arcane-tradition", "name": "Wizard Subclass",
		"classId": "wizard", "level": 3,
	})
	records.byKind["subclass"] = map[string]json.RawMessage{"evoker": mustJSON(Object{
		"kind": "subclass", "id": "evoker", "name": "Evoker", "classId": "wizard",
		"features": []any{Object{"id": "wizard-evoker-sculpt", "name": "Sculpt Spells", "level": 3}},
	})}
	records.byKind["feature"]["wizard-evoker-sculpt"] = mustJSON(Object{
		"kind": "feature", "id": "wizard-evoker-sculpt", "name": "Sculpt Spells",
		"classId": "wizard", "subclassId": "evoker", "level": 3, "localId": "wizard-evoker-sculpt",
	})
	find := func(features []any, id string) Object {
		for _, raw := range features {
			if feature := object(raw); text(feature["id"]) == id {
				return feature
			}
		}
		return nil
	}
	// Without a chosen subclass the table slot and placeholder stay as printed.
	unchosen := hydrateView(Object{"classes": []any{Object{"classId": "wizard", "level": 6}}},
		records, &profile).Sheet
	if find(values(unchosen["features"]), "wizard-arcane-tradition") == nil {
		t.Fatalf("unchosen slot row missing in %v", unchosen["features"])
	}
	if find(values(unchosen["features"]), "Subclass Feature") == nil {
		t.Fatalf("unchosen placeholder missing in %v", unchosen["features"])
	}
	sheet := hydrateView(Object{
		"classes": []any{Object{"classId": "wizard", "level": 6, "subclass": "evoker"}},
	}, records, &profile).Sheet
	features := values(sheet["features"])
	chosen := find(features, "evoker")
	if chosen == nil || text(chosen["name"]) != "Evoker" || text(chosen["kind"]) != "subclass" ||
		integer(object(chosen["source"])["level"], 0) != 3 {
		t.Fatalf("chosen subclass row = %v in %v", chosen, features)
	}
	// The generic slot and placeholder rows are replaced, not kept beside it.
	if find(features, "wizard-arcane-tradition") != nil || find(features, "Subclass Feature") != nil {
		t.Fatalf("generic subclass rows remain in %v", features)
	}
	if find(features, "wizard-evoker-sculpt") == nil {
		t.Fatalf("subclass feature row missing in %v", features)
	}
}

func TestProgressionLabelsResolveSharedFeatures(t *testing.T) {
	t.Parallel()
	records, profile := syntheticRecords(), syntheticRuleset(t)
	wizard := recordByID(records, "class", "wizard")
	wizard["progression"] = append(values(wizard["progression"]),
		Object{"level": 4, "features": []any{"Ability Score Improvement", "Unlisted Gift"}})
	records.byKind["class"]["wizard"] = mustJSON(wizard)
	records.byKind["feature"]["ability-score-improvement"] = mustJSON(Object{
		"kind": "feature", "id": "ability-score-improvement", "name": "Ability Score Improvement",
		"classId": nil, "level": nil, "shared": true,
	})
	sheet := hydrateView(Object{"classes": []any{Object{"classId": "wizard", "level": 5}}}, records, &profile).Sheet
	var shared, unlisted Object
	for _, feature := range objects(sheet["features"]) {
		switch text(feature["id"]) {
		case "ability-score-improvement":
			shared = feature
		case "Unlisted Gift":
			unlisted = feature
		}
	}
	if text(shared["name"]) != "Ability Score Improvement" ||
		integer(object(shared["source"])["level"], 0) != 4 {
		t.Fatalf("shared feature = %v in %v", shared, sheet["features"])
	}
	// A label with no record keeps its label as the only identity.
	if unlisted == nil || unlisted["name"] != nil {
		t.Fatalf("unlisted feature = %v", unlisted)
	}
}
