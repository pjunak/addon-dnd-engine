package rules

import (
	"encoding/json"
	"testing"
)

func spellPlayRecords() memoryRecords {
	records := syntheticRecords()
	additional := newMemoryRecords([]Object{
		{"kind": "spell", "id": "spark", "name": "Spark", "level": 0, "school": "Evocation", "castingTime": "Action", "classes": []any{"wizard", "warlock"}},
		{"kind": "spell", "id": "ward", "name": "Ward", "level": 1, "school": "Abjuration", "castingTime": "Action", "classes": []any{"wizard", "warlock"}},
		{"kind": "spell", "id": "find-path", "name": "Find Path", "level": 1, "ritual": true, "school": "Divination", "castingTime": "1 minute", "classes": []any{"wizard", "warlock"}},
		{"kind": "spell", "id": "other", "name": "Other", "level": 3, "school": "Divination", "classes": []any{"cleric"}},
		{"kind": "feat", "id": "gift", "name": "Gift", "grants": Object{"castingAbility": Object{"choose": []any{"INT", "CHA"}}, "spells": []any{
			Object{"id": "gift-cantrip", "choose": 1, "spellLevel": 0, "from": Object{"class": []any{"wizard"}}, "default": "spark"},
			Object{"id": "gift-spell", "choose": 1, "spellLevel": 1, "from": Object{"school": []any{"Abjuration"}, "castingTime": []any{"Action"}}, "free": "1/long"},
		}}},
		{"kind": "feat", "id": "marked", "category": "mark", "grants": Object{"spellList": []any{"ward"}}},
		{"kind": "feat", "id": "special-slot", "grants": Object{"spellSlot": Object{"count": 1, "level": Object{"divisor": 1, "max": 9}, "restriction": "mark"}}},
	})
	for kind, values := range additional.byKind {
		if records.byKind[kind] == nil {
			records.byKind[kind] = map[string]json.RawMessage{}
		}
		for key, value := range values {
			records.byKind[kind][key] = value
		}
	}
	return records
}

func TestGrantedSpellsChooseAbilityAndUseFreeCast(t *testing.T) {
	records, profile := spellPlayRecords(), syntheticRuleset(t)
	state := Object{"classes": []any{Object{"classId": "wizard", "level": 5}}, "feats": []any{Object{"featId": "gift"}}, "notes": "keep", "currency": Object{"gp": 100}}
	apply := func(change Object) {
		t.Helper()
		next, err := ApplyPlayChange(state, change, records, profile)
		if err != nil {
			t.Fatal(err)
		}
		state = next
	}
	apply(Object{"operation": "select-grant-spell", "key": "feat:gift:gift-cantrip", "ref": "spark", "selected": false})
	hydrated := Hydrate(state, records, &profile).Sheet
	choice := objects(object(hydrated["spellcasting"])["pendingChoices"])[0]
	if len(stringsOf(choice["picked"])) != 0 {
		t.Fatal("Explicitly cleared default was selected again")
	}
	apply(Object{"operation": "select-grant-spell", "key": "feat:gift:gift-cantrip", "ref": "spark", "selected": true})
	apply(Object{"operation": "select-grant-spell", "key": "feat:gift:gift-spell", "ref": "ward", "selected": true})
	apply(Object{"operation": "select-casting-ability", "key": "feat:gift:casting-ability", "ability": "CHA"})
	apply(Object{"operation": "cast-granted-spell", "key": "feat:gift:ward", "slot": "charge-ward"})
	if integer(object(state["resourceUses"])["charge-ward"], -1) != 0 {
		t.Fatalf("Uses = %+v", state["resourceUses"])
	}
	before, _ := json.Marshal(state)
	if _, err := ApplyPlayChange(state, Object{"operation": "cast-granted-spell", "key": "feat:gift:ward", "slot": "charge-ward"}, records, profile); err == nil {
		t.Fatal("Repeated an exhausted free cast")
	}
	after, _ := json.Marshal(state)
	if string(before) != string(after) {
		t.Fatal("Rejected cast changed state")
	}
	apply(Object{"operation": "cast-granted-spell", "key": "feat:gift:ward", "slot": "slot-2"})
	apply(Object{"operation": "rest", "rest": "long"})
	if object(state["resourceUses"])["charge-ward"] != nil {
		t.Fatal("Long rest did not restore grant")
	}
	options := SpellOptions(state, Hydrate(state, records, &profile).Sheet, records, profile)
	for _, entry := range objects(options["pendingChoices"]) {
		if text(entry["key"]) == "feat:gift:gift-spell" && (contains(stringsOf(entry["eligibleSpellIds"]), "find-path") || !contains(stringsOf(entry["eligibleSpellIds"]), "ward")) {
			t.Fatalf("Eligibility = %+v", entry)
		}
	}
	if text(object(state["grantCastingAbilities"])["feat:gift:casting-ability"]) != "CHA" || text(state["notes"]) != "keep" {
		t.Fatal("Lost authored state")
	}
	if _, err := ApplyPlayChange(state, Object{"operation": "select-casting-ability", "key": "feat:gift:casting-ability", "ability": "STR"}, records, profile); err == nil {
		t.Fatal("Accepted unavailable casting ability")
	}
}

func TestCopyingSpellsAndRitualsAreAtomicAndPreserveInventory(t *testing.T) {
	records, profile := spellPlayRecords(), syntheticRuleset(t)
	cost := ScrollCopyCost(1, profile)
	state := Object{"classes": []any{Object{"classId": "wizard", "level": 5}}, "currency": Object{"gp": cost + 7, "sp": 11}, "inventory": []any{Object{"id": "scroll", "name": "Scroll of Find Path", "qty": 2, "notes": "retain"}, Object{"id": "other", "name": "Ward shield", "qty": 1}}}
	change := Object{"operation": "copy-spell", "classId": "wizard", "ref": "find-path", "scrollId": "scroll"}
	next, err := ApplyPlayChange(state, change, records, profile)
	if err != nil {
		t.Fatal(err)
	}
	if number(object(next["currency"])["gp"], 0) != 7 || integer(objects(next["inventory"])[0]["qty"], 0) != 1 || len(stringsOf(object(next["spellbook"])["wizard"])) != 1 {
		t.Fatalf("Copy = %+v", next)
	}
	if integer(objects(state["inventory"])[0]["qty"], 0) != 2 || text(objects(next["inventory"])[0]["notes"]) != "retain" {
		t.Fatal("Changed input or scroll notes")
	}
	if _, err = ApplyPlayChange(next, change, records, profile); err == nil {
		t.Fatal("Charged twice for an already learned spell")
	}
	ritual, err := ApplyPlayChange(next, Object{"operation": "cast-ritual", "classId": "wizard", "ref": "find-path"}, records, profile)
	if err != nil {
		t.Fatal(err)
	}
	if len(object(ritual["resourceUses"])) != 0 {
		t.Fatal("Ritual consumed a slot")
	}
	for _, change := range []Object{
		{"operation": "copy-spell", "classId": "wizard", "ref": "ward", "scrollId": "other"},
		{"operation": "copy-spell", "classId": "wizard", "ref": "other", "scrollId": ""},
		{"operation": "cast-ritual", "classId": "wizard", "ref": "ward"},
	} {
		if _, err := ApplyPlayChange(state, change, records, profile); err == nil {
			t.Fatalf("Accepted %+v", change)
		}
	}
	poor := cloneObjectDeep(state)
	object(poor["currency"])["gp"] = cost - 1
	if _, err = ApplyPlayChange(poor, change, records, profile); err == nil {
		t.Fatal("Allowed copying without enough GP")
	}
}

func TestLevelUpSpellSwapAndRestrictedSlots(t *testing.T) {
	records, profile := spellPlayRecords(), syntheticRuleset(t)
	state := Object{"classes": []any{Object{"classId": "warlock", "level": 5}}, "preparedSpells": Object{"warlock": []any{"ward"}}, "notes": "retained"}
	next, err := ApplyPlayChange(state, Object{"operation": "swap-spell", "classId": "warlock", "out": "ward", "ref": "find-path"}, records, profile)
	if err != nil {
		t.Fatal(err)
	}
	swap := objects(next["spellSwaps"])[0]
	if text(swap["out"]) != "ward" || text(swap["in"]) != "find-path" || integer(swap["classLevel"], 0) != 5 || !contains(stringsOf(object(next["preparedSpells"])["warlock"]), "find-path") {
		t.Fatalf("Swap = %+v", next)
	}
	if _, err = ApplyPlayChange(state, Object{"operation": "swap-spell", "classId": "warlock", "out": "ward", "ref": "other"}, records, profile); err == nil {
		t.Fatal("Accepted ineligible swap")
	}
	state["feats"] = []any{Object{"featId": "marked"}, Object{"featId": "special-slot"}}
	sheet := Hydrate(NormalizeBuilderDecisions(state, records, profile), records, &profile).Sheet
	if !contains(spellSlotKeys(state, sheet, records, "ward", 1), "feat-slot-special-slot") || contains(spellSlotKeys(state, sheet, records, "find-path", 1), "feat-slot-special-slot") {
		t.Fatal("Restricted slot did not follow feat spell list")
	}
	if _, err = ApplyPlayChange(state, Object{"operation": "cast-spell", "classId": "warlock", "ref": "ward", "slot": "feat-slot-special-slot"}, records, profile); err != nil {
		t.Fatal(err)
	}
}
