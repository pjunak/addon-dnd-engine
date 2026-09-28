package rules

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/pjunak/addon-dnd-engine/character"
)

// Synthetic source data keeps the cost of a large multiclass option catalog
// reproducible without depending on a privately installed content package.
func multiclassCatalogFixture(t testing.TB) (character.Inputs, memoryRecords, Ruleset) {
	t.Helper()
	input, source, profile := characterFixture(t)
	records := source.(memoryRecords)
	for _, id := range []string{"fighter", "scholar", "explorer"} {
		records.byKind["class"][id] = mustJSON(Object{"kind": "class", "id": id, "name": id,
			"hitDie": "d8", "multiclassPrerequisites": Object{"abilities": Object{"INT": 10}},
			"startingProficiencies": Object{"skills": Object{"choose": 1, "from": []any{"arcana", "history"}}}})
	}
	input.Build.Levels = nil
	for index, id := range []string{"fighter", "fighter", "fighter", "fighter", "scholar", "fighter", "fighter", "fighter", "explorer", "explorer", "explorer", "explorer"} {
		input.Build.Levels = append(input.Build.Levels, character.Level{ID: fmt.Sprintf("level-%d", index), ClassID: id})
	}
	records.byKind["feature"] = map[string]json.RawMessage{}
	for index := 0; index < 128; index++ {
		id := fmt.Sprintf("feature-%03d", index)
		records.byKind["feature"][id] = mustJSON(Object{"kind": "feature", "id": id, "classId": "another-class", "level": 1,
			"grants": Object{"languages": []any{"synthetic-language"}}})
	}
	records.byKind["feat"] = map[string]json.RawMessage{}
	for index := 0; index < 32; index++ {
		id := fmt.Sprintf("training-%02d", index)
		records.byKind["feat"][id] = mustJSON(Object{"kind": "feat", "id": id, "name": id, "category": "general",
			"prerequisites": Object{"all": []any{Object{"level": 4}, Object{"abilities": Object{"INT": 11 + index%3}}}},
			"grants": Object{"choices": []any{Object{"id": "language", "type": "language", "count": 1,
				"from": []any{"first-language", "second-language"}}}}})
	}
	input.Build.Choices = []character.Choice{{ID: "skills:fighter", Value: mustJSON("history")},
		{ID: "asi:fighter:4", Value: mustJSON("feat")}, {ID: "asi:fighter:4:feat", Value: mustJSON("training-00")},
		{ID: "feat:training-00:language", Value: mustJSON("first-language")}}
	input.Play.Currency["gp"] = 37
	input.Notes = "Keep authored session state"
	return input, records, profile
}

func BenchmarkMulticlassCharacterCatalog(b *testing.B) {
	input, records, profile := multiclassCatalogFixture(b)
	b.ReportAllocs()
	for b.Loop() {
		result := EvaluateCharacter(input, records, profile)
		if !truth(result.Guidance["canSave"]) || len(objects(result.Guidance["classOptions"])) != 3 {
			b.Fatal("benchmark did not retain a legal unfinished multiclass build", result.Issues)
		}
	}
}
