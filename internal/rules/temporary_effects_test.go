package rules

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/pjunak/addon-dnd-engine/character"
)

// temporaryFixture is a level 2 fighter with synthetic conditions, a feature
// the player can switch on and armor with Stealth Disadvantage to wear.
func temporaryFixture(t *testing.T) (character.Inputs, memoryRecords, Ruleset) {
	t.Helper()
	input, records, profile := conditionFixture(t)
	records.byKind["rule"]["table-states"] = mustJSON(Object{"id": "table-states", "kind": "rule", "name": "Synthetic condition rules", "book": "test", "conditionDefinitions": []conditionDefinition{
		{ID: "fatigue", Name: "Fatigue", LabelKey: "condition.fatigue", MaximumLevel: 6, D20PenaltyPerLevel: 2, SpeedPenaltyPerLevel: 5},
		{ID: "held", Name: "Held", MaximumLevel: 1, SpeedZero: true},
		{ID: "queasy", Name: "Queasy", MaximumLevel: 1, Rolls: []conditionRoll{{On: character.RollAttack, Mode: character.RollDisadvantage}, {On: character.RollCheck, Mode: character.RollDisadvantage}}},
	}})
	input.Build.Levels = append(input.Build.Levels, character.Level{ID: "level-two", ClassID: "fighter"})
	records.byKind["feature"] = map[string]json.RawMessage{
		"fighter-stride": mustJSON(Object{"id": "fighter-stride", "kind": "feature", "name": "Stride", "classId": "fighter", "level": 1,
			"activations": []any{Object{"id": "stride", "name": "Stride", "modifiers": []any{
				Object{"target": "speed", "add": 10},
				Object{"target": "advantage", "on": "save", "abilities": []any{"STR"}},
			}}}}),
	}
	records.byKind["armor"] = map[string]json.RawMessage{"plate": mustJSON(Object{"kind": "armor", "id": "plate", "name": "Plate", "armorType": "heavy", "baseAC": 18, "dexCap": 0, "stealthDisadvantage": true})}
	return input, records, profile
}

func temporaryEffectsOf(t *testing.T, input character.Inputs, records Records, profile Ruleset) (character.Result, []character.TemporaryEffect) {
	t.Helper()
	result := EvaluateCharacter(input, records, profile)
	if result.Sheet.Calculation == nil {
		t.Fatal("fixture is not calculated", result.Issues)
	}
	return result, TemporaryEffects(result, records, profile)
}

func markOf(effects []character.TemporaryEffect, path, row string) *character.TemporaryEffect {
	for index := range effects {
		if effects[index].Path == path && effects[index].Row == row {
			return &effects[index]
		}
	}
	return nil
}

func intOf(value int) *int    { return &value }
func boolOf(value bool) *bool { return &value }

func TestTemporaryEffectsMarkAConditionsDisadvantage(t *testing.T) {
	input, records, profile := temporaryFixture(t)
	input.Play.Conditions = []character.Condition{{ID: "queasy", Level: 1}}
	_, effects := temporaryEffectsOf(t, input, records, profile)
	athletics := markOf(effects, "skills.athletics.total", "")
	want := character.EffectCause{Kind: character.CauseCondition, ID: "queasy", Name: "Queasy", Tone: character.TonePenalty, Changes: []character.EffectChange{{Kind: character.ChangeDisadvantage}}}
	if athletics == nil || athletics.Tone != character.TonePenalty || !reflect.DeepEqual(athletics.Causes, []character.EffectCause{want}) {
		t.Fatalf("skill check: %+v", athletics)
	}
	if markOf(effects, "derived.armorClass", "") != nil || markOf(effects, "saves.STR.total", "") != nil {
		t.Fatal("a value the condition does not change was marked", effects)
	}
}

func TestTemporaryEffectsMarkTheD20AdjustmentOnEveryD20ValueAndSpeed(t *testing.T) {
	input, records, profile := temporaryFixture(t)
	input.Play.Conditions = []character.Condition{{ID: "fatigue", Level: 2}}
	result, effects := temporaryEffectsOf(t, input, records, profile)
	d20 := character.EffectCause{Kind: character.CauseCondition, ID: "fatigue", Name: "Fatigue", LabelKey: "condition.fatigue", Level: 2, Tone: character.TonePenalty,
		Changes: []character.EffectChange{{Kind: character.ChangeAdd, Value: intOf(-4), Included: boolOf(false)}}}
	paths := []string{"derived.initiative", "abilities.STR.mod", "saves.CON.total", "skills.stealth.total"}
	for _, path := range paths {
		mark := markOf(effects, path, "")
		if mark == nil || mark.Tone != character.TonePenalty || !reflect.DeepEqual(mark.Causes, []character.EffectCause{d20}) {
			t.Fatalf("%s: %+v", path, mark)
		}
	}
	// The printed bonus stays as it was; only the roll is adjusted.
	if result.Sheet.Derived.Initiative != EvaluateCharacter(func() character.Inputs { input.Play.Conditions = nil; return input }(), records, profile).Sheet.Derived.Initiative {
		t.Fatal("the D20 adjustment entered the printed Initiative")
	}
	speed := markOf(effects, "derived.speed", "")
	if speed == nil || !reflect.DeepEqual(speed.Causes[0].Changes, []character.EffectChange{{Kind: character.ChangeAdd, Value: intOf(-10), Included: boolOf(true)}}) {
		t.Fatalf("speed: %+v", speed)
	}
	for _, path := range []string{"derived.armorClass", "derived.passivePerception", "abilities.STR.score", "derived.maxHp"} {
		if markOf(effects, path, "") != nil {
			t.Fatal("the D20 adjustment marked a value that is no D20 Test:", path)
		}
	}
}

func TestTemporaryEffectsMarkSpeedZeroAsSet(t *testing.T) {
	input, records, profile := temporaryFixture(t)
	input.Play.Conditions = []character.Condition{{ID: "held", Level: 1}}
	_, effects := temporaryEffectsOf(t, input, records, profile)
	speed := markOf(effects, "derived.speed", "")
	if speed == nil || speed.Tone != character.TonePenalty || !reflect.DeepEqual(speed.Causes[0].Changes, []character.EffectChange{{Kind: character.ChangeSet, Value: intOf(0), Included: boolOf(true)}}) {
		t.Fatalf("speed zero: %+v", speed)
	}
}

func TestTemporaryEffectsMarkAnActiveFeature(t *testing.T) {
	input, records, profile := temporaryFixture(t)
	key := activationKey(EvaluateCharacter(input, records, profile), "stride")
	input.Play.ActiveFeatures = map[string]bool{key: true}
	_, effects := temporaryEffectsOf(t, input, records, profile)
	speed := markOf(effects, "derived.speed", "")
	want := character.EffectCause{Kind: character.CauseActiveFeature, ID: key, Name: "Stride", Tone: character.ToneBonus, Changes: []character.EffectChange{{Kind: character.ChangeAdd, Value: intOf(10), Included: boolOf(true)}}}
	if speed == nil || speed.Tone != character.ToneBonus || !reflect.DeepEqual(speed.Causes, []character.EffectCause{want}) {
		t.Fatalf("speed: %+v", speed)
	}
	save := markOf(effects, "saves.STR.total", "")
	if save == nil || save.Tone != character.ToneBonus || save.Causes[0].Changes[0].Kind != character.ChangeAdvantage {
		t.Fatalf("save: %+v", save)
	}
}

func TestTemporaryEffectsMarkATimedGrantButNotAnExpiredOne(t *testing.T) {
	input, records, profile := temporaryFixture(t)
	grant := character.Grant{ID: "shrine", Name: "Shrine blessing", ActorID: "dm", GrantedAt: input.Play.AsOf, Active: true, EffectiveLevel: 1, Condition: character.ConditionAlways,
		ExpiresAt: "2026-09-12T00:00:00Z", Effects: []character.Effect{{Target: character.TargetArmorClass, Mode: character.EffectMode("add"), Value: 1}}}
	input.Grants = []character.Grant{grant}
	_, effects := temporaryEffectsOf(t, input, records, profile)
	ac := markOf(effects, "derived.armorClass", "")
	want := character.EffectCause{Kind: character.CauseTimedGrant, ID: "shrine", Name: "Shrine blessing", Tone: character.ToneBonus, Changes: []character.EffectChange{{Kind: character.ChangeAdd, Value: intOf(1), Included: boolOf(true)}}}
	if ac == nil || !reflect.DeepEqual(ac.Causes, []character.EffectCause{want}) || len(effects) != 1 {
		t.Fatalf("timed grant: %+v", effects)
	}
	grant.ExpiresAt = "2026-09-10T00:00:00Z"
	input.Grants = []character.Grant{grant}
	if _, effects := temporaryEffectsOf(t, input, records, profile); len(effects) != 0 {
		t.Fatal("an expired grant was marked", effects)
	}
	grant.ExpiresAt = ""
	input.Grants = []character.Grant{grant}
	if _, effects := temporaryEffectsOf(t, input, records, profile); len(effects) != 0 {
		t.Fatal("a grant without an expiry was marked", effects)
	}
}

func TestTemporaryEffectsMixABonusAndAPenaltyOnOneValue(t *testing.T) {
	input, records, profile := temporaryFixture(t)
	key := activationKey(EvaluateCharacter(input, records, profile), "stride")
	input.Play.ActiveFeatures = map[string]bool{key: true}
	input.Play.Conditions = []character.Condition{{ID: "fatigue", Level: 1}}
	_, effects := temporaryEffectsOf(t, input, records, profile)
	speed := markOf(effects, "derived.speed", "")
	if speed == nil || speed.Tone != character.ToneMixed || len(speed.Causes) != 2 ||
		speed.Causes[0].Kind != character.CauseCondition || speed.Causes[0].Tone != character.TonePenalty ||
		speed.Causes[1].Kind != character.CauseActiveFeature || speed.Causes[1].Tone != character.ToneBonus {
		t.Fatalf("mixed speed: %+v", speed)
	}
}

func TestTemporaryEffectsLeaveArmorsStealthDisadvantageUnmarked(t *testing.T) {
	input, records, profile := temporaryFixture(t)
	input.Play.Inventory = []character.Item{{ID: "plate", Name: "Plate", Quantity: 1, Location: "equipped", Reference: &character.Reference{Kind: "armor", ID: "plate"}}}
	result, effects := temporaryEffectsOf(t, input, records, profile)
	if !result.Sheet.Skills["stealth"].Disadvantage {
		t.Fatal("fixture armor gives no Stealth Disadvantage")
	}
	if len(effects) != 0 {
		t.Fatal("worn armor was marked as temporary", effects)
	}
}

func TestTemporaryEffectsIgnoreAConditionTheCharacterIsImmuneTo(t *testing.T) {
	input, records, profile := temporaryFixture(t)
	species := Object{"kind": "species", "id": "dwarf", "name": "Dwarf", "speeds": Object{"walk": 30}, "grants": Object{"conditionImmunities": []any{"held"}}}
	records.byKind["species"]["dwarf"] = mustJSON(species)
	input.Play.Conditions = []character.Condition{{ID: "held", Level: 1}}
	result, effects := temporaryEffectsOf(t, input, records, profile)
	if result.Sheet.Conditions[0].Status != character.ConditionImmune || result.Sheet.Derived.Speed == 0 {
		t.Fatal("fixture is not immune", result.Sheet.Conditions)
	}
	if len(effects) != 0 {
		t.Fatal("a condition the character is immune to was marked", effects)
	}
}

func TestTemporaryEffectsCostNothingWithoutCauses(t *testing.T) {
	input, records, profile := temporaryFixture(t)
	result := EvaluateCharacter(input, records, profile)
	calls, previous := 0, verifyRecords
	verifyRecords = func(snapshot *characterRecordSnapshot) {
		calls++
		previous(snapshot)
	}
	defer func() { verifyRecords = previous }()
	if effects := TemporaryEffects(result, records, profile); effects != nil || calls != 0 {
		t.Fatal("a character without temporary causes was evaluated again", effects, calls)
	}
	if TemporaryEffects(character.Result{}, records, profile) != nil {
		t.Fatal("an uncalculated sheet was marked")
	}
}

func TestTemporaryEffectsLeaveTheResultIntact(t *testing.T) {
	input, records, profile := temporaryFixture(t)
	key := activationKey(EvaluateCharacter(input, records, profile), "stride")
	input.Play.ActiveFeatures = map[string]bool{key: true}
	input.Play.Conditions = []character.Condition{{ID: "fatigue", Level: 1}, {ID: "queasy", Level: 1}}
	input.Grants = []character.Grant{{ID: "shrine", Name: "Shrine blessing", ActorID: "dm", GrantedAt: input.Play.AsOf, Active: true, EffectiveLevel: 1, Condition: character.ConditionAlways, ExpiresAt: "2026-09-12T00:00:00Z", Effects: []character.Effect{{Target: character.TargetArmorClass, Mode: "add", Value: 1}}}}
	result := EvaluateCharacter(input, records, profile)
	before, _ := json.Marshal(result)
	first := TemporaryEffects(result, records, profile)
	after, _ := json.Marshal(result)
	if string(before) != string(after) || !reflect.DeepEqual(first, TemporaryEffects(result, records, profile)) {
		t.Fatal("marking changed the result or is not deterministic")
	}
}

func TestDamageTextsCompareByAverage(t *testing.T) {
	for _, test := range []struct {
		now, before string
		kind        character.ChangeKind
		tone        character.EffectTone
	}{
		{"1d8 +6", "1d8 +4", character.ChangeAdd, character.ToneBonus},
		{"1d10 +4", "1d8 +4", character.ChangeOther, character.ToneBonus},
		{"1d6 +5", "1d8 +4", character.ChangeOther, character.ToneMixed},
		{"3", "5", character.ChangeAdd, character.TonePenalty},
	} {
		now, before := test.now, test.before
		changes, tones := valueChanges(markedValue{damage: &now}, markedValue{damage: &before}, 0)
		if len(changes) != 1 || changes[0].Kind != test.kind || tones[0] != test.tone {
			t.Fatal(test, changes, tones)
		}
	}
}

func TestTemporaryEffectsMarkAttackRowsByStableKey(t *testing.T) {
	input, records, profile := temporaryFixture(t)
	profile.Constants.UnarmedStrike = &UnarmedStrike{Name: "Unarmed Strike", Damage: "1", DamageType: "bludgeoning"}
	input.Play.Conditions = []character.Condition{{ID: "queasy", Level: 1}, {ID: "fatigue", Level: 1}}
	_, effects := temporaryEffectsOf(t, input, records, profile)
	attack := markOf(effects, "weapons.attackBonus", "unarmed")
	if attack == nil || attack.Tone != character.TonePenalty || len(attack.Causes) != 2 ||
		attack.Causes[0].ID != "queasy" || attack.Causes[0].Changes[0].Kind != character.ChangeDisadvantage ||
		attack.Causes[1].ID != "fatigue" || *attack.Causes[1].Changes[0].Value != -2 || *attack.Causes[1].Changes[0].Included {
		t.Fatalf("attack row: %+v", attack)
	}
}

// BenchmarkTemporaryEffects measures marking three causes (two conditions and
// a timed grant) on the large multiclass catalog: one more evaluation each.
func BenchmarkTemporaryEffects(b *testing.B) {
	input, records, profile := multiclassCatalogFixture(b)
	profile.Constants.Character.ConditionRules = "table-states"
	records.byKind["rule"] = map[string]json.RawMessage{"table-states": mustJSON(Object{"id": "table-states", "kind": "rule", "name": "Synthetic condition rules", "book": "test", "conditionDefinitions": []conditionDefinition{
		{ID: "fatigue", Name: "Fatigue", MaximumLevel: 6, D20PenaltyPerLevel: 2, SpeedPenaltyPerLevel: 5},
		{ID: "queasy", Name: "Queasy", MaximumLevel: 1, Rolls: []conditionRoll{{On: character.RollCheck, Mode: character.RollDisadvantage}}},
	}})}
	input.Play.Conditions = []character.Condition{{ID: "fatigue", Level: 2}, {ID: "queasy", Level: 1}}
	input.Grants = []character.Grant{{ID: "shrine", Name: "Shrine blessing", ActorID: "dm", GrantedAt: input.Play.AsOf, Active: true, EffectiveLevel: 1, Condition: character.ConditionAlways, ExpiresAt: "2026-09-12T00:00:00Z", Effects: []character.Effect{{Target: character.TargetArmorClass, Mode: "add", Value: 1}}}}
	result := EvaluateCharacter(input, records, profile)
	b.ReportAllocs()
	for b.Loop() {
		if len(TemporaryEffects(result, records, profile)) == 0 {
			b.Fatal("no marks")
		}
	}
}
