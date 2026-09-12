package rules

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestPlayRestAndHitDicePreserveAuthoredState(t *testing.T) {
	profile := syntheticRuleset(t)
	decisions := Object{"classes": []any{Object{"classId": "wizard", "level": 5}}, "baseStats": Object{"CON": 14}, "hp": 2, "tempHp": 4,
		"resourceUses": Object{"hit-dice-d6": 2, "slot-1": 0}, "notes": "Keep me", "resources": []any{Object{"id": "manual", "current": 0, "max": 3}}}
	before, _ := json.Marshal(decisions)
	spent, err := applyPlayFixture(decisions, Object{"operation": "spend-hit-die", "key": "hit-dice-d6"}, syntheticRecords(), profile)
	if err != nil {
		t.Fatal(err)
	}
	if integer(spent["hp"], 0) != 8 || integer(object(spent["resourceUses"])["hit-dice-d6"], 0) != 1 {
		t.Fatalf("spent = %+v", spent)
	}
	short, err := applyPlayFixture(spent, Object{"operation": "rest", "rest": "short"}, syntheticRecords(), profile)
	if err != nil {
		t.Fatal(err)
	}
	if integer(object(short["resourceUses"])["slot-1"], -1) != 0 || integer(short["hp"], 0) != 8 {
		t.Fatalf("short = %+v", short)
	}
	long, err := applyPlayFixture(short, Object{"operation": "rest", "rest": "long"}, syntheticRecords(), profile)
	if err != nil {
		t.Fatal(err)
	}
	if integer(long["tempHp"], -1) != 0 || integer(long["hp"], 0) != 32 || object(long["resourceUses"])["slot-1"] != nil {
		t.Fatalf("long = %+v", long)
	}
	after, _ := json.Marshal(decisions)
	manualBefore, _ := json.Marshal(decisions["resources"])
	manualAfter, _ := json.Marshal(long["resources"])
	if string(before) != string(after) || string(manualBefore) != string(manualAfter) || text(long["notes"]) != "Keep me" {
		t.Fatal("Play changes altered authored or input state")
	}
	_, err = applyPlayFixture(decisions, Object{"operation": "spend-hit-die", "key": "slot-1"}, syntheticRecords(), profile)
	if err == nil {
		t.Fatal("Accepted a spell slot as a hit die")
	}
	if _, err := applyPlayFixture(Object{"maxHp": 32, "hp": 12}, Object{"operation": "rest", "rest": "long"}, syntheticRecords(), profile); err == nil {
		t.Fatal("Replaced a hand-filled HP maximum without a valid class")
	}
}

func TestPlayRestSharesHalfLevelRecoveryAndRecognizesEitherRest(t *testing.T) {
	decisions := Object{"resourceUses": Object{"d6": 0, "d10": 0, "pool": 0}}
	sheet := Object{"totalLevel": 5, "resources": []any{
		Object{"key": "d6", "kind": "hitdice", "max": 3, "recharge": []any{Object{"on": "long", "amount": "halfLevel"}}},
		Object{"key": "d10", "kind": "hitdice", "max": 2, "recharge": []any{Object{"on": "long", "amount": "halfLevel"}}},
		Object{"key": "pool", "max": 2, "recharge": []any{Object{"on": "shortOrLong", "amount": "full"}}},
	}}
	applyRest(decisions, sheet, "long")
	uses := object(decisions["resourceUses"])
	if integer(uses["d6"], 3)+integer(uses["d10"], 2) != 2 || uses["pool"] != nil {
		t.Fatalf("uses = %+v", uses)
	}
}

func TestPlaySpellSelectionCastingAndRemoval(t *testing.T) {
	profile := syntheticRuleset(t)
	records := syntheticRecords()
	for _, spell := range []Object{
		{"kind": "spell", "id": "spark", "name": "Spark", "level": 0, "classes": []any{"wizard"}},
		{"kind": "spell", "id": "ward", "name": "Ward", "level": 1, "classes": []any{"wizard"}},
		{"kind": "spell", "id": "other", "level": 1, "classes": []any{"cleric"}},
	} {
		additional := newMemoryRecords([]Object{spell})
		if records.byKind["spell"] == nil {
			records.byKind["spell"] = map[string]json.RawMessage{}
		}
		records.byKind["spell"][text(spell["id"])] = additional.byKind["spell"][text(spell["id"])]
	}
	decisions := Object{"classes": []any{Object{"classId": "wizard", "level": 5}}}
	selectSpell := func(selection, ref string, selected bool) error {
		next, err := applyPlayFixture(decisions, Object{"operation": "select-spell", "classId": "wizard", "ref": ref, "selection": selection, "selected": selected}, records, profile)
		if err == nil {
			decisions = next
		}
		return err
	}
	if err := selectSpell("preparedSpells", "ward", true); err == nil {
		t.Fatal("Prepared an unlearned spell")
	}
	if err := selectSpell("spellbook", "other", true); err == nil {
		t.Fatal("Learned another class's spell")
	}
	for _, selection := range []string{"spellbook", "preparedSpells"} {
		if err := selectSpell(selection, "ward", true); err != nil {
			t.Fatal(err)
		}
	}
	next, err := applyPlayFixture(decisions, Object{"operation": "cast-spell", "classId": "wizard", "ref": "ward", "slot": "slot-2"}, records, profile)
	if err != nil {
		t.Fatal(err)
	}
	if integer(object(next["resourceUses"])["slot-2"], -1) != 2 {
		t.Fatalf("slots = %+v", next["resourceUses"])
	}
	object(next["resourceUses"])["slot-2"] = 0
	if _, err := applyPlayFixture(next, Object{"operation": "cast-spell", "classId": "wizard", "ref": "ward", "slot": "slot-2"}, records, profile); err == nil {
		t.Fatal("Cast with exhausted slots")
	}
	if err := selectSpell("cantrips", "spark", true); err != nil {
		t.Fatal(err)
	}
	if _, err := applyPlayFixture(decisions, Object{"operation": "cast-spell", "classId": "wizard", "ref": "spark", "slot": ""}, records, profile); err != nil {
		t.Fatal(err)
	}
	delete(records.byKind["spell"], "ward")
	if err := selectSpell("spellbook", "ward", false); err != nil {
		t.Fatal(err)
	}
	if len(stringsOf(object(decisions["preparedSpells"])["wizard"])) != 0 {
		t.Fatal("Forgotten spell remained prepared")
	}
}

func TestPlayChangeRejectsMalformedRequests(t *testing.T) {
	for _, change := range []Object{
		{"operation": "rest", "rest": "week"}, {"operation": "rest", "rest": "short", "key": "unexpected"},
		{"operation": "toggle-feature", "key": "ward", "enabled": "true"}, {"operation": "cast-spell", "classId": "wizard", "ref": "ward"},
	} {
		if err := validatePlayChange(change); err == nil {
			t.Fatalf("Accepted %+v", change)
		}
	}
}

func TestBuilderPublishesSelectedFeatAbilityBudget(t *testing.T) {
	plan := BuilderPlan(Object{"classes": []any{Object{"classId": "fighter", "level": 19}}, "featureChoices": Object{"asi:fighter:19": "feat", "asi:fighter:19:feat": "boon-of-fortitude"}}, syntheticRecords(), syntheticRuleset(t))
	for _, choice := range objects(plan["classChoices"]) {
		if text(choice["id"]) != "asi:fighter:19" {
			continue
		}
		ability := object(object(choice["feat"])["ability"])
		if integer(ability["budget"], 0) != 1 || !contains(stringsOf(ability["eligible"]), "CON") {
			t.Fatalf("ability = %+v", ability)
		}
		return
	}
	t.Fatal("Missing feat advancement")
}

func TestPlayFeatureChangesRefreshModifiersAndPactRest(t *testing.T) {
	profile := syntheticRuleset(t)
	records := syntheticRecords()
	decisions := Object{"classes": []any{Object{"classId": "warlock", "level": 5}}, "feats": []any{Object{"featId": "guardian"}}, "resourceUses": Object{"pact-slot": 0}, "hp": 2}
	before := Hydrate(NormalizeBuilderDecisions(decisions, records, profile), records, &profile).Sheet
	activation := objects(before["activations"])[0]
	next, err := applyPlayFixture(decisions, Object{"operation": "toggle-feature", "key": activation["key"], "enabled": true}, records, profile)
	if err != nil {
		t.Fatal(err)
	}
	after := Hydrate(next, records, &profile).Sheet
	if integer(object(after["derived"])["armorClass"], 0) != integer(object(before["derived"])["armorClass"], 0)+1 {
		t.Fatal("Activation did not affect armor class")
	}
	next, err = applyPlayFixture(next, Object{"operation": "rest", "rest": "short"}, records, profile)
	if err != nil {
		t.Fatal(err)
	}
	if object(next["resourceUses"])["pact-slot"] != nil || !truth(object(next["activeFeatures"])[text(activation["key"])]) {
		t.Fatalf("Short rest = %+v", next)
	}
	next, err = applyPlayFixture(next, Object{"operation": "rest", "rest": "long"}, records, profile)
	if err != nil || len(object(next["activeFeatures"])) != 0 {
		t.Fatalf("Long rest = %+v, %v", next, err)
	}
}

// Compose low-level play regressions independently of the typed character
// boundary; public command validation is covered by character tests.
func applyPlayFixture(decisions, change Object, records Records, profile Ruleset) (Object, error) {
	if err := validatePlayChange(change); err != nil {
		return nil, err
	}
	next := NormalizeBuilderDecisions(decisions, records, profile)
	sheet := Hydrate(next, records, &profile).Sheet
	if integer(object(sheet["derived"])["maxHp"], 0) <= 0 && integer(decisions["maxHp"], 0) > 0 {
		return nil, fmt.Errorf("Choose a valid class in Builder before calculating play changes. Saved values have been kept.")
	}
	return applyPlayWithSheet(decisions, change, records, profile, sheet)
}
