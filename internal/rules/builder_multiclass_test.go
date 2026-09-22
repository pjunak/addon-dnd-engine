package rules

import (
	"reflect"
	"testing"
)

func TestMulticlassSkillsUseOnlyTheDeclaredReducedPool(t *testing.T) {
	for _, test := range []struct {
		name    string
		reduced Object
		count   int
	}{
		{"missing", nil, 0}, {"empty", Object{}, 0}, {"other-proficiencies", Object{"armor": []any{"light"}}, 0},
		{"one-skill", Object{"skills": Object{"choose": 1, "from": []any{"arcana", "history"}}}, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			records, profile := syntheticRecords(), syntheticRuleset(t)
			wizard := recordByID(records, "class", "wizard")
			wizard["startingProficiencies"] = Object{"skills": Object{"choose": 2, "from": []any{"arcana", "history", "nature"}}}
			wizard["multiclassProficiencies"] = test.reduced
			records.byKind["class"]["wizard"] = mustJSON(wizard)
			before := mustJSON(records.byKind)
			for _, initial := range []bool{false, true} {
				classes := []any{Object{"classId": "fighter", "level": 1}, Object{"classId": "wizard", "level": 2}}
				want := test.count
				if initial {
					classes[0], classes[1] = classes[1], classes[0]
					want = 2
				}
				plan := BuilderPlan(Object{"classes": classes}, records, profile)
				found := 0
				for _, choice := range objects(plan["classChoices"]) {
					if text(choice["id"]) == "skills:wizard" {
						found = integer(choice["count"], 0)
						if !initial && !reflect.DeepEqual(choice["from"], []any{"arcana", "history"}) {
							t.Fatal("lost reduced pool", choice)
						}
					}
				}
				if found != want {
					t.Fatalf("initial=%v: skill count %d, want %d", initial, found, want)
				}
			}
			if string(before) != string(mustJSON(records.byKind)) {
				t.Fatal("mutated class source")
			}
		})
	}
}
