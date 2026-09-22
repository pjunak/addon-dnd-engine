package rules

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/pjunak/addon-dnd-engine/character"
)

func repeatedSpellFixture(t *testing.T) (character.Inputs, memoryRecords, Ruleset) {
	t.Helper()
	input, records, profile := repeatableFixture(t)
	input.Build.Choices = input.Build.Choices[:4]
	options := Object{}
	for _, list := range []string{"alpha", "beta", "gamma"} {
		options[list] = Object{"spells": []any{Object{"id": "spell", "choose": 1, "spellLevel": 1, "from": Object{"class": []any{list}}, "alwaysPrepared": true, "free": "1/long"}}}
		input.Build.Choices = append(input.Build.Choices, character.Choice{ID: strings.TrimSuffix(map[string]string{"alpha": trainingOrigin, "beta": trainingFour, "gamma": trainingEight}[list], "proficiencies") + "list", Value: mustJSON(list)})
	}
	records.byKind["feat"]["training"] = mustJSON(Object{"kind": "feat", "id": "training", "name": "Spell Training", "category": "general", "repeatable": Object{"by": "choice", "choice": "list"}, "prerequisites": Object{}, "grants": Object{
		"choices":        []any{Object{"id": "list", "type": "enumerated", "count": 1, "from": []any{"alpha", "beta", "gamma"}}},
		"choicePackages": []any{Object{"choiceId": "list", "options": options}},
		"castingAbility": Object{"id": "casting", "choose": []any{"INT", "WIS", "CHA"}},
		"resources":      []any{Object{"key": "focus", "fixed": 2, "recharge": "long"}},
		"activations":    []any{Object{"id": "focus", "resource": Object{"key": "focus", "cost": 1}, "modifiers": []any{}}},
	}})
	records.byKind["spell"] = map[string]json.RawMessage{}
	records.byKind["spell"]["shared"] = mustJSON(Object{"kind": "spell", "id": "shared", "name": "Shared Spell", "level": 1, "classes": []any{"alpha", "beta", "gamma"}})
	records.byKind["spell"]["alternate"] = mustJSON(Object{"kind": "spell", "id": "alternate", "level": 1, "classes": []any{"alpha", "beta", "gamma"}})
	records.byKind["spell"]["alpha-only"] = mustJSON(Object{"kind": "spell", "id": "alpha-only", "level": 1, "classes": []any{"alpha"}})
	input.Build.Spells.GrantChoices = map[string][]string{}
	input.Build.Spells.CastingAbilities = map[string]string{}
	for i, key := range []string{trainingOrigin, trainingFour, trainingEight} {
		owner := strings.TrimSuffix(key, "proficiencies")
		input.Build.Spells.GrantChoices[owner+"spell"] = []string{"shared"}
		input.Build.Spells.CastingAbilities[owner+"casting"] = []string{"INT", "WIS", "CHA"}[i]
	}
	return input, records, profile
}

func TestRepeatedSpellGrantsSpendAndRecoverIndependently(t *testing.T) {
	input, records, profile := repeatedSpellFixture(t)
	before, sources := cloneCharacter(input), mustJSON(records.byKind)
	result := EvaluateCharacter(input, records, profile)
	if !result.Ready {
		t.Fatal(result.Issues)
	}
	grants := objects(result.SpellOptions["granted"])
	if len(grants) != 3 {
		t.Fatal("collapsed grants", grants)
	}
	keys := []string{}
	for _, grant := range grants {
		source := object(grant["source"])
		owner := grantOwner(source)
		ability := input.Build.Spells.CastingAbilities[owner+":casting"]
		if text(grant["castingAbility"]) != ability {
			t.Fatal("shared casting ability", grant)
		}
		key := grantFreeResourceKey(grant)
		keys = append(keys, key)
		if !contains(stringsOf(grant["slots"]), key) {
			t.Fatal("free cast missing", grant)
		}
	}
	if len(unique(keys)) != 3 {
		t.Fatal("shared resource keys", keys)
	}
	for _, grant := range grants {
		key := grantFreeResourceKey(grant)
		var err error
		result, err = ApplyCharacterPlay(result.Inputs, Object{"operation": "cast-granted-spell", "key": grant["key"], "slot": key}, records, profile)
		if err != nil || result.Inputs.Play.ResourceUses[key] != 1 {
			t.Fatal("independent cast failed", err, result.Issues)
		}
		if _, err = ApplyCharacterPlay(result.Inputs, Object{"operation": "cast-granted-spell", "key": grant["key"], "slot": key}, records, profile); err == nil {
			t.Fatal("exhausted cast accepted")
		}
	}
	if _, err := ApplyCharacterPlay(input, Object{"operation": "cast-granted-spell", "key": grants[0]["key"], "slot": keys[1]}, records, profile); err == nil {
		t.Fatal("cast consumed another owner's free use")
	}
	rested, err := ApplyCharacterPlay(result.Inputs, Object{"operation": "rest", "rest": "short"}, records, profile)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range keys {
		if rested.Inputs.Play.ResourceUses[key] != 1 {
			t.Fatal("short rest refreshed long-rest grant")
		}
	}
	rested, err = ApplyCharacterPlay(rested.Inputs, Object{"operation": "rest", "rest": "long"}, records, profile)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range keys {
		if rested.Inputs.Play.ResourceUses[key] != 0 {
			t.Fatal("long rest failed", key)
		}
	}
	// Replacing the chosen spell retains this acquisition's spent allowance.
	owner := grantOwner(object(grants[0]["source"]))
	result.Inputs.Build.Spells.GrantChoices[owner+":spell"] = []string{"alternate"}
	changed := EvaluateCharacter(result.Inputs, records, profile)
	if changed.Inputs.Play.ResourceUses[keys[0]] != 1 || !changed.Ready {
		t.Fatal("spell replacement refreshed or invalidated uses", changed.Issues)
	}
	if !reflect.DeepEqual(input, before) || string(mustJSON(records.byKind)) != string(sources) {
		t.Fatal("mutated authored input or provider records")
	}
}

func TestRepeatedSpellListAndOwnerValidation(t *testing.T) {
	input, records, profile := repeatedSpellFixture(t)
	owner := strings.TrimSuffix(trainingFour, "proficiencies")
	input.Build.Spells.GrantChoices[owner+"spell"] = []string{"alpha-only"}
	result := EvaluateCharacter(input, records, profile)
	if !hasChoiceIssue(result, "spell-grant:"+owner+"spell:alpha-only") {
		t.Fatal("cross-list spell accepted", result.Issues)
	}
	input, records, profile = repeatedSpellFixture(t)
	input.Build.Background = "other"
	result = EvaluateCharacter(input, records, profile)
	if len(objects(result.SpellOptions["granted"])) != 2 || !hasChoiceIssue(result, "spell-grant:"+strings.TrimSuffix(trainingOrigin, "proficiencies")+"spell") {
		t.Fatal("withdrawal transferred a grant", result.Issues)
	}
	for _, row := range objects(result.SpellOptions["granted"]) {
		if !strings.Contains(text(row["key"]), "asi%3A") {
			t.Fatal("owner changed", row)
		}
	}
}

func TestSpellGrantLegacyAliasesRequireOneOwner(t *testing.T) {
	input, records, profile := repeatedSpellFixture(t)
	input.Build.Spells.GrantChoices = map[string][]string{"feat:training:spell": {"shared"}}
	input.Build.Spells.CastingAbilities = map[string]string{"feat:training:casting": "INT"}
	input.Play.ResourceUses = map[string]int{"charge-shared": 1}
	result := EvaluateCharacter(input, records, profile)
	if !reflect.DeepEqual(result.Inputs.Build.Spells, input.Build.Spells) || result.Ready {
		t.Fatal("ambiguous choices silently assigned")
	}
	input.Build.Levels = input.Build.Levels[:1]
	input.Build.Choices = input.Build.Choices[4:5]
	result = EvaluateCharacter(input, records, profile)
	owner := strings.TrimSuffix(trainingOrigin, "proficiencies")
	if !result.Ready || result.Inputs.Build.Spells.CastingAbilities[owner+"casting"] != "INT" || result.Inputs.Play.ResourceUses["charge:"+owner+"spell"] != 1 {
		t.Fatal("unambiguous state lost", result.Issues, result.Inputs)
	}
	if _, err := ApplyCharacterPlay(input, Object{"operation": "cast-granted-spell", "key": strings.TrimSuffix(owner, ":") + ":shared", "slot": "charge:" + owner + "spell"}, records, profile); err == nil {
		t.Fatal("normalized legacy expenditure ignored during play")
	}
}

func TestOriginFeatPresetIsRestrictedAndFeedsSpellPackage(t *testing.T) {
	input, records, profile := repeatedSpellFixture(t)
	record := recordByID(records, "background", "artisan")
	record["originFeatChoices"] = Object{"list": "alpha"}
	records.byKind["background"]["artisan"] = mustJSON(record)
	input.Build.Choices = append(input.Build.Choices[:4], input.Build.Choices[5:]...)
	result := EvaluateCharacter(input, records, profile)
	if !result.Ready || len(objects(result.SpellOptions["granted"])) != 3 {
		t.Fatal("origin preset failed", result.Issues)
	}
	input.Build.Choices = append(input.Build.Choices, character.Choice{ID: strings.TrimSuffix(trainingOrigin, "proficiencies") + "list", Value: mustJSON("beta")})
	result = EvaluateCharacter(input, records, profile)
	if result.Ready {
		t.Fatal("overrode the origin's fixed list")
	}
}
func TestRepeatedFeatPoolsAndActivationsStayWithTheirOwner(t *testing.T) {
	input, records, profile := repeatedSpellFixture(t)
	result := EvaluateCharacter(input, records, profile)
	activations := objects(result.Sheet["activations"])
	if len(activations) != 3 {
		t.Fatal(activations)
	}
	for _, activation := range activations {
		key := text(activation["key"])
		if text(object(activation["resource"])["key"]) != key || findPlayResource(Object(result.Sheet), key) == nil {
			t.Fatal("pool and activation ownership differ", activation)
		}
		var err error
		result, err = ApplyCharacterPlay(result.Inputs, Object{"operation": "toggle-feature", "key": key, "enabled": true}, records, profile)
		if err != nil || result.Inputs.Play.ResourceUses[key] != 1 {
			t.Fatal("activation did not spend its own pool", err, result.Inputs.Play.ResourceUses)
		}
	}
	for _, activation := range activations {
		if !result.Inputs.Play.ActiveFeatures[text(activation["key"])] {
			t.Fatal("activation displaced its sibling")
		}
	}
}
func TestClearedSpellChoiceRetainsItsSpentAllowance(t *testing.T) {
	input, records, profile := repeatedSpellFixture(t)
	key := "charge:" + strings.TrimSuffix(trainingOrigin, "proficiencies") + "spell"
	input.Play.ResourceUses = map[string]int{key: 1}
	input.Build.Spells.GrantChoices[strings.TrimSuffix(trainingOrigin, "proficiencies")+"spell"] = []string{}
	result := EvaluateCharacter(input, records, profile)
	resource := findPlayResource(Object(result.Sheet), key)
	if resource == nil || integer(resource["remaining"], -1) != 0 || hasChoiceIssue(result, "resource:"+key) {
		t.Fatal("clearing a spell lost its spent allowance", result.Issues, resource)
	}
	input.Build.Spells.GrantChoices[strings.TrimSuffix(trainingOrigin, "proficiencies")+"spell"] = []string{"alternate"}
	result = EvaluateCharacter(input, records, profile)
	if !result.Ready || result.Inputs.Play.ResourceUses[key] != 1 {
		t.Fatal("replacement refreshed allowance", result.Issues)
	}
}
func TestRepeatedFeatBonusSlotsAreIndependentAndUsable(t *testing.T) {
	input, records, profile := repeatedSpellFixture(t)
	feat := recordByID(records, "feat", "training")
	object(feat["grants"])["spellSlot"] = Object{"count": 1, "level": Object{"min": 1, "max": 1}, "recharge": "long"}
	records.byKind["feat"]["training"] = mustJSON(feat)
	result := EvaluateCharacter(input, records, profile)
	slots := []string{}
	for _, resource := range objects(result.Sheet["resources"]) {
		if text(resource["legacyKey"]) == "feat-slot-training" {
			slots = append(slots, text(resource["key"]))
		}
	}
	if len(unique(slots)) != 3 {
		t.Fatal("bonus slots collapsed", slots)
	}
	grant := objects(result.SpellOptions["granted"])[0]
	for _, slot := range slots {
		if !contains(stringsOf(grant["slots"]), slot) {
			t.Fatal("scoped slot unavailable", slot)
		}
		var err error
		result, err = ApplyCharacterPlay(result.Inputs, Object{"operation": "cast-granted-spell", "key": grant["key"], "slot": slot}, records, profile)
		if err != nil || result.Inputs.Play.ResourceUses[slot] != 1 {
			t.Fatal("cannot spend scoped bonus slot", err)
		}
	}
}
