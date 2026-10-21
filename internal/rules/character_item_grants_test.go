package rules

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/pjunak/addon-dnd-engine/character"
)

// A level 1 singer, a full caster with first-level slots, carrying synthetic
// cards: a spell cast with the card's own use after a casting ability pick, a
// cantrip pick, a sense toggle and a fixed cantrip. Each grants "marked".
func itemGrantFixture(t *testing.T) (character.Inputs, memoryRecords, Ruleset) {
	t.Helper()
	input, source, profile := characterFixture(t)
	records := source.(memoryRecords)
	marked := Object{"id": "marked", "name": "Marked", "summary": "Fate watches the bearer."}
	for kind, entries := range newMemoryRecords([]Object{
		{"kind": "class", "id": "singer", "name": "Singer", "hitDie": "d8",
			"spellcasting": Object{"ability": "CHA", "type": "full", "prepares": "list"},
			"progression":  []any{Object{"level": 1, "cantripsKnown": 0, "preparedSpells": 1}}},
		{"kind": "species", "id": "human", "name": "Human", "speeds": Object{"walk": 30}},
		{"kind": "spell", "id": "blessing", "name": "Blessing", "level": 1, "classes": []any{"priest"}},
		{"kind": "spell", "id": "glow", "name": "Glow", "level": 0, "classes": []any{"sage"}},
		{"kind": "spell", "id": "spark", "name": "Spark", "level": 0, "classes": []any{"singer"}},
		{"kind": "magic-item", "id": "card-guidance", "name": "Guidance Card", "characterMechanics": "complete", "grants": Object{
			"advantages":     []any{Object{"on": "save", "abilities": []any{"WIS"}, "when": "against fear"}},
			"castingAbility": Object{"id": "casting", "choose": []any{"INT", "WIS", "CHA"}},
			"resources":      []any{Object{"key": "uses", "name": "Guidance", "fixed": 1, "recharge": "long"}},
			"spells":         []any{Object{"ids": []any{"blessing"}, "resource": "uses"}},
			"conditions":     []any{marked},
		}},
		{"kind": "magic-item", "id": "card-power", "name": "Power Card", "characterMechanics": "complete", "grants": Object{
			"spells":     []any{Object{"id": "cantrip", "choose": 1, "spellLevel": 0}},
			"conditions": []any{marked},
		}},
		{"kind": "magic-item", "id": "card-sky", "name": "Sky Card", "characterMechanics": "complete", "grants": Object{
			"activations": []any{Object{"id": "open-sky", "name": "Under open sky", "condition": "under the open sky", "modifiers": []any{
				Object{"target": "senseBonus", "key": "darkvision", "add": 30},
				Object{"target": "sense", "key": "darkvision", "value": 60},
			}}},
			"conditions": []any{Object{"id": "marked", "name": "Marked by the sky", "summary": "The sky watches."}},
		}},
		{"kind": "magic-item", "id": "card-glow", "name": "Glow Card", "characterMechanics": "complete", "grants": Object{
			"spells": []any{Object{"ids": []any{"glow"}}}, "resistances": []any{"radiant"}, "conditions": []any{marked},
		}},
	}).byKind {
		if records.byKind[kind] == nil {
			records.byKind[kind] = map[string]json.RawMessage{}
		}
		for id, body := range entries {
			records.byKind[kind][id] = body
		}
	}
	input.Build.Levels = []character.Level{{ID: "one", ClassID: "singer"}}
	input.Play.Inventory = []character.Item{
		itemCard("guidance", "Guidance", "card-guidance"), itemCard("power", "Power", "card-power"),
		itemCard("sky", "Sky", "card-sky"), itemCard("glow", "Glow", "card-glow"),
	}
	return input, records, profile
}

func itemCard(id, name, record string) character.Item {
	return character.Item{ID: id, Name: name, Reference: &character.Reference{Kind: "magic-item", ID: record}, Quantity: 1, Location: "equipped"}
}

func evaluateReady(t *testing.T, input character.Inputs, records Records, profile Ruleset) character.Result {
	t.Helper()
	result := EvaluateCharacter(input, records, profile)
	if !result.Ready {
		t.Fatal("blocked", result.Issues)
	}
	return result
}

func playReady(t *testing.T, input character.Inputs, change Object, records Records, profile Ruleset) character.Result {
	t.Helper()
	result, err := ApplyCharacterPlay(input, change, records, profile)
	if err != nil || !result.Ready {
		t.Fatal(change, err, result.Issues)
	}
	return result
}

func withCastingAbility(t *testing.T, input character.Inputs, records Records, profile Ruleset) character.Result {
	t.Helper()
	return playReady(t, input, Object{"operation": "select-casting-ability", "key": "item:guidance:casting", "ability": "WIS"}, records, profile)
}

func conditionRow(result character.Result, id string) Object {
	for _, row := range objects(result.Sheet["conditions"]) {
		if text(row["id"]) == id && truth(row["granted"]) {
			return row
		}
	}
	return nil
}

func TestItemGrantsAcceptOnlyTheAllowlist(t *testing.T) {
	_, records, _ := itemGrantFixture(t)
	valid := Object{
		"advantages":     []any{Object{"on": "deathSave"}, Object{"on": "check", "mode": "disadvantage", "skills": []any{"stealth"}}},
		"castingAbility": Object{"fixed": "INT"},
		"resources":      []any{Object{"key": "uses", "name": "Uses", "fixed": 100, "recharge": "short"}},
		"spells": []any{
			Object{"ids": []any{"blessing"}, "resource": Object{"key": "uses", "cost": 2}},
			Object{"ids": []any{"glow"}},
			Object{"ids": []any{"unknown-to-this-provider"}},
			Object{"id": "pick", "choose": 1, "maxSpellLevel": 2, "atWill": true, "from": Object{"class": []any{"priest"}, "castingTime": "1 action", "ritual": true}},
		},
		"activations": []any{Object{"id": "toggle", "name": "Toggle", "modifiers": []any{
			Object{"target": "advantage", "on": "attack"}, Object{"target": "resistance", "value": "fire"},
			Object{"target": "conditionImmunity", "value": "charmed"}, Object{"target": "damageImmunity", "value": "poison"},
		}}},
		"resistances": []any{"cold"}, "damageImmunities": []any{"acid"}, "conditionImmunities": []any{"frightened"},
		"conditions": []any{Object{"id": "marked", "name": "Marked"}},
	}
	if grants, ok := characterItemGrants(Object{"grants": valid}, records); !ok || !reflect.DeepEqual(grants, valid) {
		t.Fatal("rejected or rewrote valid item grants", grants)
	}
	var documented Object
	if err := json.Unmarshal([]byte(`{
		"advantages": [{"on": "save", "abilities": ["WIS"], "when": "to resist the Charmed condition"}],
		"castingAbility": {"id": "casting", "choose": ["INT", "WIS", "CHA"]},
		"resources": [{"key": "uses", "name": "Divine Guidance", "fixed": 1, "recharge": "long"}],
		"spells": [{"ids": ["blessing"], "resource": "uses"}, {"id": "cantrip", "choose": 1, "spellLevel": 0}],
		"activations": [{"id": "open-sky", "name": "Under open sky", "modifiers": [
			{"target": "senseBonus", "key": "darkvision", "add": 30},
			{"target": "sense", "key": "darkvision", "value": 60}]}],
		"resistances": ["fire"], "damageImmunities": [], "conditionImmunities": [],
		"conditions": [{"id": "fate-touched", "name": "Fate-Touched", "summary": "One level per card."}]
	}`), &documented); err != nil {
		t.Fatal(err)
	}
	if _, ok := characterItemGrants(Object{"grants": documented}, records); !ok {
		t.Fatal("rejected the documented item grants")
	}
	if grants, ok := characterItemGrants(Object{}, records); !ok || grants != nil {
		t.Fatal("an item without grants is invalid")
	}
	if _, ok := characterItemGrants(Object{"grants": []any{}}, records); ok {
		t.Fatal("accepted grants that are not an object")
	}
	for name, grants := range map[string]Object{
		"unknown field":        {"acFormulas": []any{}},
		"raw choices":          {"choices": []any{}},
		"malformed list":       {"resistances": "fire"},
		"empty spell ids":      {"spells": []any{Object{"ids": []any{}}}},
		"empty name":           {"conditions": []any{Object{"id": "marked", "name": " "}}},
		"condition key":        {"conditions": []any{Object{"id": "marked", "name": "Marked", "level": 1}}},
		"duplicate condition":  {"conditions": []any{Object{"id": "marked", "name": "A"}, Object{"id": "marked", "name": "B"}}},
		"advantage roll":       {"advantages": []any{Object{"on": "damage"}}},
		"advantage key":        {"advantages": []any{Object{"on": "save", "requires": Object{}}}},
		"unknown skill":        {"advantages": []any{Object{"on": "check", "skills": []any{"juggling"}}}},
		"casting both":         {"castingAbility": Object{"fixed": "INT", "choose": []any{"WIS"}}},
		"casting without id":   {"castingAbility": Object{"choose": []any{"WIS"}}},
		"casting by increase":  {"castingAbility": Object{"fromAbilityScoreIncrease": true}},
		"no uses":              {"resources": []any{Object{"key": "uses", "fixed": 0}}},
		"too many uses":        {"resources": []any{Object{"key": "uses", "fixed": 101}}},
		"rest kind":            {"resources": []any{Object{"key": "uses", "fixed": 1, "recharge": "dawn"}}},
		"scaled uses":          {"resources": []any{Object{"key": "uses", "fixed": 1, "proficiencyBonus": true}}},
		"colon key":            {"resources": []any{Object{"key": "a:b", "fixed": 1}}},
		"leveled without uses": {"spells": []any{Object{"ids": []any{"blessing"}}}},
		"choice without uses":  {"spells": []any{Object{"id": "pick", "choose": 1, "spellLevel": 1}}},
		"unbounded choice":     {"spells": []any{Object{"id": "pick", "choose": 1, "atWill": true}}},
		"class level":          {"spells": []any{Object{"id": "pick", "choose": 1, "maxSpellLevel": "class", "atWill": true}}},
		"free cast":            {"spells": []any{Object{"ids": []any{"glow"}, "free": "1/long"}}},
		"always prepared":      {"spells": []any{Object{"ids": []any{"glow"}, "alwaysPrepared": true}}},
		"class owner":          {"spells": []any{Object{"ids": []any{"glow"}, "classId": "singer"}}},
		"replacements":         {"spells": []any{Object{"id": "pick", "choose": 1, "spellLevel": 0, "levelReplacements": 1}}},
		"foreign resource":     {"spells": []any{Object{"ids": []any{"blessing"}, "resource": "ki"}}},
		"spell filter":         {"spells": []any{Object{"id": "pick", "choose": 1, "spellLevel": 0, "from": Object{"level": 1}}}},
		"extends":              {"activations": []any{Object{"id": "more", "extends": "rage"}}},
		"while":                {"activations": []any{Object{"id": "aspect", "while": "rage"}}},
		"exclusive group":      {"activations": []any{Object{"id": "stance", "exclusiveGroup": "stances"}}},
		"activation resource":  {"activations": []any{Object{"id": "burst", "resource": "uses"}}},
		"modifier target":      {"activations": []any{Object{"id": "burst", "modifiers": []any{Object{"target": "armorClass", "add": 1}}}}},
		"modifier key":         {"activations": []any{Object{"id": "burst", "modifiers": []any{Object{"target": "sense", "key": "darkvision", "value": 60, "multiply": 2}}}}},
	} {
		if _, ok := characterItemGrants(Object{"grants": grants}, records); ok {
			t.Errorf("%s: accepted %v", name, grants)
		}
	}

	input, _, profile := itemGrantFixture(t)
	records.byKind["magic-item"]["bad"] = mustJSON(Object{"kind": "magic-item", "id": "bad", "name": "Bad", "characterMechanics": "complete",
		"grants": Object{"resistances": []any{"fire"}, "speeds": Object{"fly": 30}}})
	records.byKind["magic-item"]["story"] = mustJSON(Object{"kind": "magic-item", "id": "story", "name": "Story", "characterMechanics": "narrative",
		"grants": Object{"resistances": []any{"fire"}}})
	input.Play.Inventory = []character.Item{itemCard("bad", "Bad", "bad"), itemCard("story", "Story", "story")}
	result := EvaluateCharacter(input, records, profile)
	if !hasChoiceIssue(result, "item-effects:bad") || !hasChoiceIssue(result, "item-effects:story") || contains(stringsOf(result.Sheet["resistances"]), "fire") {
		t.Fatal("unsupported or narrative grants were applied or accepted", result.Issues, result.Sheet["resistances"])
	}
}

func TestItemGrantsUseItemOwnedKeysAndSources(t *testing.T) {
	input, records, profile := itemGrantFixture(t)
	result := withCastingAbility(t, input, records, profile)
	source := Object{"type": "item", "id": "guidance", "name": "Guidance", "ref": Object{"kind": "magic-item", "id": "card-guidance"}}
	resource := findPlayResource(Object(result.Sheet), "item:guidance:uses")
	if resource == nil || !reflect.DeepEqual(resource["source"], source) || integer(resource["max"], 0) != 1 || resource["active"] != nil ||
		!reflect.DeepEqual(resource["recharge"], []any{Object{"on": "long", "amount": "full"}}) {
		t.Fatal("item use is not keyed and sourced by its instance", resource)
	}
	blessing := grantedOption(result, "blessing")
	if text(blessing["key"]) != "item:guidance:blessing" || text(blessing["castResource"]) != "item:guidance:uses" || !reflect.DeepEqual(blessing["source"], source) || text(blessing["castingAbility"]) != "WIS" {
		t.Fatal("item spell keys", blessing)
	}
	if result.Inputs.Build.Spells.CastingAbilities["item:guidance:casting"] != "WIS" {
		t.Fatal("casting ability key", result.Inputs.Build.Spells.CastingAbilities)
	}
	var toggle Object
	for _, activation := range objects(result.Sheet["activations"]) {
		if text(activation["key"]) == "item:sky:open-sky" {
			toggle = activation
		}
	}
	if toggle == nil || text(toggle["condition"]) != "under the open sky" || text(object(toggle["source"])["type"]) != "item" {
		t.Fatal("item toggle key", result.Sheet["activations"])
	}
	advantage := objects(result.Sheet["advantages"])
	if len(advantage) != 1 || text(advantage[0]["name"]) != "Guidance" || !reflect.DeepEqual(advantage[0]["source"], source) || text(advantage[0]["when"]) != "against fear" {
		t.Fatal("item advantage", advantage)
	}
	if !contains(stringsOf(result.Sheet["resistances"]), "radiant") {
		t.Fatal("item resistance", result.Sheet["resistances"])
	}
	active := []any{
		Object{"itemId": "guidance", "name": "Guidance", "ref": Object{"kind": "magic-item", "id": "card-guidance"}},
		Object{"itemId": "power", "name": "Power", "ref": Object{"kind": "magic-item", "id": "card-power"}},
		Object{"itemId": "sky", "name": "Sky", "ref": Object{"kind": "magic-item", "id": "card-sky"}},
		Object{"itemId": "glow", "name": "Glow", "ref": Object{"kind": "magic-item", "id": "card-glow"}},
	}
	if !reflect.DeepEqual(result.Sheet["activeItems"], active) {
		t.Fatal("active items", result.Sheet["activeItems"])
	}
	if again := EvaluateCharacter(result.Inputs, records, profile); !reflect.DeepEqual(again, EvaluateCharacter(result.Inputs, records, profile)) {
		t.Fatal("item grants are not deterministic")
	}
}

func TestItemGrantsApplyTheFirstEquippedCopyOnly(t *testing.T) {
	input, records, profile := itemGrantFixture(t)
	input.Play.Inventory = []character.Item{itemCard("spare", "Spare", "card-guidance"), itemCard("guidance", "Guidance", "card-guidance")}
	result := evaluateReady(t, input, records, profile)
	first, spare := findPlayResource(Object(result.Sheet), "item:guidance:uses"), findPlayResource(Object(result.Sheet), "item:spare:uses")
	if first == nil || first["active"] != nil || spare == nil || spare["active"] != false {
		t.Fatal("copy selection", result.Sheet["resources"])
	}
	if len(objects(result.Sheet["advantages"])) != 1 || len(objects(object(result.Sheet["spellcasting"])["granted"])) != 1 {
		t.Fatal("a second copy stacked", result.Sheet["advantages"])
	}
	if row := conditionRow(result, "marked"); integer(row["level"], 0) != 1 || len(objects(row["sources"])) != 1 {
		t.Fatal("a second copy counted", row)
	}
	if items := objects(result.Sheet["activeItems"]); len(items) != 1 || text(items[0]["itemId"]) != "guidance" {
		t.Fatal("active copy", items)
	}
	input.Play.Inventory[1].Location = "carried"
	result = evaluateReady(t, input, records, profile)
	if spare := findPlayResource(Object(result.Sheet), "item:spare:uses"); spare == nil || spare["active"] != nil {
		t.Fatal("the remaining copy did not apply", result.Sheet["resources"])
	}
}

func TestItemSpellUsesRecoverOnRestsWhileUnequipped(t *testing.T) {
	input, records, profile := itemGrantFixture(t)
	result := withCastingAbility(t, input, records, profile)
	cast := playReady(t, result.Inputs, Object{"operation": "cast-granted-spell", "key": "item:guidance:blessing", "slot": "item:guidance:uses"}, records, profile)
	if cast.Inputs.Play.ResourceUses["item:guidance:uses"] != 1 || cast.Inputs.Play.ResourceUses["slot-1"] != 0 {
		t.Fatal("cast did not spend the item's use", cast.Inputs.Play.ResourceUses)
	}
	if _, err := ApplyCharacterPlay(cast.Inputs, Object{"operation": "cast-granted-spell", "key": "item:guidance:blessing", "slot": "item:guidance:uses"}, records, profile); err == nil {
		t.Fatal("cast without a remaining use")
	}
	stowed := cloneCharacter(cast.Inputs)
	stowed.Play.Inventory[0].Location = "carried"
	result = evaluateReady(t, stowed, records, profile)
	resource := findPlayResource(Object(result.Sheet), "item:guidance:uses")
	if resource == nil || resource["active"] != false || integer(resource["remaining"], -1) != 0 || grantedOption(result, "blessing") != nil {
		t.Fatal("an unequipped item lost its spent use or kept its spell", resource)
	}
	rested := playReady(t, stowed, Object{"operation": "rest", "rest": "long"}, records, profile)
	if rested.Inputs.Play.ResourceUses["item:guidance:uses"] != 0 || rested.Inputs.Build.Spells.CastingAbilities["item:guidance:casting"] != "WIS" {
		t.Fatal("a long rest did not restore the unequipped item's use", rested.Inputs.Play.ResourceUses)
	}
	rested.Inputs.Play.Inventory[0].Location = "equipped"
	if slots := stringsOf(grantedOption(evaluateReady(t, rested.Inputs, records, profile), "blessing")["slots"]); !reflect.DeepEqual(slots, []string{"item:guidance:uses"}) {
		t.Fatal("re-equipped item spell", slots)
	}
}

func TestItemSpellsNeverUseSpellSlots(t *testing.T) {
	input, records, profile := itemGrantFixture(t)
	pending := evaluateReady(t, input, records, profile)
	if findPlayResource(Object(pending.Sheet), "slot-1") == nil {
		t.Fatal("fixture has no spell slots")
	}
	if slots := stringsOf(grantedOption(pending, "blessing")["slots"]); len(slots) != 0 {
		t.Fatal("an item spell is castable before its casting ability is picked", slots)
	}
	if _, err := ApplyCharacterPlay(input, Object{"operation": "cast-granted-spell", "key": "item:guidance:blessing", "slot": "item:guidance:uses"}, records, profile); err == nil {
		t.Fatal("cast before the casting ability pick")
	}
	result := withCastingAbility(t, input, records, profile)
	if slots := stringsOf(grantedOption(result, "blessing")["slots"]); !reflect.DeepEqual(slots, []string{"item:guidance:uses"}) {
		t.Fatal("item spell cast options", slots)
	}
	for _, slot := range []string{"slot-1", ""} {
		if _, err := ApplyCharacterPlay(result.Inputs, Object{"operation": "cast-granted-spell", "key": "item:guidance:blessing", "slot": slot}, records, profile); err == nil {
			t.Fatalf("cast an item spell with %q", slot)
		}
	}
	records.byKind["magic-item"]["card-guidance"] = mustJSON(Object{"kind": "magic-item", "id": "card-guidance", "name": "Guidance Card", "characterMechanics": "complete",
		"grants": Object{"spells": []any{Object{"ids": []any{"blessing"}, "atWill": true}}}})
	result = evaluateReady(t, input, records, profile)
	if slots := stringsOf(grantedOption(result, "blessing")["slots"]); !reflect.DeepEqual(slots, []string{""}) {
		t.Fatal("at-will item spell offered slots", slots)
	}
	playReady(t, input, Object{"operation": "cast-granted-spell", "key": "item:guidance:blessing", "slot": ""}, records, profile)
}

func TestItemCantripsCastFreelyAndPicksOnlyWarn(t *testing.T) {
	input, records, profile := itemGrantFixture(t)
	result := evaluateReady(t, input, records, profile)
	if slots := stringsOf(grantedOption(result, "glow")["slots"]); !reflect.DeepEqual(slots, []string{""}) {
		t.Fatal("item cantrip cast options", slots)
	}
	for _, id := range []string{"choice:item:power:cantrip", "choice:item:guidance:casting"} {
		found := false
		for _, issue := range result.Issues {
			found = found || issue.ID == id && issue.Severity == "warning"
		}
		if !found {
			t.Fatal("item pick is not a warning", id, result.Issues)
		}
	}
	var labels []string
	for _, section := range objects(result.Guidance["sections"]) {
		for _, issue := range objects(section["issues"]) {
			if text(issue["severity"]) == "warning" {
				labels = append(labels, text(issue["label"]))
			}
		}
	}
	if !reflect.DeepEqual(labels, []string{"Choose spells: Power", "Choose casting ability: Guidance"}) {
		t.Fatal("item pick labels", labels)
	}
	var choice Object
	for _, candidate := range objects(result.SpellOptions["pendingChoices"]) {
		if text(candidate["key"]) == "item:power:cantrip" {
			choice = candidate
		}
	}
	if !reflect.DeepEqual(stringsOf(choice["eligibleSpellIds"]), []string{"glow", "spark"}) {
		t.Fatal("cantrip pick options", choice)
	}
	picked := playReady(t, input, Object{"operation": "select-grant-spell", "key": "item:power:cantrip", "ref": "spark", "selected": true}, records, profile)
	if spark := grantedOption(picked, "spark"); spark == nil || !reflect.DeepEqual(stringsOf(spark["slots"]), []string{""}) {
		t.Fatal("picked item cantrip", spark)
	}
	cast := playReady(t, picked.Inputs, Object{"operation": "cast-granted-spell", "key": "item:power:spark", "slot": ""}, records, profile)
	if !reflect.DeepEqual(cast.Inputs.Play.ResourceUses, picked.Inputs.Play.ResourceUses) {
		t.Fatal("a cantrip spent a resource", cast.Inputs.Play.ResourceUses)
	}
	if _, err := ApplyCharacterPlay(picked.Inputs, Object{"operation": "cast-granted-spell", "key": "item:power:spark", "slot": "slot-1"}, records, profile); err == nil {
		t.Fatal("an item cantrip spent a spell slot")
	}
}

func TestSenseBonusExtendsOrGrantsASense(t *testing.T) {
	for _, test := range []struct {
		species  string
		on, off  int
		hasSense bool
	}{{"dwarf", 150, 120, true}, {"human", 60, 0, false}} {
		t.Run(test.species, func(t *testing.T) {
			input, records, profile := itemGrantFixture(t)
			input.Build.Species = test.species
			off := evaluateReady(t, input, records, profile)
			if _, exists := object(off.Sheet["senses"])["darkvision"]; exists != test.hasSense || integer(object(off.Sheet["senses"])["darkvision"], 0) != test.off {
				t.Fatal("darkvision before the toggle", off.Sheet["senses"])
			}
			on := playReady(t, input, Object{"operation": "toggle-feature", "key": "item:sky:open-sky", "enabled": true}, records, profile)
			if got := integer(object(on.Sheet["senses"])["darkvision"], 0); got != test.on {
				t.Fatalf("darkvision = %d, want %d", got, test.on)
			}
			terms := on.Explanations["senses.darkvision"].Terms
			applied := 0
			for _, term := range terms {
				if term.Source != nil && *term.Source == (character.Reference{Kind: "magic-item", ID: "card-sky"}) && term.Status == "applied" {
					applied++
				}
			}
			if applied != 2 {
				t.Fatal("sense explanation does not name the item", terms)
			}
		})
	}
	sheet := Object{"senses": Object{}, "derived": Object{}, "proficiencies": Object{}}
	applyGenericGrants(Object{}, sheet, nil, []Object{{"target": "senseBonus", "key": "blindsight", "add": 10}, {"target": "senseBonus", "key": "blindsight", "add": 5}})
	if integer(object(sheet["senses"])["blindsight"], 0) != 15 {
		t.Fatal("feature senseBonus", sheet["senses"])
	}
}

func TestUnequippedItemsKeepTheirStateWithoutBlockers(t *testing.T) {
	input, records, profile := itemGrantFixture(t)
	input.Build.Spells.CastingAbilities = map[string]string{"item:guidance:casting": "WIS"}
	input.Build.Spells.GrantChoices = map[string][]string{"item:power:cantrip": {"spark"}}
	input.Play.ResourceUses = map[string]int{"item:guidance:uses": 1}
	input.Play.ActiveFeatures = map[string]bool{"item:sky:open-sky": true}
	equipped := evaluateReady(t, input, records, profile)
	if integer(object(equipped.Sheet["senses"])["darkvision"], 0) != 150 || grantedOption(equipped, "spark") == nil {
		t.Fatal("saved item state did not apply", equipped.Sheet["senses"])
	}
	for index := range input.Play.Inventory {
		input.Play.Inventory[index].Location = "carried"
	}
	stowed := evaluateReady(t, input, records, profile)
	if !reflect.DeepEqual(stowed.Inputs, input) || integer(object(stowed.Sheet["senses"])["darkvision"], 0) != 120 || grantedOption(stowed, "spark") != nil || len(objects(stowed.Sheet["activeItems"])) != 0 || conditionRow(stowed, "marked") != nil {
		t.Fatal("unequipped item state changed or still applies", stowed.Sheet["senses"])
	}
	for _, issue := range stowed.Issues {
		if issue.Severity == "blocker" {
			t.Fatal("unequipped item state blocks", issue)
		}
	}
	// Saved state of an item no longer owned still needs repair.
	input.Play.Inventory = input.Play.Inventory[1:]
	if result := EvaluateCharacter(input, records, profile); !hasChoiceIssue(result, "casting-ability:item:guidance:casting") || !hasChoiceIssue(result, "resource:item:guidance:uses") {
		t.Fatal("removed item state was accepted", result.Issues)
	}
}

func TestStowingAHandItemSuspendsItsGrants(t *testing.T) {
	input, records, profile := handsFixture(t)
	records.byKind["magic-item"]["guard"] = mustJSON(Object{"kind": "magic-item", "id": "guard", "name": "Guard", "armorType": "shield", "acBonus": 2, "attunement": true, "characterMechanics": "complete",
		"grants": Object{"resistances": []any{"force"}, "resources": []any{Object{"key": "parry", "fixed": 1}},
			"activations": []any{Object{"id": "ward", "name": "Ward", "modifiers": []any{Object{"target": "resistance", "value": "thunder"}}}}}})
	input.Play.ActiveFeatures = map[string]bool{"item:off-copy:ward": true}
	input.Play.ResourceUses = map[string]int{"item:off-copy:parry": 1}
	start := evaluateReady(t, input, records, profile)
	if resistances := stringsOf(start.Sheet["resistances"]); !contains(resistances, "force") || !contains(resistances, "thunder") {
		t.Fatal("held item grants", resistances)
	}
	two := playReady(t, input, Object{"operation": "set-grip", "grip": "two"}, records, profile)
	if resistances := stringsOf(two.Sheet["resistances"]); contains(resistances, "force") || contains(resistances, "thunder") || findPlayResource(Object(two.Sheet), "item:off-copy:parry")["active"] != false {
		t.Fatal("a suspended item still grants", resistances)
	}
	if !reflect.DeepEqual(two.Inputs.Play.ActiveFeatures, input.Play.ActiveFeatures) || two.Inputs.Play.ResourceUses["item:off-copy:parry"] != 1 {
		t.Fatal("suspension dropped the item's state", two.Inputs.Play)
	}
	one := playReady(t, two.Inputs, Object{"operation": "set-grip", "grip": "one"}, records, profile)
	if !contains(stringsOf(one.Sheet["resistances"]), "thunder") {
		t.Fatal("restored item lost its toggle", one.Sheet["resistances"])
	}
}

func TestConsumingAnItemsLastUnitClearsItsState(t *testing.T) {
	input, records, profile := itemGrantFixture(t)
	records.byKind["magic-item"]["card-all"] = mustJSON(Object{"kind": "magic-item", "id": "card-all", "name": "All Card", "characterMechanics": "complete", "grants": Object{
		"castingAbility": Object{"id": "casting", "choose": []any{"WIS"}},
		"resources":      []any{Object{"key": "uses", "fixed": 1}},
		"spells":         []any{Object{"id": "cantrip", "choose": 1, "spellLevel": 0}},
		"activations":    []any{Object{"id": "shine", "name": "Shine"}},
	}})
	// Instance IDs may contain colons; "card:two" owns its own keys.
	input.Play.Inventory = []character.Item{itemCard("card", "Card", "card-all"), itemCard("card:two", "Second card", "card-all")}
	input.Build.Spells.CastingAbilities = map[string]string{"item:card:casting": "WIS"}
	input.Build.Spells.GrantChoices = map[string][]string{"item:card:cantrip": {"spark"}}
	input.Play.ActiveFeatures = map[string]bool{"item:card:shine": true}
	input.Play.ResourceUses = map[string]int{"item:card:uses": 1, "item:card:two:uses": 1}
	evaluateReady(t, input, records, profile)
	before := cloneCharacter(input)
	result := playReady(t, input, Object{"operation": "consume-item", "itemId": "card"}, records, profile)
	if !reflect.DeepEqual(input, before) {
		t.Fatal("consumption changed the caller's input")
	}
	spells, play := result.Inputs.Build.Spells, result.Inputs.Play
	if len(spells.CastingAbilities) != 0 || len(spells.GrantChoices) != 0 || len(play.ActiveFeatures) != 0 ||
		!reflect.DeepEqual(play.ResourceUses, map[string]int{"item:card:two:uses": 1}) {
		t.Fatal("consumed item state", spells, play.ResourceUses, play.ActiveFeatures)
	}
	if play.Inventory[0].Quantity != 0 || play.Inventory[0].Location != "carried" {
		t.Fatal("consumed item", play.Inventory[0])
	}
	if items := objects(result.Sheet["activeItems"]); len(items) != 1 || text(items[0]["itemId"]) != "card:two" {
		t.Fatal("the remaining copy did not apply", items)
	}
}

func TestItemSpellsDoNotMeetAttunementPrerequisites(t *testing.T) {
	input, records, profile := itemGrantFixture(t)
	input.Build.Levels = []character.Level{{ID: "one", ClassID: "fighter"}}
	records.byKind["magic-item"]["focus"] = mustJSON(Object{"kind": "magic-item", "id": "focus", "name": "Focus", "attunement": true,
		"attunementPrerequisites": Object{"spellcaster": true}, "characterMechanics": "complete"})
	focus := itemCard("focus", "Focus", "focus")
	focus.Attuned = true
	input.Play.Inventory = []character.Item{itemCard("glow", "Glow", "card-glow"), focus}
	result := EvaluateCharacter(input, records, profile)
	if grantedOption(result, "glow") == nil || !hasChoiceIssue(result, "attunement:focus") {
		t.Fatal("an item cantrip qualified for attunement", result.Issues)
	}
}

func TestItemGrantExplanationsNameTheItemRecord(t *testing.T) {
	input, records, profile := itemGrantFixture(t)
	result := evaluateReady(t, input, records, profile)
	ref := character.Reference{Kind: "magic-item", ID: "card-guidance"}
	if sources := result.Explanations["resources.item:guidance:uses.max"].Sources; !reflect.DeepEqual(sources, []character.Reference{ref}) {
		t.Fatal("resource explanation sources", sources)
	}
	found := false
	for _, entry := range result.Evidence {
		found = found || entry.Reference == ref
	}
	if !found {
		t.Fatal("item record is not evidence")
	}
	cards := []character.Reference{}
	for _, id := range []string{"card-glow", "card-guidance", "card-power", "card-sky"} {
		cards = append(cards, character.Reference{Kind: "magic-item", ID: id})
	}
	if sources := result.Explanations["conditions"].Sources; !reflect.DeepEqual(sources, cards) {
		t.Fatal("condition explanation sources", sources)
	}
}

func TestItemConditionsCountActiveItems(t *testing.T) {
	input, records, profile := itemGrantFixture(t)
	input.Play.Conditions = []character.Condition{{ID: "marked", Level: 1}}
	result := EvaluateCharacter(input, records, profile)
	row := conditionRow(result, "marked")
	sources := objects(row["sources"])
	if integer(row["level"], 0) != 4 || text(row["name"]) != "Marked" || text(row["summary"]) != "Fate watches the bearer." || text(row["status"]) != "active" || len(sources) != 4 || text(sources[2]["id"]) != "sky" {
		t.Fatal("granted condition", row)
	}
	if rows := objects(result.Sheet["conditions"]); len(rows) != 2 || truth(rows[0]["granted"]) || !reflect.DeepEqual(result.Inputs.Play.Conditions, input.Play.Conditions) {
		t.Fatal("granted condition replaced authored tracking", rows)
	}
	input.Play.Conditions = nil
	input.Play.Inventory[0].Location, input.Play.Inventory[1].Location = "carried", "carried"
	row = conditionRow(evaluateReady(t, input, records, profile), "marked")
	if integer(row["level"], 0) != 2 || text(row["name"]) != "Marked by the sky" {
		t.Fatal("the first active item does not name the condition", row)
	}
	records.byKind["magic-item"]["calm"] = mustJSON(Object{"kind": "magic-item", "id": "calm", "name": "Calm", "characterMechanics": "complete",
		"grants": Object{"conditionImmunities": []any{"marked"}}})
	input.Play.Inventory = append(input.Play.Inventory, itemCard("calm", "Calm", "calm"))
	if row = conditionRow(evaluateReady(t, input, records, profile), "marked"); text(row["status"]) != "immune" || integer(row["level"], 0) != 2 {
		t.Fatal("condition immunity", row)
	}
}

func TestItemTogglesDoNotTriggerFeatureExtensions(t *testing.T) {
	input, records, profile := itemGrantFixture(t)
	records.byKind["feature"] = map[string]json.RawMessage{"singer-echo": mustJSON(Object{"kind": "feature", "id": "singer-echo", "name": "Echo", "classId": "singer", "level": 1,
		"activations": []any{Object{"id": "echo", "extends": "open-sky", "modifiers": []any{Object{"target": "resistance", "value": "thunder"}}}}})}
	on := playReady(t, input, Object{"operation": "toggle-feature", "key": "item:sky:open-sky", "enabled": true}, records, profile)
	if integer(object(on.Sheet["senses"])["darkvision"], 0) != 150 || contains(stringsOf(on.Sheet["resistances"]), "thunder") {
		t.Fatal("an item toggle triggered a feature extension", on.Sheet["resistances"])
	}
}
