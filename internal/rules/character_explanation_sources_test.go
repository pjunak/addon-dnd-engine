package rules

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/pjunak/addon-dnd-engine/character"
)

func TestCharacterExplanationsScopeMulticlassAndAcquisitionSources(t *testing.T) {
	ref := func(kind, id string) character.Reference { return character.Reference{Kind: kind, ID: id} }
	refs := []character.Reference{ref("class", "fighter"), ref("class", "warlock"), ref("class", "wizard"),
		ref("subclass", "arcane"), ref("species", "hearty"), ref("background", "scholar"),
		ref("feat", "training"), ref("feature", "unrelated"), ref("magic-item", "charm")}
	evidence := []character.Evidence{}
	for _, r := range refs {
		facts := Object{}
		switch r.ID {
		case "hearty":
			facts["grants"] = Object{"hpPerLevel": 1}
		case "scholar":
			facts["skillProficiencies"] = []any{"history"}
		case "training":
			facts["grants"] = Object{"choices": []any{Object{"type": "skillProficiency"}}}
		}
		evidence = append(evidence, character.Evidence{Reference: r, Facts: facts})
	}
	input := character.Blank()
	input.Build.Subclasses["fighter"] = "arcane"
	sheet := Object{
		"classes": []any{Object{"classId": "fighter", "hitDie": "d10"}, Object{"classId": "warlock", "hitDie": "d8"}, Object{"classId": "wizard", "hitDie": "d6"}},
		"skills":  Object{"history": Object{"ability": "INT", "proficient": true}, "arcana": Object{"ability": "INT", "proficient": false}},
		"resources": []any{
			Object{"key": "pact", "source": Object{"type": "pactMagic", "id": "warlock"}},
			Object{"key": "ordinary", "source": Object{"type": "spellcasting"}},
			Object{"key": "hit", "kind": "hitdice", "die": "d10", "source": Object{"type": "class"}},
			Object{"key": "grant-owned", "source": Object{"type": "feat", "id": "training", "acquisition": Object{"id": "grant:one"}}},
		},
		"spellcasting": Object{"perClass": []any{
			Object{"classId": "fighter", "ability": "INT"}, Object{"classId": "warlock", "ability": "CHA", "pact": Object{"slots": 1}}, Object{"classId": "wizard", "ability": "INT"},
		}},
	}
	decisions := Object{"abilityGrants": []any{Object{"assign": Object{"INT": 2}, "source": Object{"type": "background", "id": "scholar"}}}}
	want := map[string][]character.Reference{
		"abilities.INT.score":             {ref("background", "scholar"), ref("magic-item", "charm")},
		"abilities.INT.mod":               {ref("background", "scholar"), ref("magic-item", "charm")},
		"skills.history.total":            {ref("background", "scholar"), ref("feat", "training"), ref("magic-item", "charm")},
		"skills.arcana.total":             {ref("background", "scholar"), ref("magic-item", "charm")},
		"resources.pact.max":              {ref("class", "warlock")},
		"resources.ordinary.remaining":    {ref("class", "fighter"), ref("class", "wizard"), ref("subclass", "arcane")},
		"resources.hit.max":               {ref("class", "fighter")},
		"resources.grant-owned.remaining": {ref("feat", "training")},
		"spellcasting.perClass.0.saveDC":  {ref("background", "scholar"), ref("class", "fighter"), ref("magic-item", "charm"), ref("subclass", "arcane")},
		"derived.maxHp":                   {ref("class", "fighter"), ref("class", "warlock"), ref("class", "wizard"), ref("species", "hearty"), ref("subclass", "arcane")},
	}
	explanations := map[string]character.Explanation{}
	for path := range want {
		explanations[path] = character.Explanation{Label: path, Value: 7, Formula: "Retain formula", Terms: []character.Term{{Label: "Retain term", Value: 3}}, Sources: refs}
	}
	item := ref("magic-item", "charm")
	e := explanations["abilities.INT.score"]
	e.Terms = append(e.Terms, character.Term{Label: "Typed item effect", Value: 1, Source: &item, Status: "applied"})
	explanations["abilities.INT.score"] = e
	before := string(mustJSON(Object{"input": input, "sheet": sheet, "decisions": decisions, "evidence": evidence}))
	scopeCharacterExplanationSources(input, decisions, sheet, evidence, explanations)
	for path, sources := range want {
		actual := explanations[path]
		if !reflect.DeepEqual(actual.Sources, sources) {
			t.Errorf("%s: sources=%+v, want %+v", path, actual.Sources, sources)
		}
		if actual.Value != 7 || actual.Formula != "Retain formula" || actual.Terms[0].Value != 3 {
			t.Fatal("scoping changed a calculation", actual)
		}
	}
	scopeCharacterExplanationSources(input, decisions, sheet, evidence, explanations)
	for path, sources := range want {
		if !reflect.DeepEqual(explanations[path].Sources, sources) {
			t.Fatal("unstable repeated scoping", path)
		}
	}
	if before != string(mustJSON(Object{"input": input, "sheet": sheet, "decisions": decisions, "evidence": evidence})) {
		t.Fatal("explanation scoping changed authored state, rules or full evidence")
	}
}

func TestNarrativeFeaturesDoNotMultiplyResourceOrAbilityEvidence(t *testing.T) {
	input, source, profile := characterFixture(t)
	records := source.(memoryRecords)
	class := recordByID(records, "class", "fighter")
	class["classResources"] = []any{Object{"key": "focus", "fixed": 2, "recharge": "short"}}
	records.byKind["class"]["fighter"] = mustJSON(class)
	first := EvaluateCharacter(input, records, profile)
	if records.byKind["feature"] == nil {
		records.byKind["feature"] = map[string]json.RawMessage{}
	}
	for i := 0; i < 48; i++ {
		id := fmt.Sprintf("narrative-feature-%02d", i)
		records.byKind["feature"][id] = mustJSON(Object{"kind": "feature", "id": id, "name": id, "classId": "fighter", "level": 1, "text": "Retain offline source prose."})
	}
	before := cloneCharacter(input)
	result := EvaluateCharacter(input, records, profile)
	for _, path := range []string{"resources.focus.max", "resources.focus.remaining", "abilities.STR.score", "abilities.STR.mod", "skills.athletics.total", "derived.maxHp"} {
		if !reflect.DeepEqual(first.Explanations[path], result.Explanations[path]) {
			t.Fatal("unrelated features multiplied a calculation's sources", path)
		}
	}
	count := 0
	for _, entry := range result.Evidence {
		if entry.Reference.Kind == "feature" {
			count++
			if entry.Summary != "Retain offline source prose." || len(entry.Hash) != 64 {
				t.Fatal("lost retained evidence", entry)
			}
		}
	}
	if count < 48 || !reflect.DeepEqual(input, before) || !reflect.DeepEqual(result.Inputs, before) {
		t.Fatal("deduplication dropped full source evidence or changed authored inputs")
	}
}
