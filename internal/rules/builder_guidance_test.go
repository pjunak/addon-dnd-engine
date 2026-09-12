package rules

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestBuilderGuidanceLabelsOptionsAndCountsValidChoices(t *testing.T) {
	records, profile := syntheticRecords(), syntheticRuleset(t)
	state := Object{"classes": []any{Object{"classId": "wizard", "level": 5}}, "baseStats": Object{"STR": 8, "DEX": 15, "CON": 15, "INT": 15, "WIS": 8, "CHA": 8}, "species": "Dwarf", "background": "Acolyte", "featureChoices": Object{"study#0": "wizard-scholar", "study#1": "wizard-scholar"}}
	plan := BuilderPlan(state, records, profile)
	plan["classChoices"] = append(values(plan["classChoices"]), Object{"id": "study", "kind": "enumerated", "classId": "wizard", "count": 2, "from": []any{"wizard-scholar", "wizard-arcane-recovery"}, "source": Object{"level": 2}})
	before, _ := json.Marshal(state)
	result := BuilderGuidance(state, plan, records, profile)
	choice := object(object(result["choices"])["study"])
	if integer(choice["picked"], 0) != 1 || truth(choice["done"]) || text(objects(choice["options"])[0]["label"]) != "Arcane Recovery" {
		t.Fatalf("choice = %+v", choice)
	}
	foundation := objects(result["sections"])[0]
	for _, issue := range objects(foundation["issues"]) {
		if text(issue["id"]) == "abilities" {
			t.Fatal("Complete point buy was reported incomplete")
		}
	}
	after, _ := json.Marshal(state)
	if string(before) != string(after) {
		t.Fatal("Read-only guidance changed authored decisions")
	}
	repeated := BuilderGuidance(state, plan, records, profile)
	if !reflect.DeepEqual(result, repeated) {
		t.Fatal("Guidance is nondeterministic")
	}
	mastery := builderChoiceOptions(Object{"kind": "weaponMastery"}, Object{}, records)
	if len(mastery) != 1 || text(object(mastery[0])["label"]) != "Longsword" {
		t.Fatalf("Mastery options = %+v", mastery)
	}
	expertise := builderChoiceOptions(Object{"kind": "expertise"}, Object{"skills": Object{"arcana": Object{"proficient": true}}}, records)
	if len(expertise) != 1 || text(object(expertise[0])["id"]) != "arcana" {
		t.Fatalf("Expertise = %+v", expertise)
	}
}

func TestBuilderGuidanceRequiresAllocatedFeatAbilities(t *testing.T) {
	records, profile := syntheticRecords(), syntheticRuleset(t)
	state := Object{"classes": []any{Object{"classId": "fighter", "level": 19}}, "featureChoices": Object{"asi:fighter:19": "feat", "asi:fighter:19:feat": "boon-of-fortitude"}}
	plan := BuilderPlan(state, records, profile)
	result := BuilderGuidance(state, plan, records, profile)
	choice := object(object(result["choices"])["asi:fighter:19"])
	if truth(choice["done"]) {
		t.Fatal("Unallocated feat ability was marked complete")
	}
	state = ApplyBuilderChoice(state, Object{"choiceId": "asi:fighter:19:featability", "value": Object{"ability": "CON", "amount": 1}}, records, profile)
	result = BuilderGuidance(state, BuilderPlan(state, records, profile), records, profile)
	if !truth(object(object(result["choices"])["asi:fighter:19"])["done"]) {
		t.Fatal("Allocated feat remains incomplete")
	}
}

func TestPlayDoesNotPromoteDerivedFeatsToManualFeats(t *testing.T) {
	records, profile := syntheticRecords(), syntheticRuleset(t)
	for _, manual := range []any{nil, []any{Object{"featId": "adaptable", "note": "authored"}}} {
		state := Object{"classes": []any{Object{"classId": "wizard", "level": 5}}, "extraFeats": []any{Object{"id": "reward", "featId": "guardian", "sourceNote": "earned"}}}
		if manual != nil {
			state["feats"] = manual
		}
		next, err := applyPlayFixture(state, Object{"operation": "rest", "rest": "short"}, records, profile)
		if err != nil {
			t.Fatal(err)
		}
		actual, _ := json.Marshal(next["feats"])
		expected, _ := json.Marshal(manual)
		if string(actual) != string(expected) {
			t.Fatalf("Authored feats changed: %+v", next["feats"])
		}
		next["extraFeats"] = []any{}
		normalized := NormalizeBuilderDecisions(next, records, profile)
		if contains(selectedFeatIDs(normalized, records), "guardian") {
			t.Fatal("Removed reward survived through calculated feat cache")
		}
	}
}

func TestBuilderSkillSpellingsCalculateWithoutRewritingAuthoredKeys(t *testing.T) {
	records, profile := syntheticRecords(), syntheticRuleset(t)
	state := Object{"level": 5, "abilities": Object{"WIS": 12, "DEX": 14}, "skillProficiencies": []any{"animal-handling", "sleight of hand"}, "skillExpertise": Object{"animal_handling": true}}
	before, _ := json.Marshal(state)
	sheet := Hydrate(state, records, &profile).Sheet
	animal := object(object(sheet["skills"])["animalHandling"])
	if !truth(animal["expertise"]) || integer(animal["total"], 0) != 7 || !truth(object(object(sheet["skills"])["sleightOfHand"])["proficient"]) {
		t.Fatalf("Skills = %+v", sheet["skills"])
	}
	options := builderChoiceOptions(Object{"kind": "expertise", "from": []any{"animal-handling", "athletics"}}, sheet, records)
	if len(options) != 1 || text(object(options[0])["id"]) != "animal-handling" {
		t.Fatalf("Options = %+v", options)
	}
	after, _ := json.Marshal(state)
	if string(before) != string(after) {
		t.Fatal("Skill calculation changed input")
	}
}
