package rules

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

type parityRecords map[string][]json.RawMessage

func (records parityRecords) Value(kind, id string) (json.RawMessage, bool) {
	for _, body := range records[kind] {
		value, _ := DecodeObject(body)
		if text(value["id"]) == id {
			return body, true
		}
	}
	return nil, false
}
func (records parityRecords) ValueByName(kind, name string) (json.RawMessage, bool) {
	for _, body := range records[kind] {
		value, _ := DecodeObject(body)
		if strings.EqualFold(text(value["name"]), name) {
			return body, true
		}
	}
	return nil, false
}
func (records parityRecords) Values(kind string) []json.RawMessage { return records[kind] }

func TestPreservedV1Parity(t *testing.T) {
	var fixture struct {
		Rulesets map[string]json.RawMessage `json:"rulesets"`
		Records  parityRecords              `json:"records"`
		Cases    []struct {
			Name, Edition, Operation string
			Input                    Object
			Expected                 any
			ClassWeaponProficiencies []any
		} `json:"cases"`
	}
	body, err := os.ReadFile(filepath.Join("..", "..", "testdata", "v1-parity.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(body, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Cases) == 0 {
		t.Fatal("preserved comparison fixture contains no cases")
	}
	profiles := map[string]Ruleset{}
	for edition, raw := range fixture.Rulesets {
		profile, err := DecodeRuleset(raw)
		if err != nil {
			t.Fatal(err)
		}
		profiles[edition] = profile
	}
	for _, vector := range fixture.Cases {
		t.Run(vector.Name, func(t *testing.T) {
			profile := profiles[vector.Edition]
			before, _ := json.Marshal(vector.Input)
			var result any
			switch vector.Operation {
			case "hydrate":
				if vector.Edition == "" {
					result = HydrateWithoutRulesData(vector.Input, "missing")
				} else {
					result = Hydrate(NormalizeBuilderDecisions(vector.Input, fixture.Records, profile), fixture.Records, &profile)
				}
			case "builder-plan":
				result = BuilderPlan(vector.Input, fixture.Records, profile)
			case "apply":
				result = ApplyBuilderChoice(object(vector.Input["decisions"]), object(vector.Input["change"]), fixture.Records, profile)
			case "reconcile":
				result = ReconcileBuilderDecisions(vector.Input, fixture.Records, profile)
			default:
				t.Fatal("Unknown vector operation")
			}
			after, _ := json.Marshal(vector.Input)
			if string(before) != string(after) {
				t.Fatal("Engine mutated input decisions")
			}
			encoded, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			var actual any
			if err = json.Unmarshal(encoded, &actual); err != nil {
				t.Fatal(err)
			}
			if len(vector.ClassWeaponProficiencies) > 0 {
				proficiencies := object(object(object(vector.Expected)["sheet"])["proficiencies"])
				if !reflect.DeepEqual(proficiencies["weapons"], []any{}) {
					t.Fatal("reviewed class weapon correction no longer matches the v1 baseline")
				}
				proficiencies["weapons"] = vector.ClassWeaponProficiencies
			}
			if diff := parityDifference(vector.Expected, actual, "$"); diff != "" {
				t.Error(diff)
			}
		})
	}
}

func parityDifference(expected, actual any, path string) string {
	if reflect.DeepEqual(expected, actual) {
		return ""
	}
	if left, ok := expected.(map[string]any); ok {
		if right, ok := actual.(map[string]any); ok {
			keys := make([]string, 0, len(left)+len(right))
			seen := map[string]bool{}
			for key := range left {
				seen[key] = true
			}
			for key := range right {
				seen[key] = true
			}
			for key := range seen {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			differences := make([]string, 0)
			for _, key := range keys {
				l, lp := left[key]
				r, rp := right[key]
				if lp != rp {
					differences = append(differences, fmt.Sprintf("%s.%s: field presence differs (v1 %t, Go %t)", path, key, lp, rp))
					continue
				}
				if diff := parityDifference(l, r, path+"."+key); diff != "" {
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
				if diff := parityDifference(left[index], right[index], fmt.Sprintf("%s[%d]", path, index)); diff != "" {
					differences = append(differences, diff)
				}
			}
			return strings.Join(differences, "\n")
		}
	}
	return fmt.Sprintf("%s: v1=%v Go=%v", path, expected, actual)
}
