package rules

import (
	"encoding/json"
	"testing"

	"github.com/pjunak/addon-dnd-engine/character"
)

func TestBuilderResultsDoNotExposeBorrowedRecordsOrCallerDecisions(t *testing.T) {
	input, records, profile := multiclassCatalogFixture(t)
	fighter := recordByID(records, "class", "fighter")
	fighter["grants"] = Object{
		"choices": []any{Object{"id": "path", "type": "enumerated", "from": []any{"first", "second"}}},
		"choicePackages": []any{Object{"choiceId": "path", "options": Object{"first": Object{
			"choices": []any{Object{"id": "branch", "type": "enumerated", "from": []any{"north", "south"}}},
		}}}},
	}
	records.byKind["class"]["fighter"] = mustJSON(fighter)
	for _, kind := range []string{"subclass", "feature"} {
		record := Object{"kind": kind, "id": kind + "-training", "classId": "fighter", "level": 1, "subclassLevel": 3,
			"grants": Object{"choices": []any{Object{"id": kind + "-choice", "type": "enumerated", "from": []any{"one", "two"}}}}}
		if records.byKind[kind] == nil {
			records.byKind[kind] = map[string]json.RawMessage{}
		}
		records.byKind[kind][kind+"-training"] = mustJSON(record)
	}
	input.Build.Subclasses["fighter"] = "subclass-training"
	input.Build.Choices = append(input.Build.Choices, character.Choice{ID: "path", Value: mustJSON("first")})
	decisions := characterDecisions(input, records, profile, &character.Result{})
	originalSources, originalDecisions := string(mustJSON(records.byKind)), string(mustJSON(decisions))
	cached := newCharacterRecordSnapshot(records)
	operations := []struct {
		name string
		run  func(Records) Object
	}{
		{"plan", func(source Records) Object { return BuilderPlan(decisions, source, profile) }},
		{"normalize", func(source Records) Object { return NormalizeBuilderDecisions(decisions, source, profile) }},
		{"choose", func(source Records) Object {
			return ApplyBuilderChoice(decisions, Object{"choiceId": "asi:explorer:4:feat", "value": "training-01"}, source, profile)
		}},
	}
	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			want := string(mustJSON(operation.run(records)))
			got := operation.run(cached)
			if string(mustJSON(got)) != want {
				t.Fatal("borrowing source records changed builder output")
			}
			mutateBuilderOutput(got)
			if string(mustJSON(operation.run(cached))) != want {
				t.Fatal("mutating builder output changed subsequent work in the same calculation")
			}
			if string(mustJSON(decisions)) != originalDecisions || string(mustJSON(records.byKind)) != originalSources {
				t.Fatal("builder output changed caller decisions or source records")
			}
		})
	}
	plan := BuilderPlan(decisions, cached, profile)
	for _, id := range []string{"path", "branch", "subclass-choice", "feature-choice"} {
		if findChoice(objects(plan["classChoices"]), id) == nil {
			t.Fatal("fixture did not exercise", id)
		}
	}
	if findChoice(objects(plan["creationChoices"]), "feat:training-00:language") == nil {
		t.Fatal("fixture did not exercise selected feat choices")
	}
}

func mutateBuilderOutput(value any) {
	if fields := object(value); fields != nil {
		for _, child := range fields {
			mutateBuilderOutput(child)
		}
		fields["caller-mutation"] = true
	} else if items := values(value); items != nil {
		for index, child := range items {
			mutateBuilderOutput(child)
			items[index] = "caller-mutation"
		}
	}
}
