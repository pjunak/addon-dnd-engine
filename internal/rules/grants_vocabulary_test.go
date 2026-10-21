package rules

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/pjunak/addon-dnd-engine/character"
)

func TestBadGrantsAreReportedNotCoerced(t *testing.T) {
	input, records, profile := characterFixture(t)
	baseline := EvaluateCharacter(input, records, profile)
	records.(memoryRecords).byKind["species"]["dwarf"] = mustJSON(Object{"kind": "species", "id": "dwarf", "name": "Dwarf", "speeds": Object{"walk": 30},
		"grants": Object{"hpBonus": "5", "senses": Object{"darkvision": 60}, "flight": true}})
	result := EvaluateCharacter(input, records, profile)
	var warning *character.Issue
	for index := range result.Issues {
		if result.Issues[index].ID == "rules-data:species:dwarf" {
			warning = &result.Issues[index]
		}
	}
	if warning == nil || warning.Severity != character.SeverityWarning || !result.Ready {
		t.Fatalf("bad grants were not reported as a warning: %+v", result.Issues)
	}
	if result.Sheet.Derived.MaxHP != baseline.Sheet.Derived.MaxHP || result.Sheet.Senses["darkvision"] != 60 {
		t.Fatal("a numeric string was read as a number, or valid grants were dropped", result.Sheet.Derived.MaxHP, result.Sheet.Senses)
	}
}

func TestFixtureGrantsFollowTheVocabulary(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "testdata", "regression-vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture regressionFixture
	if err := json.Unmarshal(body, &fixture); err != nil {
		t.Fatal(err)
	}
	sources := []json.RawMessage{}
	for _, list := range fixture.Records {
		sources = append(sources, list...)
	}
	for _, list := range syntheticRecords().byKind {
		for _, body := range list {
			sources = append(sources, body)
		}
	}
	for _, body := range sources {
		record, _ := DecodeObject(body)
		if err := validateGrants(record["grants"]); err != nil {
			t.Errorf("%s:%s grants: %v", text(record["kind"]), text(record["id"]), err)
		}
	}
}

func TestPackagedGrantsSchemaMatchesTheVocabulary(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "contracts", "grants.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var packaged map[string]any
	if err := json.Unmarshal(body, &packaged); err != nil {
		t.Fatal(err)
	}
	generated := view(character.Schema(Grants{}))
	delete(packaged, "$schema")
	delete(packaged, "title")
	if string(mustJSON(view(packaged))) != string(mustJSON(generated)) {
		t.Fatal("contracts/grants.schema.json is stale; run go run ./cmd/character-contract")
	}
}
