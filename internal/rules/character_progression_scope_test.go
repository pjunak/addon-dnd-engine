package rules

import (
	"reflect"
	"strings"
	"testing"

	"github.com/pjunak/addon-dnd-engine/character"
)

func TestEditorProgressionChecksMatchFullValidation(t *testing.T) {
	variants := []struct {
		name   string
		change func(*character.Inputs, memoryRecords)
	}{
		{"self qualification", func(*character.Inputs, memoryRecords) {}},
		{"earlier ability increase", func(input *character.Inputs, _ memoryRecords) {
			input.Build.Choices = []character.Choice{
				{ID: "asi:fighter:4", Value: mustJSON("asi")},
				{ID: "asi:fighter:4:ability", Value: mustJSON(Object{"INT": 2})},
				{ID: "asi:fighter:8", Value: mustJSON("feat")},
				{ID: "asi:fighter:8:feat", Value: mustJSON("scholar")},
			}
		}},
		{"exact waiver", func(input *character.Inputs, _ memoryRecords) {
			input.Grants = []character.Grant{{ID: "review", Name: "Reviewed training", ActorID: "dm",
				Reason: "Adjudication", GrantedAt: input.Play.AsOf, Active: true, Condition: "always",
				EffectiveLevel: 1, Waivers: []string{"feat:veteran"}}}
		}},
		{"repeated acquisition", func(input *character.Inputs, _ memoryRecords) {
			input.Build.BaseScores["INT"] = 15
			input.Build.Choices[3].Value = mustJSON("scholar")
		}},
		{"parent and child feats", func(input *character.Inputs, records memoryRecords) {
			records.byKind["feat"]["mentor"] = mustJSON(Object{"kind": "feat", "id": "mentor", "category": "general",
				"grants": Object{"choices": []any{Object{"id": "student", "type": "feat", "count": 1, "from": []any{"scholar"}}}}})
			input.Build.Choices[1].Value = mustJSON("mentor")
			input.Build.Choices = append(input.Build.Choices, character.Choice{ID: "feat:mentor:student", Value: mustJSON("scholar")})
		}},
	}
	seen := map[string]bool{}
	for _, variant := range variants {
		t.Run(variant.name, func(t *testing.T) {
			input, records, profile := progressionChoiceInput(t)
			fighter := recordByID(records, "class", "fighter")
			fighter["multiclassPrerequisites"] = Object{}
			records.byKind["class"]["fighter"] = mustJSON(fighter)
			records.byKind["class"]["mage"] = mustJSON(Object{"kind": "class", "id": "mage", "hitDie": "d6",
				"multiclassPrerequisites": Object{"abilities": Object{"WIS": 13}}})
			levels := append([]character.Level(nil), input.Build.Levels...)
			input.Build.Levels = append(append(append([]character.Level{}, levels[:4]...),
				character.Level{ID: "mage-entry", ClassID: "mage"}), levels[4:]...)
			records.byKind["feat"]["scholar"] = mustJSON(Object{"kind": "feat", "id": "scholar", "category": "general",
				"prerequisites": Object{"abilities": Object{"INT": 13}},
				"grants":        Object{"abilityScoreIncrease": Object{"from": []any{"INT"}, "amount": 1}}})
			records.byKind["feat"]["veteran"] = mustJSON(Object{"kind": "feat", "id": "veteran", "category": "general",
				"prerequisites": Object{"text": "Requires an exact waiver"}})
			input.Build.Choices = []character.Choice{
				{ID: "asi:fighter:4", Value: mustJSON("feat")},
				{ID: "asi:fighter:4:feat", Value: mustJSON("scholar")},
				{ID: "asi:fighter:8", Value: mustJSON("feat")},
				{ID: "asi:fighter:8:feat", Value: mustJSON("veteran")},
			}
			variant.change(&input, records)
			original, sources := cloneCharacter(input), string(mustJSON(records.byKind))
			full := character.Result{Issues: []character.Issue{}}
			validateCharacterProgression(input, newCharacterRecordSnapshot(records), profile, &full)
			for _, issue := range full.Issues {
				seen[issue.ID] = true
			}
			for _, checks := range []progressionChecks{
				{classes: true}, {feats: true}, {feats: true, featID: "scholar"}, {feats: true, featID: "veteran"},
			} {
				want := []character.Issue{}
				for _, issue := range full.Issues {
					id := strings.TrimPrefix(issue.ID, "waived:")
					if checks.classes && strings.HasPrefix(id, "multiclass:") ||
						checks.feats && strings.HasPrefix(id, "feat:") && (checks.featID == "" || id == "feat:"+checks.featID) {
						want = append(want, issue)
					}
				}
				got := character.Result{Issues: []character.Issue{}}
				validateSelectedProgression(input, newCharacterRecordSnapshot(records), profile, &got, checks)
				if !reflect.DeepEqual(got.Issues, want) {
					t.Fatalf("selected %+v changed issues: got %+v, want %+v", checks, got.Issues, want)
				}
			}
			if !reflect.DeepEqual(input, original) || string(mustJSON(records.byKind)) != sources {
				t.Fatal("progression checks mutated authored input or source records")
			}
		})
	}
	for _, id := range []string{"feat:scholar", "feat:veteran", "waived:feat:veteran", "multiclass:mage-entry:mage"} {
		if !seen[id] {
			t.Fatal("fixture did not exercise", id)
		}
	}
}

func TestEmptyPrerequisitesStillEnforceFeatRepetition(t *testing.T) {
	for _, prerequisite := range []any{nil, Object{}} {
		input, records, profile := progressionChoiceInput(t)
		feat := Object{"kind": "feat", "id": "training", "category": "general", "prerequisites": prerequisite}
		records.byKind["feat"]["training"] = mustJSON(feat)
		input.Build.Choices = []character.Choice{
			{ID: "asi:fighter:4", Value: mustJSON("feat")}, {ID: "asi:fighter:4:feat", Value: mustJSON("training")},
			{ID: "asi:fighter:8", Value: mustJSON("feat")}, {ID: "asi:fighter:8:feat", Value: mustJSON("training")},
		}
		if !hasFeatBlock(EvaluateCharacter(input, records, profile), "training") {
			t.Fatal("empty prerequisites permitted duplicate acquisition")
		}
		feat["repeatable"] = true
		records.byKind["feat"]["training"] = mustJSON(feat)
		if hasFeatBlock(EvaluateCharacter(input, records, profile), "training") {
			t.Fatal("source-authorized repetition was blocked")
		}
	}
}
