package rules

import (
	"reflect"
	"slices"
	"testing"

	"github.com/pjunak/addon-dnd-engine/character"
)

func magicGearFixture(t *testing.T) (character.Inputs, memoryRecords, Ruleset) {
	t.Helper()
	input, source, profile := characterFixture(t)
	records := source.(memoryRecords)
	weapon := func(kind string, ids ...any) Object { return Object{"kind": kind, "ids": ids} }
	for kind, entries := range newMemoryRecords([]Object{
		{"kind": "weapon", "id": "blade", "name": "Blade", "category": "martial", "range": "melee", "damage": "1d8", "damageType": "slashing", "properties": []any{"versatile"}, "versatileDamage": "1d10", "mastery": "Sap", "weightLb": 3},
		{"kind": "weapon", "id": "knife", "name": "Knife", "category": "simple", "range": "melee", "damage": "1d4", "damageType": "piercing", "properties": []any{"finesse", "light", "thrown"}, "weightLb": 1},
		{"kind": "weapon", "id": "bow", "name": "Bow", "category": "martial", "range": "ranged", "damage": "1d8", "damageType": "piercing", "properties": []any{"ammunition", "two-handed"}},
		{"kind": "armor", "id": "mail", "name": "Mail", "armorType": "heavy", "baseAC": 16, "dexCap": 0, "strReq": "Str 13", "stealthDisadvantage": true, "weightLb": 55, "bodyPlacements": []any{"body"}},
		{"kind": "armor", "id": "hide", "name": "Hide", "armorType": "medium", "baseAC": 12, "dexCap": 2, "weightLb": 12},
		{"kind": "armor", "id": "board", "name": "Board", "armorType": "shield", "acBonus": 2, "weightLb": 6},
		{"kind": "magic-item", "id": "plus-blade", "name": "+1 Blade", "characterMechanics": "complete", "baseItem": Object{"kind": "weapon", "ids": []any{"blade"}, "bonus": 1}},
		{"kind": "magic-item", "id": "any-weapon", "name": "Weapon, +1, +2, or +3", "characterMechanics": "complete", "baseItem": Object{"kind": "weapon", "categories": []any{"simple", "martial"}, "bonus": []any{3, 1, 2}}},
		{"kind": "magic-item", "id": "any-armor", "name": "Armor, +1, +2, or +3", "characterMechanics": "complete", "bodyPlacements": []any{"body"}, "baseItem": Object{"kind": "armor", "armorTypes": []any{"light", "medium", "heavy"}, "bonus": []any{1, 2, 3}}},
		{"kind": "magic-item", "id": "any-shield", "name": "Shield, +1, +2, or +3", "characterMechanics": "complete", "baseItem": Object{"kind": "armor", "armorTypes": []any{"shield"}, "bonus": []any{1, 2, 3}}},
		{"kind": "magic-item", "id": "plus-mail", "name": "+1 Mail", "characterMechanics": "complete", "weightLb": 40, "baseItem": Object{"kind": "armor", "ids": []any{"mail"}, "bonus": 1}},
		{"kind": "magic-item", "id": "bound-blade", "name": "Bound Blade", "attunement": true, "characterMechanics": "complete", "baseItem": Object{"kind": "weapon", "ids": []any{"blade"}, "bonus": 2}},
		{"kind": "magic-item", "id": "story-blade", "name": "Story Blade", "characterMechanics": "narrative", "baseItem": weapon("weapon", "blade")},
		{"kind": "magic-item", "id": "lost-blade", "name": "Lost Blade", "characterMechanics": "complete", "baseItem": Object{"kind": "weapon", "ids": []any{"missing"}, "bonus": 1}},
		{"kind": "magic-item", "id": "charm", "name": "Charm", "characterMechanics": "narrative"},
	}).byKind {
		records.byKind[kind] = entries
	}
	return input, records, profile
}

func gearItem(id, ref, location string) character.Item {
	return character.Item{ID: id, Reference: &character.Reference{Kind: "magic-item", ID: ref}, Name: id, Quantity: 1, Location: location}
}

func plainItem(id, kind, ref string) character.Item {
	return character.Item{ID: id, Reference: &character.Reference{Kind: kind, ID: ref}, Name: id, Quantity: 1, Location: "equipped"}
}

// weaponOf is the attack row made with an inventory instance, if any.
func weaponOf(result character.Result, itemID string) (Object, int) {
	for index, row := range objects(view(result.Sheet)["weapons"]) {
		if text(row["itemId"]) == itemID {
			return row, index
		}
	}
	return nil, -1
}

func severityOf(result character.Result, id string) character.IssueSeverity {
	for _, issue := range result.Issues {
		if issue.ID == id {
			return issue.Severity
		}
	}
	return ""
}

func TestMagicWeaponAttacksAsItsBaseWithItsBonus(t *testing.T) {
	input, records, profile := magicGearFixture(t)
	input.Play.Inventory = []character.Item{plainItem("plain", "weapon", "blade"), gearItem("magic", "plus-blade", "equipped")}
	original := cloneCharacter(input)
	result := EvaluateCharacter(input, records, profile)
	if !result.Ready {
		t.Fatal(result.Issues)
	}
	if !reflect.DeepEqual(input, original) || !reflect.DeepEqual(result.Inputs, original) {
		t.Fatal("evaluation changed the inputs")
	}
	plain, _ := weaponOf(result, "plain")
	magic, index := weaponOf(result, "magic")
	if magic == nil || text(magic["ref"]) != "blade" || text(magic["name"]) != "magic" || text(magic["mastery"]) != "Sap" {
		t.Fatal("the magic weapon is not its base", magic)
	}
	if integer(magic["attackBonus"], 0) != integer(plain["attackBonus"], 0)+1 || text(plain["damage"]) != "1d8 +2" || text(magic["damage"]) != "1d8 +3" || text(magic["versatileDamage"]) != "1d10 +3" {
		t.Fatal("the bonus is missing", plain, magic)
	}
	modifiers := objects(magic["modifiers"])
	source := object(modifiers[0]["source"])
	if len(modifiers) != 1 || integer(modifiers[0]["attackBonus"], 0) != 1 || integer(modifiers[0]["damageBonus"], 0) != 1 ||
		text(source["type"]) != "item" || text(source["id"]) != "magic" || text(object(source["ref"])["id"]) != "plus-blade" {
		t.Fatal("the bonus is not named by its item", modifiers)
	}
	explained := false
	for _, term := range result.Explanations[fmtIndex("weapons", index)+".attackBonus"].Terms {
		explained = explained || term.Source != nil && *term.Source == character.Reference{Kind: "magic-item", ID: "plus-blade"}
	}
	if !explained || !slices.ContainsFunc(result.Evidence, func(entry character.Evidence) bool {
		return entry.Reference == character.Reference{Kind: "weapon", ID: "blade"}
	}) {
		t.Fatal("the explanation or evidence omits the item or its base", result.Explanations[fmtIndex("weapons", index)+".attackBonus"])
	}

	// A two-handed grip uses the versatile die and keeps the bonus.
	input.Play.Inventory[0].Location = "carried"
	input.Play.Hands = &character.Hands{Main: "magic", Grip: character.GripTwo}
	result = EvaluateCharacter(input, records, profile)
	if magic, _ = weaponOf(result, "magic"); !result.Ready || text(magic["damage"]) != "1d10 +3" || text(magic["grip"]) != "two" {
		t.Fatal("a two-handed grip dropped the bonus", magic, result.Issues)
	}
}

func TestMagicWeaponWithSeveralBasesNeedsTheOwnersPicks(t *testing.T) {
	input, records, profile := magicGearFixture(t)
	input.Play.Inventory = []character.Item{plainItem("plain", "weapon", "knife"), gearItem("pick", "any-weapon", "equipped")}
	result := EvaluateCharacter(input, records, profile)
	guidance := object(object(view(result.Guidance)["equipment"])["pick"])
	labels := []string{}
	for _, option := range objects(guidance["baseItems"]) {
		labels = append(labels, text(option["label"]))
	}
	if !reflect.DeepEqual(labels, []string{"Blade", "Bow", "Knife"}) || !reflect.DeepEqual(integersOf(guidance["bonuses"]), []int{1, 2, 3}) {
		t.Fatal("the options are not listed by name", guidance)
	}
	if row, _ := weaponOf(result, "pick"); row != nil || !result.Ready || severityOf(result, "item-base:pick") != character.SeverityWarning || severityOf(result, "item-bonus:pick") != character.SeverityWarning {
		t.Fatal("an unpicked base must warn and give no attack", row, result.Issues)
	}
	if !truth(guidance["canEquip"]) {
		t.Fatal("missing picks must not stop equipping", guidance)
	}

	input.Play.Inventory[1].BaseItemID = "knife"
	result = EvaluateCharacter(input, records, profile)
	plain, _ := weaponOf(result, "plain")
	row, _ := weaponOf(result, "pick")
	if row == nil || text(row["damage"]) != text(plain["damage"]) || severityOf(result, "item-base:pick") != "" || severityOf(result, "item-bonus:pick") != character.SeverityWarning {
		t.Fatal("a picked base without a bonus is the plain base", row, result.Issues)
	}
	input.Play.Inventory[1].Bonus = 2
	result = EvaluateCharacter(input, records, profile)
	if row, _ = weaponOf(result, "pick"); integer(row["attackBonus"], 0) != integer(plain["attackBonus"], 0)+2 || len(result.Issues) != 0 {
		t.Fatal("the picked bonus is missing", row, result.Issues)
	}

	for _, invalid := range []character.Item{{BaseItemID: "mail"}, {BaseItemID: "knife", Bonus: 4}} {
		candidate := cloneCharacter(input)
		candidate.Play.Inventory[1].BaseItemID, candidate.Play.Inventory[1].Bonus = invalid.BaseItemID, invalid.Bonus
		result = EvaluateCharacter(candidate, records, profile)
		id := "item-base:pick"
		if invalid.Bonus != 0 {
			id = "item-bonus:pick"
		}
		equipment := object(object(view(result.Guidance)["equipment"])["pick"])
		if result.Ready || severityOf(result, id) != character.SeverityBlocker || truth(equipment["canEquip"]) || text(equipment["equipReason"]) != "mechanics" {
			t.Fatal("a pick the item does not offer was accepted", invalid, result.Issues, equipment)
		}
		// An item that is not in use keeps its picks unchecked.
		candidate.Play.Inventory[1].Location = "carried"
		if result = EvaluateCharacter(candidate, records, profile); severityOf(result, id) != "" {
			t.Fatal("a stowed item's picks were checked", result.Issues)
		}
	}
}

func TestMagicWeaponBonusNeedsAttunementAndBelongsToEachCopy(t *testing.T) {
	input, records, profile := magicGearFixture(t)
	input.Play.Inventory = []character.Item{plainItem("plain", "weapon", "blade"), gearItem("bound", "bound-blade", "equipped")}
	result := EvaluateCharacter(input, records, profile)
	plain, _ := weaponOf(result, "plain")
	bound, _ := weaponOf(result, "bound")
	if !result.Ready || bound == nil || bound["attackBonus"] != plain["attackBonus"] || len(objects(bound["modifiers"])) != 0 {
		t.Fatal("an unattuned item still adds its bonus", bound, result.Issues)
	}
	input.Play.Inventory[1].Attuned = true
	result = EvaluateCharacter(input, records, profile)
	if bound, _ = weaponOf(result, "bound"); integer(bound["attackBonus"], 0) != integer(plain["attackBonus"], 0)+2 {
		t.Fatal("an attuned item lacks its bonus", bound)
	}

	// Two copies of one record each add their own bonus to their own attacks.
	first, second := gearItem("first", "any-weapon", "equipped"), gearItem("second", "any-weapon", "equipped")
	first.BaseItemID, first.Bonus, second.BaseItemID, second.Bonus = "knife", 1, "blade", 3
	input.Play.Inventory = []character.Item{plainItem("plain", "weapon", "blade"), plainItem("dagger", "weapon", "knife"), first, second}
	result = EvaluateCharacter(input, records, profile)
	plainKnife, _ := weaponOf(result, "dagger")
	knife, _ := weaponOf(result, "first")
	blade, _ := weaponOf(result, "second")
	if !result.Ready || integer(knife["attackBonus"], 0) != integer(plainKnife["attackBonus"], 0)+1 || integer(blade["attackBonus"], 0) != integer(plain["attackBonus"], 0)+3 {
		t.Fatal("copies changed each other's bonus", knife, blade, result.Issues)
	}
}

func TestMagicArmorAndShieldAddTheirBonusToTheirBase(t *testing.T) {
	input, records, profile := magicGearFixture(t)
	input.Play.Inventory = []character.Item{plainItem("mail", "armor", "mail"), plainItem("board", "armor", "board")}
	plain := EvaluateCharacter(input, records, profile)
	armor, shield := gearItem("armor", "any-armor", "equipped"), gearItem("shield", "any-shield", "equipped")
	armor.BaseItemID, armor.Bonus, shield.Bonus = "mail", 1, 2
	input.Play.Inventory = []character.Item{armor, shield}
	result := EvaluateCharacter(input, records, profile)
	ac := object(view(result.Sheet)["ac"])
	if !result.Ready || integer(ac["value"], 0) != integer(object(view(plain.Sheet)["ac"])["value"], 0)+3 || text(ac["base"]) != "Mail" || integer(ac["shield"], 0) != 2 {
		t.Fatal("magic armor and shield do not add their bonus", ac, result.Issues)
	}
	bonuses := objects(ac["bonuses"])
	if len(bonuses) != 2 || text(bonuses[0]["name"]) != "armor" || integer(bonuses[0]["value"], 0) != 1 || text(object(bonuses[1]["source"])["id"]) != "shield" || integer(bonuses[1]["value"], 0) != 2 {
		t.Fatal("the AC bonuses are not named by their items", bonuses)
	}
	restrictions := object(view(result.Sheet)["armorRestrictions"])
	if text(restrictions["armorId"]) != "mail" || !truth(restrictions["stealthDisadvantage"]) {
		t.Fatal("the base armor's restrictions are missing", restrictions)
	}
	equipment := object(view(result.Guidance)["equipment"])
	if text(object(equipment["armor"])["slot"]) != "armor" || text(object(equipment["shield"])["slot"]) != "shield" || !reflect.DeepEqual(object(equipment["armor"])["bodyPlacements"], []any{"body"}) {
		t.Fatal("the slots are not the bases'", equipment)
	}
	if grips := handGrips(&input.Play.Inventory[1], records); !reflect.DeepEqual(grips, []character.Grip{character.GripOne}) {
		t.Fatal("a magic shield is held like a shield", grips)
	}
	decisions := Object{"inventory": []any{Object{"kind": "magic-item", "ref": "any-armor", "baseKind": "armor", "baseRef": "mail", "location": "equipped"}}}
	if state := equippedArmor(decisions, records); state.bodyType != "heavy" {
		t.Fatal("armor requirements do not see the base armor", state)
	}

	// An armor whose base is still to be picked keeps the slot its options share.
	armor.BaseItemID, armor.Bonus = "", 0
	input.Play.Inventory = []character.Item{plainItem("mail", "armor", "mail"), armor}
	result = EvaluateCharacter(input, records, profile)
	if !hasChoiceIssue(result, "equipped:armor") {
		t.Fatal("two body armors were accepted", result.Issues)
	}
	input.Play.Inventory = []character.Item{plainItem("board", "armor", "board"), shield}
	if result = EvaluateCharacter(input, records, profile); !hasChoiceIssue(result, "equipped:shield") {
		t.Fatal("two shields were accepted", result.Issues)
	}
}

func TestMagicGearWeightAndPlacementFallBackToTheBase(t *testing.T) {
	input, records, profile := magicGearFixture(t)
	profile.Constants.Carrying = &CarryingPolicy{CapacityPerStrength: 15, CoinsPerLb: 50}
	knife, mail, authored := gearItem("knife", "any-weapon", "carried"), gearItem("mail", "plus-mail", "carried"), gearItem("authored", "plus-blade", "carried")
	knife.BaseItemID = "knife"
	weight := 2.5
	authored.WeightLb = &weight
	input.Play.Inventory = []character.Item{knife, mail, authored, gearItem("unpicked", "any-weapon", "carried")}
	result := EvaluateCharacter(input, records, profile)
	carrying := object(view(result.Sheet)["carrying"])
	items := object(carrying["items"])
	if number(object(items["knife"])["eachLb"], 0) != 1 || number(object(items["mail"])["eachLb"], 0) != 40 || number(object(items["authored"])["eachLb"], 0) != 2.5 || !reflect.DeepEqual(carrying["unknown"], []any{"unpicked"}) {
		t.Fatal("weights do not fall back to the base", carrying)
	}
	if placements := object(object(view(result.Guidance)["equipment"])["mail"])["bodyPlacements"]; !reflect.DeepEqual(placements, []any{"body"}) {
		t.Fatal("placements do not fall back to the base", placements)
	}
}

func TestMagicGearDeclarationsAreReadAllOrNothing(t *testing.T) {
	input, records, profile := magicGearFixture(t)
	for _, declared := range []any{
		"blade", Object{"ids": []any{"blade"}}, Object{"kind": "sword"}, Object{"kind": "weapon", "ids": []any{}},
		Object{"kind": "weapon", "armorTypes": []any{"shield"}}, Object{"kind": "armor", "categories": []any{"simple"}},
		Object{"kind": "weapon", "ids": []any{"blade"}, "categories": []any{"martial"}}, Object{"kind": "weapon", "ids": []any{"blade", "blade"}},
		Object{"kind": "weapon", "bonus": 0}, Object{"kind": "weapon", "bonus": 11}, Object{"kind": "weapon", "bonus": []any{1, 1}}, Object{"kind": "weapon", "bonus": "1"},
		Object{"kind": "weapon", "ranges": []any{"far"}}, Object{"kind": "weapon", "extra": true},
	} {
		records.byKind["magic-item"]["odd"] = mustJSON(Object{"kind": "magic-item", "id": "odd", "name": "Odd", "characterMechanics": "complete", "baseItem": declared})
		input.Play.Inventory = []character.Item{gearItem("odd", "odd", "equipped")}
		result := EvaluateCharacter(input, records, profile)
		if row, _ := weaponOf(result, "odd"); row != nil || severityOf(result, "item-effects:odd") != character.SeverityBlocker {
			t.Fatal("a malformed baseItem was read", declared, row, result.Issues)
		}
	}
	// A narrative item is still its base weapon, but may not declare a bonus.
	input.Play.Inventory = []character.Item{gearItem("story", "story-blade", "equipped")}
	result := EvaluateCharacter(input, records, profile)
	if row, _ := weaponOf(result, "story"); !result.Ready || row == nil || text(row["ref"]) != "blade" || len(objects(row["modifiers"])) != 0 {
		t.Fatal("a narrative item is not its base", row, result.Issues)
	}
	records.byKind["magic-item"]["story-blade"] = mustJSON(Object{"kind": "magic-item", "id": "story-blade", "name": "Story Blade", "characterMechanics": "narrative", "baseItem": Object{"kind": "weapon", "ids": []any{"blade"}, "bonus": 1}})
	if result = EvaluateCharacter(input, records, profile); severityOf(result, "item-effects:story") != character.SeverityBlocker {
		t.Fatal("a narrative item declared a bonus", result.Issues)
	}
	// A base missing from the rules data warns; a pick on an item with no base blocks.
	charm := gearItem("charm", "charm", "equipped")
	charm.BaseItemID = "blade"
	input.Play.Inventory = []character.Item{gearItem("lost", "lost-blade", "equipped"), charm}
	result = EvaluateCharacter(input, records, profile)
	if row, _ := weaponOf(result, "lost"); row != nil || severityOf(result, "item-base:lost") != character.SeverityWarning || severityOf(result, "item-base:charm") != character.SeverityBlocker {
		t.Fatal("missing bases or stale picks were not reported", row, result.Issues)
	}
}
