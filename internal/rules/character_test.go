package rules

import (
	"encoding/json"
	"fmt"
	"github.com/pjunak/addon-dnd-engine/character"
	"reflect"
	"testing"
)

func TestCharacterConditionalMovementSensesAndExpiry(t *testing.T) {
	input, source, profile := characterFixture(t)
	records := source.(memoryRecords)
	extra := syntheticRecords()
	feat := recordByID(extra, "feat", "guardian")
	activation := objects(object(feat["grants"])["activations"])[0]
	activation["condition"] = "While the stance is active and touching stone"
	activation["modifiers"] = append(values(activation["modifiers"]), Object{"target": "sense", "key": "tremorsense", "value": 60})
	records.byKind["feat"] = map[string]json.RawMessage{"guardian": mustJSON(feat)}
	input.Grants = []character.Grant{
		{ID: "feat-reward", Name: "Stance training", ActorID: "dm", Reason: "Training", GrantedAt: input.Play.AsOf, Active: true, EffectiveLevel: 1, Condition: "always", Feat: &character.Reference{Kind: "feat", ID: "guardian"}},
		{ID: "speed-reward", Name: "Speed blessing", ActorID: "dm", Reason: "Quest", GrantedAt: input.Play.AsOf, Active: true, EffectiveLevel: 1, Condition: "always", ExpiresAt: "2026-09-12T00:00:00Z", Effects: []character.Effect{{Target: "speed", Mode: "add", Value: 10}}},
	}
	base := EvaluateCharacter(input, records, profile)
	if !base.Ready {
		t.Fatal(base.Issues)
	}
	key := text(objects(base.Sheet["activations"])[0]["key"])
	active, err := ApplyCharacterPlay(input, Object{"operation": "toggle-feature", "key": key, "enabled": true}, records, profile)
	if err != nil || !active.Ready {
		t.Fatalf("activate: %v %v", err, active.Issues)
	}
	if integer(active.Sheet["flySpeed"], 0) != 45 || integer(object(active.Sheet["senses"])["tremorsense"], 0) != 60 {
		t.Fatalf("conditional results: %v %v", active.Sheet["flySpeed"], active.Sheet["senses"])
	}
	explanation := active.Explanations["senses.tremorsense"]
	if len(explanation.Terms) == 0 || explanation.Terms[0].Source == nil || explanation.Terms[0].Status != "applied" {
		t.Fatalf("missing condition/source: %+v", explanation)
	}
	expired := cloneCharacter(active.Inputs)
	expired.Play.AsOf = "2026-09-12T00:00:00Z"
	if value := integer(EvaluateCharacter(expired, records, profile).Sheet["flySpeed"], 0); value != 35 {
		t.Fatalf("expired speed persisted: %d", value)
	}
	ended, err := ApplyCharacterPlay(active.Inputs, Object{"operation": "toggle-feature", "key": key, "enabled": false}, records, profile)
	if err != nil || integer(ended.Sheet["flySpeed"], 0) != 0 || integer(object(ended.Sheet["senses"])["tremorsense"], 0) != 0 {
		t.Fatal("ending stance retained conditional effects", err)
	}
	if !reflect.DeepEqual(EvaluateCharacter(active.Inputs, records, profile).Sheet, active.Sheet) {
		t.Fatal("later time mutated historical inputs")
	}
}

func TestCharacterResourceAdjustmentsAreBoundAndReversible(t *testing.T) {
	input, records, profile := characterFixture(t)
	input.Grants = []character.Grant{{ID: "reserve", Name: "Reserve", ActorID: "dm", Reason: "Reward", GrantedAt: input.Play.AsOf, Active: true, EffectiveLevel: 1, Condition: "always", Effects: []character.Effect{{Target: "resourceMax", Key: "hit-dice-d10", Mode: "add", Value: 2}}}}
	input.Play.ResourceUses["hit-dice-d10"] = 2
	result := EvaluateCharacter(input, records, profile)
	pool := findPlayResource(Object(result.Sheet), "hit-dice-d10")
	if !result.Ready || integer(pool["remaining"], 0) != 1 {
		t.Fatalf("adjusted capacity: %v %v", result.Issues, pool)
	}
	input.Grants[0].Active = false
	if EvaluateCharacter(input, records, profile).Ready {
		t.Fatal("revocation silently erased spent uses")
	}
	input.Grants[0].Active = true
	input.Grants[0].Effects[0].Key = "unknown-resource"
	if EvaluateCharacter(input, records, profile).Ready {
		t.Fatal("unknown resource adjustment silently ignored")
	}
}

func TestCharacterKnownSpellSwapHasRecordedLevelAndCannotRepeat(t *testing.T) {
	input, source, profile := characterFixture(t)
	records := spellPlayRecords()
	for kind, entries := range source.(memoryRecords).byKind {
		if kind != "class" {
			records.byKind[kind] = entries
		}
	}
	input.Build.Levels = []character.Level{}
	for i := 0; i < 5; i++ {
		input.Build.Levels = append(input.Build.Levels, character.Level{ID: fmt.Sprintf("level-%d", i), ClassID: "warlock"})
	}
	input.Play.PreparedSpells["warlock"] = []string{"ward"}
	warlock := recordByID(records, "class", "warlock")
	object(warlock["spellcasting"])["levelReplacements"] = 1
	records.byKind["class"]["warlock"] = mustJSON(warlock)
	for _, choice := range objects(EvaluateCharacter(input, records, profile).Plan["classChoices"]) {
		if text(choice["kind"]) == "asiMode" {
			input.Build.Choices = append(input.Build.Choices, character.Choice{ID: text(choice["id"]), Value: mustJSON("asi")}, character.Choice{ID: text(object(choice["ability"])["id"]), Value: mustJSON(Object{"CHA": 2})})
		}
	}
	result, err := ApplyCharacterPlay(input, Object{"operation": "swap-spell", "classId": "warlock", "out": "ward", "ref": "find-path"}, records, profile)
	if err != nil {
		t.Fatalf("%v: %+v", err, result.Issues)
	}
	if len(result.Inputs.Build.Spells.Swaps) != 1 || result.Inputs.Build.Spells.Swaps[0].Origin != "recorded" {
		t.Fatalf("swap ledger: %+v", result.Inputs.Build.Spells.Swaps)
	}
	if _, err := ApplyCharacterPlay(result.Inputs, Object{"operation": "swap-spell", "classId": "warlock", "out": "find-path", "ref": "ward"}, records, profile); err == nil {
		t.Fatal("same level replacement allowance reused")
	}
}

func mustJSON(value any) json.RawMessage {
	body, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return body
}

func characterFixture(t *testing.T) (character.Inputs, Records, Ruleset) {
	t.Helper()
	profile := syntheticRuleset(t)
	profile.Constants.Character = &CharacterPolicy{MaximumLevel: 20, MinimumHPGain: 1, StandardArray: []int{15, 14, 13, 12, 10, 8}, RollDice: 4, RollSides: 6, RollKeep: 3, DistanceUnit: "ft"}
	profile.Builder.BackgroundAbilityGrant = json.RawMessage("false")
	records := newMemoryRecords([]Object{
		{"kind": "class", "id": "fighter", "name": "Fighter", "hitDie": "d10"},
		{"kind": "species", "id": "dwarf", "name": "Dwarf", "speeds": Object{"walk": 30}, "senses": Object{"darkvision": 120}},
		{"kind": "background", "id": "artisan", "name": "Artisan"},
	})
	input := character.Blank()
	input.Build.Method = "array"
	input.Build.BaseScores = map[string]int{"STR": 15, "DEX": 14, "CON": 13, "INT": 12, "WIS": 10, "CHA": 8}
	input.Build.Species = "dwarf"
	input.Build.Background = "artisan"
	input.Build.Levels = []character.Level{{ID: "level-one", ClassID: "fighter"}}
	input.Play.AsOf = "2026-09-11T00:00:00Z"
	return input, records, profile
}

func TestCharacterDeterministicBuildAndDMPropagation(t *testing.T) {
	input, records, profile := characterFixture(t)
	original := cloneCharacter(input)
	base := EvaluateCharacter(input, records, profile)
	if !base.Ready {
		t.Fatalf("base blockers: %+v", base.Issues)
	}
	input.Grants = []character.Grant{{ID: "boon", Name: "Constitution boon", Reason: "Quest reward", ActorID: "dm", GrantedAt: input.Play.AsOf, Active: true, EffectiveLevel: 1, Condition: "always", Effects: []character.Effect{{Target: "abilityScore", Key: "CON", Mode: "add", Value: 2}}}}
	result := EvaluateCharacter(input, records, profile)
	if !result.Ready {
		t.Fatalf("grant blockers: %+v", result.Issues)
	}
	if integer(object(result.Sheet["derived"])["maxHp"], 0) != 12 {
		t.Fatalf("Constitution did not propagate: %v", result.Sheet["hp"])
	}
	if len(result.Explanations["abilities.CON.score"].Terms) < 4 {
		t.Fatal("missing grant explanation")
	}
	again := EvaluateCharacter(input, records, profile)
	if !reflect.DeepEqual(result, again) {
		t.Fatal("evaluation is not deterministic")
	}
	if !reflect.DeepEqual(original, base.Inputs) {
		t.Fatal("evaluation mutated inputs")
	}
	input.Grants[0].Active = false
	if integer(object(EvaluateCharacter(input, records, profile).Sheet["derived"])["maxHp"], 0) != 11 {
		t.Fatal("revocation did not reverse derived effect")
	}
}

func TestCharacterHPBoundsAndRetainedInvalidChoices(t *testing.T) {
	input, records, profile := characterFixture(t)
	input.Play.HP = 10
	input.Play.TemporaryHP = 3
	result, err := ApplyCharacterPlay(input, Object{"operation": "damage", "amount": float64(5)}, records, profile)
	if err != nil || result.Inputs.Play.HP != 8 || result.Inputs.Play.TemporaryHP != 0 {
		t.Fatalf("damage: %+v %v", result.Inputs.Play, err)
	}
	if _, err = ApplyCharacterPlay(input, Object{"operation": "set-hp", "amount": float64(99)}, records, profile); err == nil {
		t.Fatal("unbounded HP accepted")
	}
	input.Build.Choices = []character.Choice{{ID: "removed-source", Value: json.RawMessage(`"old-pick"`)}}
	result = EvaluateCharacter(input, records, profile)
	if result.Ready || len(result.Inputs.Build.Choices) != 1 {
		t.Fatal("invalid earlier decision was silently removed or accepted")
	}
}

func TestCharacterMalformedPrerequisitesFailClosed(t *testing.T) {
	for _, predicate := range []Object{{"level": "garbage"}, {"abilities": Object{}}, {"abilities": Object{"STR": -1}}, {"feature": false}} {
		if _, known := prerequisiteMatches(predicate, Object{"totalLevel": 20}); known {
			t.Fatalf("accepted malformed predicate: %v", predicate)
		}
	}
}

func TestCharacterRecordedHitDieAndRestUseSpentCounters(t *testing.T) {
	input, records, profile := characterFixture(t)
	input.Play.HP = 1
	result, err := ApplyCharacterPlay(input, Object{"operation": "spend-hit-die", "key": "hit-dice-d10", "result": float64(4), "rollId": "recorded-one"}, records, profile)
	if err != nil || result.Inputs.Play.HP != 6 || result.Inputs.Play.ResourceUses["hit-dice-d10"] != 1 || len(result.Inputs.Play.Rolls) != 1 {
		t.Fatalf("recorded hit die: %+v %v", result.Inputs.Play, err)
	}
	if _, err = ApplyCharacterPlay(result.Inputs, Object{"operation": "spend-hit-die", "key": "hit-dice-d10", "result": float64(10), "rollId": "recorded-two"}, records, profile); err == nil {
		t.Fatal("spent an exhausted hit die")
	}
	rested, err := ApplyCharacterPlay(result.Inputs, Object{"operation": "rest", "rest": "long"}, records, profile)
	if err != nil || rested.Inputs.Play.ResourceUses["hit-dice-d10"] != 0 || rested.Inputs.Play.HP != 11 {
		t.Fatalf("rest: %+v %v", rested.Inputs.Play, err)
	}
	if !reflect.DeepEqual(rested.Inputs.Play.Rolls, result.Inputs.Play.Rolls) {
		t.Fatal("rest erased recorded rolls")
	}
}

func TestCharacterItemsShareEffectsWithoutForgingDMGrants(t *testing.T) {
	input, source, profile := characterFixture(t)
	records := source.(memoryRecords)
	extra := newMemoryRecords([]Object{{"kind": "magic-item", "id": "synthetic-amulet", "name": "Synthetic amulet", "attunement": true, "characterMechanics": "complete", "characterEffects": []any{Object{"target": "abilityScore", "key": "CON", "mode": "minimum", "value": 19}}}})
	records.byKind["magic-item"] = extra.byKind["magic-item"]
	ref := &character.Reference{Kind: "magic-item", ID: "synthetic-amulet"}
	input.Play.Inventory = []character.Item{{ID: "one", Reference: ref, Name: "Amulet", Quantity: 1, Location: "equipped", Attuned: true}, {ID: "two", Reference: ref, Name: "Spare amulet", Quantity: 1, Location: "equipped", Attuned: true}}
	result := EvaluateCharacter(input, records, profile)
	if !result.Ready || integer(object(object(result.Sheet["abilities"])["CON"])["score"], 0) != 19 || integer(object(result.Sheet["derived"])["maxHp"], 0) != 14 {
		t.Fatalf("item projection: %v %v", result.Issues, result.Sheet["abilities"])
	}
	if len(result.Inputs.Grants) != 0 {
		t.Fatal("source mechanics became a DM grant")
	}
	profile.Constants.Character.UniqueAttunement = true
	if EvaluateCharacter(input, records, profile).Ready {
		t.Fatal("attuned two copies of the same item")
	}
	profile.Constants.Character.UniqueAttunement = false
	terms := result.Explanations["abilities.CON.score"].Terms
	if terms[len(terms)-1].Status != "inactive" || terms[len(terms)-1].Source == nil || terms[len(terms)-1].GrantID != "" {
		t.Fatalf("suppressed source provenance missing: %v", terms)
	}
	input.Play.Inventory[0].Attuned = false
	input.Play.Inventory[1].Attuned = false
	result = EvaluateCharacter(input, records, profile)
	if integer(object(result.Sheet["derived"])["maxHp"], 0) != 11 {
		t.Fatal("unattunement retained item effects")
	}
	input.Play.Inventory[0].Attuned = true
	record := recordByID(records, "magic-item", ref.ID)
	delete(record, "characterMechanics")
	records.byKind["magic-item"][ref.ID], _ = json.Marshal(record)
	if EvaluateCharacter(input, records, profile).Ready {
		t.Fatal("unstructured magic item accepted silently")
	}
}

func TestCharacterUnknownFoundationsDoNotInventStats(t *testing.T) {
	_, records, profile := characterFixture(t)
	result := EvaluateCharacter(character.Blank(), records, profile)
	if result.Ready || result.Sheet["status"] != "needs-choices" || result.Sheet["derived"] != nil {
		t.Fatalf("invented incomplete stats: %+v", result.Sheet)
	}
}

func TestCharacterAbilityEffectsApplyAfterIncreasesAndRaisedCaps(t *testing.T) {
	input, records, profile := characterFixture(t)
	for _, example := range []struct {
		base   int
		effect character.Effect
		want   int
	}{
		{13, character.Effect{Target: "abilityScore", Key: "CON", Mode: "set", Value: 19}, 19},
		{19, character.Effect{Target: "abilityCap", Key: "CON", Mode: "add", Value: 2}, 21},
	} {
		input.Grants = []character.Grant{{ID: "boon", Name: "Reward", Active: true, Condition: "always", EffectiveLevel: 1, Effects: []character.Effect{example.effect}}}
		decisions := Object{"baseStats": Object{"CON": example.base}, "abilityGrants": []any{Object{"assign": Object{"CON": 2}, "cap": 20}}}
		result := character.Result{}
		applyCharacterAbilityEffects(input, decisions, &result, profile, records)
		if effective := integer(object(decisions["baseStats"])["CON"], 0) + 2; effective != example.want {
			t.Fatalf("effect applied before increases/cap: %d, want %d", effective, example.want)
		}
	}
}

func TestCharacterMulticlassRequirementsUseEarlierLevels(t *testing.T) {
	input, source, profile := characterFixture(t)
	records := source.(memoryRecords)
	for _, record := range []Object{
		{"kind": "class", "id": "fighter", "name": "Fighter", "hitDie": "d10", "multiclassRequirements": Object{"STR|DEX": 13}},
		{"kind": "class", "id": "mage", "name": "Mage", "hitDie": "d6", "multiclassRequirements": Object{"INT": 13}},
	} {
		records.byKind["class"][text(record["id"])], _ = json.Marshal(record)
	}
	input.Build.Levels = append(input.Build.Levels, character.Level{ID: "level-two", ClassID: "mage"})
	input.Grants = []character.Grant{{ID: "later-boon", Name: "Later reward", Reason: "After multiclass", ActorID: "dm", GrantedAt: input.Play.AsOf, Active: true, Condition: "always", EffectiveLevel: 2, Effects: []character.Effect{{Target: "abilityScore", Key: "INT", Mode: "add", Value: 2}}}}
	result := EvaluateCharacter(input, records, profile)
	found := false
	for _, issue := range result.Issues {
		if issue.ID == "multiclass:level-two:mage" {
			found = true
		}
	}
	if !found {
		t.Fatal("later ability reward qualified an earlier multiclass decision")
	}
	input.Grants[0].EffectiveLevel = 1
	result = EvaluateCharacter(input, records, profile)
	for _, issue := range result.Issues {
		if issue.ID == "multiclass:level-two:mage" {
			t.Fatal("earlier qualifying reward ignored")
		}
	}
}

func TestCharacterCopySpellRecordsCostAndDoesNotRecreateScroll(t *testing.T) {
	input, origin, profile := characterFixture(t)
	records := spellPlayRecords()
	for kind, entries := range origin.(memoryRecords).byKind {
		if kind != "class" {
			records.byKind[kind] = entries
		}
	}
	input.Build.Levels[0].ClassID = "wizard"
	profile.Constants.Spellbook.BaseKnown = 1
	input.Build.Spells.Spellbook["wizard"] = []string{"ward"}
	input.Play.Currency["gp"] = 57
	input.Play.Inventory = []character.Item{{ID: "scroll", Name: "Personal label", SpellID: "find-path", Quantity: 1, Location: "carried", Notes: "Do not recreate"}}
	command := Object{"operation": "copy-spell", "classId": "wizard", "ref": "find-path", "scrollId": "scroll", "acquisitionId": "copy-one"}
	result, err := ApplyCharacterPlay(input, command, records, profile)
	if err != nil || !result.Ready {
		t.Fatalf("copy: %v %+v", err, result.Issues)
	}
	if result.Inputs.Play.Currency["gp"] != 7 || result.Inputs.Play.Inventory[0].Quantity != 0 || len(result.Inputs.Build.Spells.Acquisitions) != 1 {
		t.Fatalf("copy did not record exact costs: %+v", result.Inputs)
	}
	again := EvaluateCharacter(result.Inputs, records, profile)
	if !again.Ready || again.Inputs.Play.Inventory[0].Quantity != 0 {
		t.Fatal("recalculation recreated consumed scroll or lost copied spell capacity")
	}
	if _, err := ApplyCharacterPlay(result.Inputs, command, records, profile); err == nil {
		t.Fatal("duplicate acquisition charged again")
	}
	input.Play.Inventory[0].SpellID = "ward"
	if _, err := ApplyCharacterPlay(input, command, records, profile); err == nil {
		t.Fatal("copy used unrelated scroll based on a name")
	}
}

func TestCharacterSpellbookChecksTheLevelOfEachGrantedSlot(t *testing.T) {
	input, origin, profile := characterFixture(t)
	records := spellPlayRecords()
	for kind, entries := range origin.(memoryRecords).byKind {
		if kind != "class" {
			records.byKind[kind] = entries
		}
	}
	profile.Constants.Spellbook.BaseKnown = 1
	profile.Constants.Spellbook.KnownPerLevel = 1
	input.Build.Levels = []character.Level{}
	for index := 1; index <= 5; index++ {
		input.Build.Levels = append(input.Build.Levels, character.Level{ID: fmt.Sprintf("level-%d", index), ClassID: "wizard"})
	}
	for index := 1; index <= 4; index++ {
		id := fmt.Sprintf("early-%d", index)
		records.byKind["spell"][id] = mustJSON(Object{"kind": "spell", "id": id, "name": id, "level": 1, "classes": []any{"wizard"}})
	}
	records.byKind["spell"]["late"] = mustJSON(Object{"kind": "spell", "id": "late", "name": "Late spell", "level": 3, "classes": []any{"wizard"}})
	input.Build.Spells.Spellbook["wizard"] = []string{"late", "early-1", "early-2", "early-3", "early-4"}
	result := character.Result{SpellOptions: map[string]any{}}
	validateCharacterSpellProgression(input, records, profile, &result)
	if len(result.Issues) != 1 || result.Issues[0].ID != "spell-acquired-level:wizard:late" {
		t.Fatalf("late spell qualified for early slot: %+v", result.Issues)
	}
	input.Build.Spells.Spellbook["wizard"] = []string{"early-1", "early-2", "early-3", "early-4", "late"}
	result = character.Result{SpellOptions: map[string]any{}}
	validateCharacterSpellProgression(input, records, profile, &result)
	if len(result.Issues) != 0 {
		t.Fatalf("valid ordered progression: %+v", result.Issues)
	}
	last := objects(result.SpellOptions["acquisitionLevels"])[4]
	if integer(last["level"], 0) != 5 || integer(last["classLevel"], 0) != 5 {
		t.Fatalf("missing effective acquisition level: %+v", last)
	}
}

func TestCharacterProfileDoesNotDefaultOmittedZeroValuedPolicy(t *testing.T) {
	_, _, profile := characterFixture(t)
	body := mustJSON(profile)
	var value Object
	_ = json.Unmarshal(body, &value)
	policy := object(object(value["constants"])["character"])
	for _, field := range []string{"uniqueAttunement", "minimumHpGain"} {
		previous := policy[field]
		delete(policy, field)
		if _, err := DecodeRuleset(mustJSON(value)); err == nil {
			t.Fatalf("omitted %s used an implicit default", field)
		}
		policy[field] = previous
	}
	if _, err := DecodeRuleset(mustJSON(value)); err != nil {
		t.Fatalf("complete policy rejected: %v", err)
	}
}
