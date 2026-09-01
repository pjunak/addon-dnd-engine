package rules

import (
	"encoding/json"
	"testing"
)

func TestHydrateCoreCharacterMath(t *testing.T) {
	t.Parallel()
	profile := syntheticRuleset(t)
	result := Hydrate(Object{
		"abilities": Object{"INT": 16, "CON": 14},
		"className": "Wizard", "level": 5,
	}, syntheticRecords(), &profile)
	if len(result.Warnings) != 0 {
		t.Fatalf("warnings = %v", result.Warnings)
	}
	if text(object(result.Sheet["class"])["id"]) != "wizard" ||
		integer(object(result.Sheet["derived"])["maxHp"], 0) != 32 ||
		!truth(object(object(result.Sheet["saves"])["INT"])["proficient"]) {
		t.Fatalf("sheet = %+v", result.Sheet)
	}
	spellcasting := object(result.Sheet["spellcasting"])
	if slots := integersOf(spellcasting["slots"]); len(slots) != 3 || slots[2] != 2 {
		t.Fatalf("slots = %v", slots)
	}
	perClass := object(values(spellcasting["perClass"])[0])
	if integer(perClass["saveDC"], 0) != 14 {
		t.Fatalf("caster = %+v", perClass)
	}
}

func TestHydrateKeepsPactMagicSeparate(t *testing.T) {
	t.Parallel()
	profile := syntheticRuleset(t)
	result := Hydrate(Object{
		"classes": []any{Object{"classId": "warlock", "level": 5}},
	}, syntheticRecords(), &profile)
	spellcasting := object(result.Sheet["spellcasting"])
	if len(integersOf(spellcasting["slots"])) != 0 {
		t.Fatalf("slots = %v", spellcasting["slots"])
	}
	pact := object(object(values(spellcasting["perClass"])[0])["pact"])
	if integer(pact["slots"], 0) != 2 || integer(pact["level"], 0) != 3 {
		t.Fatalf("pact = %+v", pact)
	}
	found := false
	for _, raw := range values(result.Sheet["resources"]) {
		resource := object(raw)
		if text(resource["key"]) == "pact-slot" {
			found = integer(resource["max"], 0) == 2 &&
				text(object(values(resource["recharge"])[0])["on"]) == "short"
		}
	}
	if !found {
		t.Fatalf("resources = %+v", result.Sheet["resources"])
	}
}

func TestHydrateUsesMulticlassOrderAndProfileRounding(t *testing.T) {
	t.Parallel()
	profile := syntheticRuleset(t)
	profile.Constants.CasterFractions.Half = "down"
	wizardFirst := Hydrate(Object{"classes": []any{
		Object{"classId": "wizard", "level": 3}, Object{"classId": "fighter", "level": 1},
	}}, syntheticRecords(), &profile).Sheet
	fighterFirst := Hydrate(Object{"classes": []any{
		Object{"classId": "fighter", "level": 1}, Object{"classId": "wizard", "level": 3},
	}}, syntheticRecords(), &profile).Sheet
	if !truth(object(object(wizardFirst["saves"])["INT"])["proficient"]) ||
		!truth(object(object(fighterFirst["saves"])["STR"])["proficient"]) {
		t.Fatal("origin class did not control saving throw proficiency")
	}
	multiclass := Hydrate(Object{"classes": []any{
		Object{"classId": "paladin", "level": 5}, Object{"classId": "sorcerer", "level": 1},
	}}, syntheticRecords(), &profile).Sheet
	slots := integersOf(object(multiclass["spellcasting"])["slots"])
	if len(slots) != 2 || slots[0] != 4 || slots[1] != 2 {
		t.Fatalf("slots = %v", slots)
	}
}

func TestHydrateInterpretsGenericSpeciesAndWeaponRecords(t *testing.T) {
	t.Parallel()
	profile := syntheticRuleset(t)
	result := Hydrate(Object{
		"abilities": Object{"STR": 16, "DEX": 14, "CON": 14},
		"className": "Barbarian", "level": 5, "race": "Dwarf",
		"inventory": []any{Object{"ref": "longsword", "location": "equipped"}},
	}, syntheticRecords(), &profile).Sheet
	if integer(object(result["derived"])["maxHp"], 0) != 55 ||
		integer(object(result["senses"])["darkvision"], 0) != 120 ||
		!contains(stringsOf(result["resistances"]), "poison") || len(values(result["weapons"])) != 1 {
		t.Fatalf("sheet = %+v", result)
	}
}

func TestHydrateWithoutProviderKeepsUniversalMath(t *testing.T) {
	t.Parallel()
	result := Hydrate(Object{
		"abilities": Object{"STR": 16, "DEX": 14}, "level": 5,
	}, nil, nil)
	if AbilityModifier(number(object(object(result.Sheet["abilities"])["STR"])["score"], 0)) != 3 ||
		integer(object(result.Sheet["derived"])["proficiencyBonus"], 0) != 3 ||
		integer(object(result.Sheet["derived"])["initiative"], 0) != 2 || len(result.Warnings) != 1 {
		t.Fatalf("result = %+v", result)
	}
}

func TestHydrateAppliesGenericChoicePackagesAndActiveModifiers(t *testing.T) {
	t.Parallel()
	profile := syntheticRuleset(t)
	result := Hydrate(Object{
		"abilities": Object{"DEX": 14, "CON": 14},
		"classes":   []any{Object{"classId": "fighter", "level": 3}},
		"feats":     []any{Object{"featId": "guardian"}, Object{"featId": "adaptable"}},
		"featureChoices": Object{
			"feat:adaptable:heritage": "stone",
		},
		"activeFeatures": Object{"feat:guardian:stance": true},
	}, syntheticRecords(), &profile).Sheet
	if integer(object(result["derived"])["armorClass"], 0) != 13 ||
		integer(result["speed"], 0) != 35 || integer(result["flySpeed"], 0) != 35 ||
		integer(result["concentrationSaveBonus"], 0) != 2 ||
		!contains(stringsOf(result["languages"]), "giant") ||
		!contains(stringsOf(result["languages"]), "dwarvish") ||
		!contains(stringsOf(result["resistances"]), "fire") ||
		!contains(stringsOf(result["resistances"]), "cold") {
		t.Fatalf("sheet = %+v", result)
	}
	activation := object(values(result["activations"])[0])
	if !truth(activation["active"]) || !truth(activation["available"]) {
		t.Fatalf("activation = %+v", activation)
	}
}

func TestHydrateMaterializesSpellGrantsResourcesAndFeatureIdentity(t *testing.T) {
	t.Parallel()
	profile := syntheticRuleset(t)
	result := Hydrate(Object{
		"abilities": Object{"INT": 16},
		"classes":   []any{Object{"classId": "wizard", "level": 2}},
		"feats":     []any{Object{"featId": "magic-initiate"}, Object{"featId": "guardian"}},
		"grantChoices": Object{
			"feat:magic-initiate:mi-cantrips": []any{"fire-bolt"},
			"feat:magic-initiate:mi-spell":    []any{"mage-armor"},
		},
	}, syntheticRecords(), &profile).Sheet
	spellcasting := object(result["spellcasting"])
	if len(values(spellcasting["granted"])) != 2 || len(values(spellcasting["pendingChoices"])) != 2 {
		t.Fatalf("spellcasting = %+v", spellcasting)
	}
	resources := values(result["resources"])
	if !hasResource(resources, "guardian-pool", 2) || !hasResource(resources, "charge-mage-armor", 1) {
		t.Fatalf("resources = %+v", resources)
	}
	features := values(result["features"])
	if !hasFeature(features, "wizard-arcane-recovery") || !hasFeature(features, "wizard-scholar") ||
		hasFeature(features, "wizard-spell-mastery") || hasFeature(features, "Spell Mastery") {
		t.Fatalf("features = %+v", features)
	}
}

type memoryRecords struct {
	byKind map[string]map[string]json.RawMessage
}

func (records memoryRecords) Value(kind, id string) (json.RawMessage, bool) {
	value, exists := records.byKind[kind][id]
	return append(json.RawMessage(nil), value...), exists
}

func (records memoryRecords) ValueByName(kind, name string) (json.RawMessage, bool) {
	for _, value := range records.byKind[kind] {
		current, _ := DecodeObject(value)
		if text(current["name"]) == name {
			return append(json.RawMessage(nil), value...), true
		}
	}
	return nil, false
}

func (records memoryRecords) Values(kind string) []json.RawMessage {
	result := make([]json.RawMessage, 0, len(records.byKind[kind]))
	for _, value := range records.byKind[kind] {
		result = append(result, append(json.RawMessage(nil), value...))
	}
	return result
}

func syntheticRecords() memoryRecords {
	return newMemoryRecords([]Object{
		{"kind": "class", "id": "wizard", "name": "Wizard", "hitDie": "d6", "savingThrows": []any{"INT", "WIS"},
			"spellcasting": Object{"ability": "INT", "type": "full", "prepares": "spellbook", "ritual": true},
			"progression": []any{
				Object{"level": 1, "spellSlots": []any{2}, "features": []any{"Arcane Recovery"}},
				Object{"level": 2, "spellSlots": []any{3}, "features": []any{"Scholar", "Spell Mastery"}},
				Object{"level": 5, "preparedSpells": 9, "cantripsKnown": 4, "spellSlots": []any{4, 3, 2}},
			}},
		{"kind": "class", "id": "warlock", "name": "Warlock", "hitDie": "d8", "savingThrows": []any{"WIS", "CHA"},
			"spellcasting": Object{"ability": "CHA", "type": "pact", "prepares": "list"},
			"progression":  []any{Object{"level": 1, "preparedSpells": 2}, Object{"level": 5, "preparedSpells": 6}}},
		{"kind": "class", "id": "fighter", "name": "Fighter", "hitDie": "d10", "savingThrows": []any{"STR", "CON"}},
		{"kind": "class", "id": "paladin", "name": "Paladin", "hitDie": "d10", "savingThrows": []any{"WIS", "CHA"},
			"spellcasting": Object{"ability": "CHA", "type": "half", "prepares": "list"}},
		{"kind": "class", "id": "sorcerer", "name": "Sorcerer", "hitDie": "d6", "savingThrows": []any{"CON", "CHA"},
			"spellcasting": Object{"ability": "CHA", "type": "full", "prepares": "list"}},
		{"kind": "class", "id": "barbarian", "name": "Barbarian", "hitDie": "d12", "savingThrows": []any{"STR", "CON"}},
		{"kind": "species", "id": "dwarf", "name": "Dwarf", "speeds": Object{"walk": 30},
			"senses": Object{"darkvision": 120}, "resistances": []any{"poison"}, "grants": Object{"hpPerLevel": 1}},
		{"kind": "weapon", "id": "longsword", "name": "Longsword", "category": "martial", "range": "melee",
			"damage": "1d8", "damageType": "slashing", "properties": []any{"versatile"}, "versatileDamage": "1d10", "mastery": "Sap"},
		{"kind": "feat", "id": "guardian", "name": "Guardian", "grants": Object{
			"languages": []any{"giant"}, "resistances": []any{"fire"},
			"resources": []any{Object{"key": "guardian-pool", "name": "Guardian Pool", "fixed": 2, "recharge": "short"}},
			"activations": []any{Object{"id": "stance", "name": "Guardian Stance", "modifiers": []any{
				Object{"target": "armorClass", "add": 1}, Object{"target": "speed", "add": 5},
				Object{"target": "flySpeed", "value": "speed"}, Object{"target": "concentrationSave", "addAbility": "CON"},
			}}},
		}},
		{"kind": "feat", "id": "adaptable", "name": "Adaptable", "grants": Object{
			"choicePackages": []any{Object{"choiceId": "heritage", "options": Object{
				"stone": Object{"languages": []any{"dwarvish"}, "resistances": []any{"cold"}},
			}}},
		}},
		{"kind": "feat", "id": "magic-initiate", "name": "Magic Initiate", "grants": Object{
			"spells": []any{
				Object{"id": "mi-cantrips", "choose": 2, "spellLevel": 0, "from": Object{"class": []any{"wizard"}}, "alwaysPrepared": true},
				Object{"id": "mi-spell", "choose": 1, "spellLevel": 1, "from": Object{"class": []any{"wizard"}}, "alwaysPrepared": true, "free": "1/long"},
			},
		}},
		{"kind": "spell", "id": "fire-bolt", "name": "Fire Bolt", "level": 0, "school": "Evocation"},
		{"kind": "spell", "id": "mage-armor", "name": "Mage Armor", "level": 1, "school": "Abjuration"},
		{"kind": "feature", "id": "wizard-arcane-recovery", "name": "Arcane Recovery", "classId": "wizard", "level": 1},
		{"kind": "feature", "id": "wizard-scholar", "name": "Scholar", "classId": "wizard", "level": 2},
		{"kind": "feature", "id": "wizard-spell-mastery", "name": "Spell Mastery", "classId": "wizard", "level": 18},
	})
}

func hasResource(resources []any, key string, maximum int) bool {
	for _, raw := range resources {
		resource := object(raw)
		if text(resource["key"]) == key && integer(resource["max"], 0) == maximum {
			return true
		}
	}
	return false
}

func hasFeature(features []any, id string) bool {
	for _, raw := range features {
		if text(object(raw)["id"]) == id {
			return true
		}
	}
	return false
}

func newMemoryRecords(records []Object) memoryRecords {
	result := memoryRecords{byKind: make(map[string]map[string]json.RawMessage)}
	for _, record := range records {
		kind, id := text(record["kind"]), text(record["id"])
		if result.byKind[kind] == nil {
			result.byKind[kind] = make(map[string]json.RawMessage)
		}
		body, _ := json.Marshal(record)
		result.byKind[kind][id] = body
	}
	return result
}
