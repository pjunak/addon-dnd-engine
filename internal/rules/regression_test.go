package rules

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

var updateVectors = flag.Bool("update", false, "rewrite testdata/regression-vectors.json from current results")

// The vectors pin complete Hydrate, BuilderPlan and ApplyBuilderChoice results
// for representative 2014 and 2024 characters. After a deliberate behaviour
// change, run `go test ./internal/rules -run TestRegressionVectors -update`
// and review the fixture diff.
type regressionFixture struct {
	Rulesets map[string]json.RawMessage `json:"rulesets"`
	Records  vectorRecords              `json:"records"`
	Cases    []regressionVector         `json:"cases"`
}

type regressionVector struct {
	Name      string `json:"name"`
	Edition   string `json:"edition"`
	Operation string `json:"operation"`
	Input     Object `json:"input"`
	Expected  any    `json:"expected"`
}

type vectorRecords map[string][]json.RawMessage

func (records vectorRecords) Value(kind, id string) (json.RawMessage, bool) {
	for _, body := range records[kind] {
		value, _ := DecodeObject(body)
		if text(value["id"]) == id {
			return body, true
		}
	}
	return nil, false
}

func (records vectorRecords) ValueByName(kind, name string) (json.RawMessage, bool) {
	for _, body := range records[kind] {
		value, _ := DecodeObject(body)
		if strings.EqualFold(text(value["name"]), name) {
			return body, true
		}
	}
	return nil, false
}

func (records vectorRecords) Values(kind string) []json.RawMessage { return records[kind] }

func TestRegressionVectors(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "regression-vectors.json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fixture regressionFixture
	if err = json.Unmarshal(body, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Cases) == 0 {
		t.Fatal("regression fixture contains no cases")
	}
	profiles := map[string]Ruleset{}
	for edition, raw := range fixture.Rulesets {
		profile, err := DecodeRuleset(raw)
		if err != nil {
			t.Fatal(err)
		}
		profiles[edition] = profile
	}
	for index := range fixture.Cases {
		vector := &fixture.Cases[index]
		t.Run(vector.Name, func(t *testing.T) {
			actual := runRegressionVector(t, *vector, fixture.Records, profiles[vector.Edition])
			if *updateVectors {
				vector.Expected = actual
				return
			}
			if diff := vectorDifference(vector.Expected, actual, "$"); diff != "" {
				t.Error(diff)
			}
		})
	}
	if *updateVectors {
		updated, err := json.Marshal(fixture)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, append(updated, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func runRegressionVector(t *testing.T, vector regressionVector, records vectorRecords, profile Ruleset) any {
	t.Helper()
	before, _ := json.Marshal(vector.Input)
	var result any
	switch vector.Operation {
	case "hydrate":
		if vector.Edition == "" {
			result = HydrateWithoutRulesData(vector.Input, "missing")
		} else {
			result = Hydrate(NormalizeBuilderDecisions(vector.Input, records, profile), records, &profile)
		}
	case "builder-plan":
		result = BuilderPlan(vector.Input, records, profile)
	case "apply":
		result = ApplyBuilderChoice(object(vector.Input["decisions"]), object(vector.Input["change"]), records, profile)
	default:
		t.Fatalf("unknown vector operation %q", vector.Operation)
	}
	if after, _ := json.Marshal(vector.Input); string(before) != string(after) {
		t.Fatal("engine mutated the input decisions")
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var actual any
	if err = json.Unmarshal(encoded, &actual); err != nil {
		t.Fatal(err)
	}
	return actual
}

func vectorDifference(expected, actual any, path string) string {
	if reflect.DeepEqual(expected, actual) {
		return ""
	}
	if left, ok := expected.(map[string]any); ok {
		if right, ok := actual.(map[string]any); ok {
			keys := make([]string, 0, len(left)+len(right))
			for key := range left {
				keys = append(keys, key)
			}
			for key := range right {
				if _, shared := left[key]; !shared {
					keys = append(keys, key)
				}
			}
			sort.Strings(keys)
			differences := make([]string, 0)
			for _, key := range keys {
				l, inExpected := left[key]
				r, inActual := right[key]
				if inExpected != inActual {
					differences = append(differences, fmt.Sprintf("%s.%s: present in expected %t, actual %t", path, key, inExpected, inActual))
					continue
				}
				if diff := vectorDifference(l, r, path+"."+key); diff != "" {
					differences = append(differences, diff)
				}
			}
			return strings.Join(differences, "\n")
		}
	}
	if left, ok := expected.([]any); ok {
		if right, ok := actual.([]any); ok && len(left) == len(right) {
			differences := make([]string, 0)
			for index := range left {
				if diff := vectorDifference(left[index], right[index], fmt.Sprintf("%s[%d]", path, index)); diff != "" {
					differences = append(differences, diff)
				}
			}
			return strings.Join(differences, "\n")
		}
	}
	return fmt.Sprintf("%s: expected %v, actual %v", path, expected, actual)
}
