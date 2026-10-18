package rules

import (
	"encoding/json"
	"testing"

	"github.com/pjunak/addon-dnd-engine/character"
)

func TestConcentrationFollowsCastsDamageAndCommands(t *testing.T) {
	input, records, profile := repeatedSpellFixture(t)
	records.byKind["spell"]["shared"] = mustJSON(Object{"kind": "spell", "id": "shared", "name": "Shared Spell", "level": 1, "classes": []any{"alpha", "beta", "gamma"}, "concentration": true})
	profile.Constants.Concentration = &ConcentrationPolicy{SaveAbility: "CON", MinimumDC: 10, DamageDivisor: 2, MaximumDC: 30}
	profile.Constants.Character.ConditionRules = "table-states"
	records.byKind["rule"] = map[string]json.RawMessage{"table-states": mustJSON(Object{"id": "table-states", "kind": "rule", "name": "Synthetic condition rules", "book": "test", "conditionDefinitions": []conditionDefinition{
		{ID: "dazed", Name: "Dazed", MaximumLevel: 1},
		{ID: "out", Name: "Out Cold", MaximumLevel: 1, EndsConcentration: true},
	}})}
	result := EvaluateCharacter(input, records, profile)
	if !result.Ready || result.Sheet["concentration"] != nil {
		t.Fatal("concentration without a cast", result.Issues, result.Sheet["concentration"])
	}
	grant := objects(result.SpellOptions["granted"])[0]
	result, err := ApplyCharacterPlay(result.Inputs, Object{"operation": "cast-granted-spell", "key": grant["key"], "slot": grantFreeResourceKey(grant)}, records, profile)
	if err != nil || result.Inputs.Play.Concentration == nil || result.Inputs.Play.Concentration.Ref != "shared" {
		t.Fatal("concentration cast was not saved", err, result.Inputs.Play.Concentration)
	}
	row := object(result.Sheet["concentration"])
	if row["name"] != "Shared Spell" || row["saveAbility"] != "CON" || row["saveBonus"] != integer(object(object(result.Sheet["saves"])["CON"])["total"], 0) || row["saveDC"] != nil {
		t.Fatal("projection", row)
	}
	for damage, dc := range map[float64]int{3: 10, 25: 12, 90: 30} {
		hurt, err := ApplyCharacterPlay(result.Inputs, Object{"operation": "damage", "amount": damage}, records, profile)
		if err != nil || hurt.Inputs.Play.Concentration.SaveDC != dc || object(hurt.Sheet["concentration"])["saveDC"] != dc {
			t.Fatal("save DC after damage", damage, err, hurt.Inputs.Play.Concentration)
		}
	}
	hurt, _ := ApplyCharacterPlay(result.Inputs, Object{"operation": "damage", "amount": float64(25)}, records, profile)
	kept, err := ApplyCharacterPlay(hurt.Inputs, Object{"operation": "keep-concentration"}, records, profile)
	if err != nil || kept.Inputs.Play.Concentration == nil || kept.Inputs.Play.Concentration.SaveDC != 0 || kept.Inputs.Play.Concentration.Ref != "shared" {
		t.Fatal("keeping concentration", err, kept.Inputs.Play.Concentration)
	}
	healed, _ := ApplyCharacterPlay(kept.Inputs, Object{"operation": "heal", "amount": float64(0)}, records, profile)
	rested, _ := ApplyCharacterPlay(healed.Inputs, Object{"operation": "rest", "rest": "long"}, records, profile)
	if rested.Inputs.Play.Concentration == nil || rested.Inputs.Play.Concentration.SaveDC != 0 {
		t.Fatal("healing or rest changed concentration", rested.Inputs.Play.Concentration)
	}
	out := cloneCharacter(kept.Inputs)
	out.Play.Conditions = []character.Condition{{ID: "dazed", Level: 1}, {ID: "out", Level: 1}}
	if ended := EvaluateCharacter(out, records, profile); object(ended.Sheet["concentration"])["endedBy"] != "Out Cold" || ended.Inputs.Play.Concentration == nil {
		t.Fatal("ending condition", ended.Sheet["concentration"])
	}
	ended, err := ApplyCharacterPlay(kept.Inputs, Object{"operation": "end-concentration"}, records, profile)
	if err != nil || ended.Inputs.Play.Concentration != nil || ended.Sheet["concentration"] != nil {
		t.Fatal("ending concentration", err)
	}
	for _, change := range []Object{{"operation": "end-concentration"}, {"operation": "keep-concentration"}} {
		if _, err := ApplyCharacterPlay(ended.Inputs, change, records, profile); err == nil {
			t.Fatal("command without concentration accepted", change)
		}
	}
	if _, err := ApplyCharacterPlay(kept.Inputs, Object{"operation": "end-concentration", "ref": "shared"}, records, profile); err == nil {
		t.Fatal("extra field accepted")
	}
	profile.Constants.Concentration = nil
	if hurt, _ := ApplyCharacterPlay(kept.Inputs, Object{"operation": "damage", "amount": float64(25)}, records, profile); hurt.Inputs.Play.Concentration.SaveDC != 0 {
		t.Fatal("save DC without a ruleset policy")
	}
}

func TestConcentrationCastReplacesOnlyForConcentrationSpells(t *testing.T) {
	input, records, profile := repeatedSpellFixture(t)
	input.Play.Concentration = &character.Concentration{Ref: "alternate", SaveDC: 11}
	result := EvaluateCharacter(input, records, profile)
	grant := objects(result.SpellOptions["granted"])[0]
	cast, err := ApplyCharacterPlay(result.Inputs, Object{"operation": "cast-granted-spell", "key": grant["key"], "slot": grantFreeResourceKey(grant)}, records, profile)
	if err != nil || *cast.Inputs.Play.Concentration != (character.Concentration{Ref: "alternate", SaveDC: 11}) {
		t.Fatal("a spell without Concentration replaced it", err, cast.Inputs.Play.Concentration)
	}
	records.byKind["spell"]["shared"] = mustJSON(Object{"kind": "spell", "id": "shared", "name": "Shared Spell", "level": 1, "classes": []any{"alpha", "beta", "gamma"}, "concentration": true})
	cast, err = ApplyCharacterPlay(result.Inputs, Object{"operation": "cast-granted-spell", "key": grant["key"], "slot": grantFreeResourceKey(grant)}, records, profile)
	if err != nil || *cast.Inputs.Play.Concentration != (character.Concentration{Ref: "shared"}) {
		t.Fatal("a new Concentration spell did not replace the old one", err, cast.Inputs.Play.Concentration)
	}
	input.Play.Concentration = &character.Concentration{Ref: "alternate", SaveDC: -1}
	if invalid := EvaluateCharacter(input, records, profile); invalid.Ready {
		t.Fatal("negative save DC accepted")
	}
	if ended, err := ApplyCharacterPlay(input, Object{"operation": "end-concentration"}, records, profile); err != nil || ended.Inputs.Play.Concentration != nil || !ended.Ready {
		t.Fatal("invalid concentration could not be ended", err)
	}
	input.Play.Concentration = &character.Concentration{Ref: "missing"}
	missing := EvaluateCharacter(input, records, profile)
	if !missing.Ready || object(missing.Sheet["concentration"])["name"] != "missing" {
		t.Fatal("unavailable spell", missing.Issues)
	}
}

func TestRulesetRejectsInvalidConcentration(t *testing.T) {
	t.Parallel()
	for _, policy := range []ConcentrationPolicy{
		{SaveAbility: "LUCK", MinimumDC: 10, DamageDivisor: 2, MaximumDC: 30},
		{SaveAbility: "CON", MinimumDC: 0, DamageDivisor: 2, MaximumDC: 30},
		{SaveAbility: "CON", MinimumDC: 10, DamageDivisor: 0, MaximumDC: 30},
		{SaveAbility: "CON", MinimumDC: 10, DamageDivisor: 2, MaximumDC: 9},
	} {
		ruleset := syntheticRuleset(t)
		ruleset.Constants.Concentration = &policy
		if ruleset.Validate() == nil {
			t.Errorf("accepted %+v", policy)
		}
	}
}
