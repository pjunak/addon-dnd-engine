package rules

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestUniversalArithmetic(t *testing.T) {
	t.Parallel()
	if AbilityModifier(9) != -1 || AbilityModifier(16) != 3 {
		t.Fatal("ability modifier regression")
	}
	if ProficiencyBonus(1) != 2 || ProficiencyBonus(17) != 6 {
		t.Fatal("proficiency bonus regression")
	}
	if HitDieAverage("d10") != 6 || HitDieAverage("broken") != 5 {
		t.Fatal("hit-die average regression")
	}
	if ClampHP(-2, 10) != 0 || ClampHP(12, 10) != 10 || ClampHP(4, 0) != 4 {
		t.Fatal("hit point clamp regression")
	}
	if SaveDC(16, 5) != 14 {
		t.Fatal("save DC regression")
	}
}

func TestValidatedRulesetDrivesEditionPolicy(t *testing.T) {
	t.Parallel()
	ruleset := syntheticRuleset(t)
	if ruleset.StableID() != "synthetic-dnd-2024" || ScrollCopyCost(3, ruleset) != 150 {
		t.Fatalf("ruleset = %+v", ruleset)
	}
	if PointBuyCost(15, ruleset) != 9 || PointsSpent(map[string]int{
		"STR": 15, "DEX": 8, "CON": 8, "INT": 8, "WIS": 8, "CHA": 8,
	}, ruleset) != 9 {
		t.Fatal("point-buy regression")
	}
	if slots := MulticlassSlots(5, ruleset); len(slots) != 3 || slots[2] != 2 {
		t.Fatalf("slots = %v", slots)
	}
	if pact := PactMagic(17, ruleset); pact == nil || pact.Slots != 4 || pact.Level != 5 {
		t.Fatalf("pact magic = %+v", pact)
	}
	if values := FeatASIFrom(map[string]any{"from": []any{"ANY"}}); len(values) != 6 {
		t.Fatalf("feat ASI values = %v", values)
	}
	if cap := FeatAbilityCap(map[string]any{"category": "epicBoon"}, ruleset); cap == nil || *cap != 30 {
		t.Fatalf("feat cap = %v", cap)
	}
}

func TestRulesetRejectsIncompleteAndInheritedProfiles(t *testing.T) {
	t.Parallel()
	if _, err := DecodeRuleset(json.RawMessage(`{"id":"incomplete"}`)); err == nil {
		t.Fatal("incomplete ruleset was accepted")
	}
	body, err := os.ReadFile(filepath.Join("..", "..", "testdata", "synthetic-ruleset.json"))
	if err != nil {
		t.Fatal(err)
	}
	var inherited map[string]any
	if err := json.Unmarshal(body, &inherited); err != nil {
		t.Fatal(err)
	}
	inherited["extends"] = "engine-default"
	changed, _ := json.Marshal(inherited)
	if _, err := DecodeRuleset(changed); err == nil {
		t.Fatal("inherited ruleset was accepted")
	}
}

func syntheticRuleset(t *testing.T) Ruleset {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "testdata", "synthetic-ruleset.json"))
	if err != nil {
		t.Fatal(err)
	}
	ruleset, err := DecodeRuleset(body)
	if err != nil {
		t.Fatal(err)
	}
	return ruleset
}
