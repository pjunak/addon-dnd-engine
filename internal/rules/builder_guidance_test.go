package rules

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/pjunak/addon-dnd-engine/character"
)

func TestBuilderGuidanceLabelsOptionsAndCountsValidChoices(t *testing.T) {
	records, profile := syntheticRecords(), syntheticRuleset(t)
	state := Object{"classes": []any{Object{"classId": "wizard", "level": 5}}, "baseStats": Object{"STR": 8, "DEX": 15, "CON": 15, "INT": 15, "WIS": 8, "CHA": 8}, "species": "dwarf", "background": "acolyte", "featureChoices": Object{"study#0": "wizard-scholar", "study#1": "wizard-scholar"}}
	plan := BuilderPlan(state, records, profile)
	plan.ClassChoices = append(plan.ClassChoices, decodeAs[character.PlanChoice](Object{"id": "study", "kind": "enumerated", "classId": "wizard", "count": 2, "from": []any{"wizard-scholar", "wizard-arcane-recovery"}, "source": Object{"level": 2}}))
	before, _ := json.Marshal(state)
	result := BuilderGuidance(state, plan, records, profile)
	choice := object(object(view(result)["choices"])["study"])
	if integer(choice["picked"], 0) != 1 || truth(choice["done"]) || text(objects(choice["options"])[0]["label"]) != "Arcane Recovery" {
		t.Fatalf("choice = %+v", choice)
	}
	foundation := objects(view(result)["sections"])[0]
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
	mastery := builderChoiceOptions(character.PlanChoice{Kind: character.ChoiceWeaponMastery}, &character.Calculation{}, records)
	if len(mastery) != 1 || mastery[0].Label != "Longsword" {
		t.Fatalf("Mastery options = %+v", mastery)
	}
	expertise := builderChoiceOptions(character.PlanChoice{Kind: character.ChoiceExpertise}, &character.Calculation{Skills: map[string]character.SkillRow{"arcana": {Proficient: true}}}, records)
	if len(expertise) != 1 || expertise[0].ID != "arcana" {
		t.Fatalf("Expertise = %+v", expertise)
	}
}

func TestBuilderGuidanceRequiresAllocatedFeatAbilities(t *testing.T) {
	records, profile := syntheticRecords(), syntheticRuleset(t)
	state := Object{"classes": []any{Object{"classId": "fighter", "level": 19}}, "featureChoices": Object{"asi:fighter:19": "feat", "asi:fighter:19:feat": "boon-of-fortitude"}}
	plan := BuilderPlan(state, records, profile)
	result := BuilderGuidance(state, plan, records, profile)
	if result.Choices["asi:fighter:19"].Done {
		t.Fatal("Unallocated feat ability was marked complete")
	}
	state = ApplyBuilderChoice(state, Object{"choiceId": "asi:fighter:19:featability", "value": Object{"ability": "CON", "amount": 1}}, records, profile)
	result = BuilderGuidance(state, BuilderPlan(state, records, profile), records, profile)
	if !result.Choices["asi:fighter:19"].Done {
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
		if findFeat(values(normalized["feats"]), "guardian") != nil {
			t.Fatal("Removed reward survived through calculated feat cache")
		}
	}
}

func TestBuilderSkillSpellingsCalculateWithoutRewritingAuthoredKeys(t *testing.T) {
	records, profile := syntheticRecords(), syntheticRuleset(t)
	state := Object{"classes": []any{Object{"classId": "fighter", "level": 5}}, "baseStats": Object{"WIS": 12, "DEX": 14}, "skillProficiencies": []any{"animal-handling", "sleight of hand"}, "skillExpertise": Object{"animal_handling": true}}
	before, _ := json.Marshal(state)
	sheet := Hydrate(state, records, profile).Sheet
	animal := sheet.Skills["animalHandling"]
	if !animal.Expertise || animal.Total != 7 || !sheet.Skills["sleightOfHand"].Proficient {
		t.Fatalf("Skills = %+v", sheet.Skills)
	}
	options := builderChoiceOptions(character.PlanChoice{Kind: character.ChoiceExpertise, From: []string{"animal-handling", "athletics"}}, sheet, records)
	if len(options) != 1 || options[0].ID != "animal-handling" {
		t.Fatalf("Options = %+v", options)
	}
	after, _ := json.Marshal(state)
	if string(before) != string(after) {
		t.Fatal("Skill calculation changed input")
	}
}

func TestBuilderGuidanceIncludesRequiredClassSpellSelections(t *testing.T) {
	records, profile := syntheticRecords(), syntheticRuleset(t)
	state := Object{"classes": []any{Object{"classId": "wizard", "level": 5}}}
	result := BuilderGuidance(state, BuilderPlan(state, records, profile), records, profile)
	targets := map[string]Object{}
	for _, issue := range objects(objects(view(result)["sections"])[2]["issues"]) {
		targets[text(issue["id"])] = issue
	}
	for _, id := range []string{"cantrips:wizard", "spellbook:wizard"} {
		issue := targets[id]
		if issue == nil || text(issue["tab"]) != "spells" || len(values(issue["labelArgs"])) != 1 {
			t.Fatal("required class spells have no translated navigation target", id, result)
		}
	}
}

func TestBuilderChoiceHeadingsNameWhatIsChosenAndItsSource(t *testing.T) {
	records := newMemoryRecords([]Object{
		{"kind": "class", "id": "ranger", "name": "Ranger"},
		{"kind": "species", "id": "elf", "name": "Elf"},
		{"kind": "feature", "id": "ranger-weapon-mastery", "name": "Weapon Mastery"},
		{"kind": "class", "id": "hb.knight", "name": "Sworn Knight"},
	})
	for _, test := range []struct {
		choice Object
		label  string
	}{
		{Object{"id": "skills:ranger", "kind": "skills", "source": Object{"type": "class", "id": "ranger"}}, "Skills · Ranger"},
		{Object{"id": "species:elf:elf-keen-senses", "kind": "skills", "source": Object{"type": "species", "id": "elf"}}, "Skills · Elf"},
		{Object{"id": "species:elf:elf-lineage-spell", "kind": "enumerated", "source": Object{"type": "species", "id": "elf"}}, "Lineage Spell · Elf"},
		{Object{"id": "species:elf:size", "kind": "size", "prompt": "Size", "source": Object{"type": "species", "id": "elf"}}, "Size"},
		{Object{"id": "study", "kind": "enumerated"}, "Study"},
		{Object{"id": "ranger-weapon-mastery-weapons", "kind": "weaponMastery", "classId": "ranger", "source": Object{"type": "feature", "id": "ranger-weapon-mastery"}}, "Weapon Mastery · Ranger"},
		{Object{"id": "asi:hb.knight:4", "kind": "asiMode", "classId": "hb.knight", "level": 4, "feat": Object{"id": "asi:hb.knight:4:feat"}}, "Sworn Knight level 4 advancement"},
	} {
		entry := builderChoiceGuidance(Object{}, decodeAs[character.PlanChoice](test.choice), &character.Calculation{}, records, []character.Option{})
		if entry.Label != test.label {
			t.Errorf("%s heading = %q, want %q", text(test.choice["id"]), entry.Label, test.label)
		}
	}
}

// Provider IDs need not read as names: homebrew records are "hb.<id>".
func TestGrantGuidanceNamesSourcesByTheirRecords(t *testing.T) {
	records, profile := syntheticRecords(), syntheticRuleset(t)
	records.byKind["species"]["hb.emberkin"] = mustJSON(Object{"kind": "species", "id": "hb.emberkin", "name": "Emberkin", "lineages": []any{Object{"id": "ember", "grants": Object{
		"castingAbility": Object{"id": "ember-casting", "choose": []any{"INT", "WIS", "CHA"}},
		"spells":         []any{Object{"ids": []any{"fire-bolt"}}},
	}}}})
	records.byKind["feat"]["hb.fire-touched"] = mustJSON(Object{"kind": "feat", "id": "hb.fire-touched", "name": "Fire-Touched", "grants": Object{
		"castingAbility": Object{"choose": []any{"INT", "CHA"}},
		"spells":         []any{Object{"id": "touched-spell", "choose": 1, "spellLevel": 1, "from": Object{"school": []any{"Abjuration"}}, "free": "1/long"}},
	}})
	state := Object{"classes": []any{Object{"classId": "wizard", "level": 1}}, "species": "hb.emberkin", "lineage": "ember", "feats": []any{Object{"featId": "hb.fire-touched"}}}
	labels := map[string]string{}
	for _, issue := range objects(objects(view(BuilderGuidance(state, BuilderPlan(state, records, profile), records, profile))["sections"])[2]["issues"]) {
		labels[text(issue["id"])] = text(issue["label"])
	}
	for key, want := range map[string]string{
		"species:hb.emberkin:ember-casting":    "Choose casting ability: Emberkin",
		"feat:hb.fire-touched:casting-ability": "Choose casting ability: Fire-Touched",
		"feat:hb.fire-touched:touched-spell":   "Choose spells: Fire-Touched",
	} {
		if labels[key] != want {
			t.Errorf("%s label = %q, want %q", key, labels[key], want)
		}
	}
	found := false
	for _, resource := range Hydrate(NormalizeBuilderDecisions(state, records, profile), records, profile).Sheet.Resources {
		found = found || resource.Key == "charge:feat:hb.fire-touched:touched-spell" && resource.Name == "Fire-Touched (free cast)"
	}
	if !found {
		t.Fatal("free cast is not named by its feat")
	}
}
