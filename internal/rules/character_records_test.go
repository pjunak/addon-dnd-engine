package rules

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/pjunak/addon-dnd-engine/character"
)

type countedCharacterRecords struct {
	Records
	featureReads int
	nameReads    int
}

func (records *countedCharacterRecords) Values(kind string) []json.RawMessage {
	if kind == "feature" {
		records.featureReads++
	}
	return records.Records.Values(kind)
}
func (records *countedCharacterRecords) ValueByName(kind, name string) (json.RawMessage, bool) {
	records.nameReads++
	return records.Records.ValueByName(kind, name)
}
func (records *countedCharacterRecords) Provenance(string, string) SourceIdentity {
	return SourceIdentity{PackageID: "synthetic", PackageGeneration: "generation-one", ContentRevision: "revision-one"}
}

func TestHighLevelGuidanceUsesOneFeatureCatalog(t *testing.T) {
	input, source, profile := characterFixture(t)
	records := source.(memoryRecords)
	records.byKind["feature"] = map[string]json.RawMessage{}
	records.byKind["feat"] = map[string]json.RawMessage{}
	// Large catalogs and many alternatives must not trigger a source scan for
	// every offered feat at every prior character level.
	for i := 0; i < 256; i++ {
		id := fmt.Sprintf("catalog-feature-%03d", i)
		records.byKind["feature"][id] = mustJSON(Object{"kind": "feature", "id": id, "name": id, "classId": "another-class", "level": 1, "grants": Object{"languages": []any{"synthetic-language"}}})
	}
	for i := 0; i < 60; i++ {
		id := fmt.Sprintf("option-%02d", i)
		records.byKind["feat"][id] = mustJSON(Object{"kind": "feat", "id": id, "name": id, "category": "general"})
	}
	input.Build.Levels = nil
	for i := 1; i <= 20; i++ {
		input.Build.Levels = append(input.Build.Levels, character.Level{ID: fmt.Sprintf("level-%d", i), ClassID: "fighter"})
	}
	originalInput := cloneCharacter(input)
	originalSources := mustJSON(records.byKind)
	counted := &countedCharacterRecords{Records: records}
	result := EvaluateCharacter(input, counted, profile)
	if counted.featureReads != 1 || counted.nameReads != 0 {
		t.Fatalf("feature scans=%d; name lookups=%d", counted.featureReads, counted.nameReads)
	}
	if result.Ready || !truth(result.Guidance["canSave"]) {
		t.Fatalf("incomplete build changed legality: %+v", result.Issues)
	}
	options := objects(object(object(result.Guidance["choices"])["asi:fighter:4"])["featOptions"])
	if len(options) != 60 {
		t.Fatalf("eligible options=%d, want 60", len(options))
	}
	for i, option := range options {
		if text(option["id"]) != fmt.Sprintf("option-%02d", i) {
			t.Fatal("ineligible or unsorted option", option)
		}
	}
	if !reflect.DeepEqual(input, originalInput) || string(mustJSON(records.byKind)) != string(originalSources) {
		t.Fatal("evaluation mutated authored input or provider records")
	}
}

func TestCharacterEvaluationCacheIsDetachedAndRefreshesSources(t *testing.T) {
	input, source, profile := characterFixture(t)
	counted := &countedCharacterRecords{Records: source}
	first := EvaluateCharacter(input, counted, profile)
	original := mustJSON(first)
	for _, evidence := range first.Evidence {
		if evidence.PackageID != "synthetic" || evidence.PackageGeneration != "generation-one" || evidence.ContentRevision != "revision-one" {
			t.Fatal("lost source provenance", evidence)
		}
		evidence.Facts["name"] = "caller mutation"
	}
	object(first.Sheet["derived"])["maxHp"] = 999
	first.Inputs.Build.BaseScores["STR"] = 1
	if string(mustJSON(EvaluateCharacter(input, counted, profile))) != string(original) {
		t.Fatal("caller mutations escaped into the next evaluation")
	}
	records := source.(memoryRecords)
	class := recordByID(records, "class", "fighter")
	class["hitDie"] = "d6"
	records.byKind["class"]["fighter"] = mustJSON(class)
	changed := EvaluateCharacter(input, counted, profile)
	if integer(object(changed.Sheet["derived"])["maxHp"], 0) != 7 {
		t.Fatal("source change was hidden by cached records", changed.Sheet["derived"])
	}
	input.Build.Species = "Dwarf"
	if truth(EvaluateCharacter(input, counted, profile).Guidance["canSave"]) || counted.nameReads != 0 {
		t.Fatal("display name used as a durable source identity")
	}
}

func TestCharacterRecordCopiesKeepNestedStatePrivate(t *testing.T) {
	source := newMemoryRecords([]Object{{"kind": "feat", "id": "nested", "grants": Object{"choices": []any{Object{"id": "choice", "from": []any{"one", "two"}}}}}})
	cached := newCharacterRecordSnapshot(source)
	first := recordByID(cached, "feat", "nested")
	values(objects(object(first["grants"])["choices"])[0]["from"])[0] = "changed"
	second := recordByID(cached, "feat", "nested")
	if stringsOf(objects(object(second["grants"])["choices"])[0]["from"])[0] != "one" {
		t.Fatal("nested cached source mutated")
	}
	if recordByID(cached, "feat", "absent") != nil || recordByID(newCharacterRecordSnapshot(nil), "feat", "absent") != nil {
		t.Fatal("missing source invented")
	}
}
