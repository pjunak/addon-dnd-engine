package rules

import "testing"

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
	sheet := Hydrate(Object{"classes": []any{Object{"classId": "wizard", "level": 5}}}, records, &profile).Sheet
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
