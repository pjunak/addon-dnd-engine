package rules

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/pjunak/addon-dnd-engine/character"
)

func carryingFixture(t *testing.T) (character.Inputs, memoryRecords, Ruleset) {
	t.Helper()
	input, source, profile := characterFixture(t)
	records := source.(memoryRecords)
	profile.Constants.Carrying = &CarryingPolicy{CapacityPerStrength: 15, CoinsPerLb: 50}
	records.byKind["gear"] = map[string]json.RawMessage{
		"pack":    mustJSON(Object{"kind": "gear", "id": "pack", "name": "Pack", "weightLb": 5, "container": Object{"capacityLb": 30}}),
		"bag":     mustJSON(Object{"kind": "gear", "id": "bag", "name": "Bag", "weightLb": 5, "container": Object{"capacityLb": 500, "weightless": true}}),
		"rope":    mustJSON(Object{"kind": "gear", "id": "rope", "name": "Rope", "weightLb": 5}),
		"tent":    mustJSON(Object{"kind": "gear", "id": "tent", "name": "Tent", "weightLb": 20}),
		"mystery": mustJSON(Object{"kind": "gear", "id": "mystery", "name": "Mystery"}),
	}
	ref := func(id string) *character.Reference { return &character.Reference{Kind: "gear", ID: id} }
	anvil := 50.0
	input.Play.Inventory = []character.Item{
		{ID: "pack", Name: "Pack", Reference: ref("pack"), Quantity: 1, Location: "carried"},
		{ID: "bag", Name: "Bag", Reference: ref("bag"), Quantity: 1, Location: "carried"},
		{ID: "rope", Name: "Rope", Reference: ref("rope"), Quantity: 1, Location: "carried", ContainerID: "in-pack"},
		{ID: "tent", Name: "Tent", Reference: ref("tent"), Quantity: 1, Location: "carried", ContainerID: "in-bag"},
		{ID: "anvil", Name: "Anvil", Quantity: 1, Location: "stored", ContainerID: "camp", WeightLb: &anvil},
		{ID: "mystery", Name: "Mystery", Reference: ref("mystery"), Quantity: 2, Location: "carried"},
	}
	input.Play.Containers = []character.Container{{ID: "in-pack", Name: "Pack", ItemID: "pack"}, {ID: "in-bag", Name: "Bag", ItemID: "bag"}, {ID: "camp", Name: "Camp"}}
	input.Play.Currency = map[string]float64{"gp": 90, "sp": 10}
	return input, records, profile
}

func TestCarryingCountsCarriedWeightAgainstStrength(t *testing.T) {
	input, records, profile := carryingFixture(t)
	result := EvaluateCharacter(input, records, profile)
	if !result.Ready {
		t.Fatal(result.Issues)
	}
	carrying := result.Sheet.Carrying
	if carrying == nil || carrying.WeightLb != 17 || carrying.CapacityLb != 225 || carrying.ItemsLb != 15 || carrying.CoinsLb != 2 || !carrying.CoinWeight || carrying.StoredLb != 50 || carrying.WeightlessLb != 20 {
		t.Fatalf("carrying = %+v", carrying)
	}
	if !reflect.DeepEqual(carrying.Unknown, []string{"mystery"}) {
		t.Error("unknown weights", carrying.Unknown)
	}
	containers := carrying.Containers
	if containers["in-pack"] != (character.CarriedContainer{LoadLb: 5, CapacityLb: 30}) ||
		containers["in-bag"] != (character.CarriedContainer{LoadLb: 20, CapacityLb: 500, Weightless: true}) ||
		containers["camp"] != (character.CarriedContainer{LoadLb: 50, Stored: true}) {
		t.Error("containers", containers)
	}
	if carrying.Items["anvil"] != (character.CarriedItem{EachLb: 50, TotalLb: 50, Custom: true}) {
		t.Error("custom item weight", carrying.Items["anvil"])
	}
	if offered := result.Guidance.Storage.ContainerItems; !reflect.DeepEqual(offered, []character.ContainerItem{{ItemID: "pack", CapacityLb: 30}, {ItemID: "bag", Weightless: true, CapacityLb: 500}}) {
		t.Error("container items", offered)
	}
	off := false
	input.Play.CoinWeight = &off
	if carrying := EvaluateCharacter(input, records, profile).Sheet.Carrying; carrying.WeightLb != 15 || carrying.CoinWeight {
		t.Error("coin weight off", carrying)
	}
	profile.Constants.Carrying = nil
	if EvaluateCharacter(input, records, profile).Sheet.Carrying != nil {
		t.Error("carrying without a ruleset policy")
	}
}

func TestCarryingRejectsInvalidWeightsAndLinks(t *testing.T) {
	for name, change := range map[string]func(*character.Inputs){
		"negative weight": func(input *character.Inputs) { weight := -1.0; input.Play.Inventory[0].WeightLb = &weight },
		"missing item":    func(input *character.Inputs) { input.Play.Containers[0].ItemID = "gone" },
		"shared item":     func(input *character.Inputs) { input.Play.Containers[1].ItemID = "pack" },
		"inside itself":   func(input *character.Inputs) { input.Play.Inventory[0].ContainerID = "in-pack" },
	} {
		input, records, profile := carryingFixture(t)
		change(&input)
		if EvaluateCharacter(input, records, profile).Ready {
			t.Errorf("%s was accepted", name)
		}
	}
	ruleset := syntheticRuleset(t)
	ruleset.Constants.Carrying = &CarryingPolicy{CapacityPerStrength: 0, CoinsPerLb: 50}
	if ruleset.Validate() == nil {
		t.Error("zero capacity accepted")
	}
}

// setCarryingRecord replaces a fixture record with changed fields.
func setCarryingRecord(records memoryRecords, kind, id string, fields Object) {
	record := recordByID(records, kind, id)
	for key, value := range fields {
		record[key] = value
	}
	records.byKind[kind][id] = mustJSON(record)
}

func carryingSizeTerm(t *testing.T, result character.Result) character.Term {
	t.Helper()
	terms := result.Explanations["carrying"].Terms
	last := terms[len(terms)-1]
	if !strings.HasSuffix(last.LabelKey, " size: capacity × {0}") {
		t.Fatalf("the last carrying term is not the size multiplier: %+v", terms)
	}
	return last
}

func TestCarryingCapacityScalesWithSize(t *testing.T) {
	for size, capacity := range map[string]float64{"Tiny": 112.5, "Small": 225, "Medium": 225, "Large": 450, "Huge": 900, "Gargantuan": 1800} {
		input, records, profile := carryingFixture(t)
		setCarryingRecord(records, "species", "dwarf", Object{"size": size})
		result := EvaluateCharacter(input, records, profile)
		if got := result.Sheet.Carrying.CapacityLb; got != capacity {
			t.Errorf("%s capacity = %v, want %v", size, got, capacity)
		}
		sizeTerm := carryingSizeTerm(t, result)
		if sizeTerm.LabelKey != size+" size: capacity × {0}" || sizeTerm.Value != capacity {
			t.Errorf("%s size term = %+v", size, sizeTerm)
		}
	}
	input, records, profile := carryingFixture(t)
	setCarryingRecord(records, "species", "dwarf", Object{"size": "Large"})
	profile.Constants.Carrying.SizeMultipliers = map[string]float64{"Tiny": 1, "Small": 1, "Medium": 1, "Large": 3, "Huge": 3, "Gargantuan": 3}
	result := EvaluateCharacter(input, records, profile)
	if result.Sheet.Carrying.CapacityLb != 675 || !reflect.DeepEqual(carryingSizeTerm(t, result).LabelArgs, []string{"3"}) {
		t.Error("the ruleset's size multipliers were not used", result.Sheet.Carrying.CapacityLb)
	}
	terms := result.Explanations["carrying"].Terms
	if base := terms[len(terms)-2]; base.LabelKey != "Capacity: Strength {0} × {1}" || base.Value != 225.0 {
		t.Error("the Strength term no longer shows the capacity before size", base)
	}
}

func TestCarryingSizeGrantCountsTheCharacterLarger(t *testing.T) {
	input, records, profile := carryingFixture(t)
	setCarryingRecord(records, "species", "dwarf", Object{"size": "Medium", "grants": Object{"carryingSizeSteps": 1}})
	result := EvaluateCharacter(input, records, profile)
	if !result.Ready || result.Sheet.Carrying.CapacityLb != 450 {
		t.Fatal("a Medium character counted one size larger must carry as Large", result.Sheet.Carrying, result.Issues)
	}
	explanation := result.Explanations["carrying"]
	species := character.Reference{Kind: "species", ID: "dwarf"}
	grant := explanation.Terms[len(explanation.Terms)-2]
	if grant.LabelKey != "{0}: carrying size +{1}" || !reflect.DeepEqual(grant.LabelArgs, []string{"Dwarf", "1"}) || grant.Source == nil || *grant.Source != species {
		t.Error("the grant term does not name its source", grant)
	}
	if sizeTerm := carryingSizeTerm(t, result); sizeTerm.LabelKey != "Large size: capacity × {0}" || sizeTerm.Value != 450.0 {
		t.Error("the counted size is not shown", sizeTerm)
	}
	if !reflect.DeepEqual(explanation.Sources, []character.Reference{species}) {
		t.Error("explanation sources", explanation.Sources)
	}
	if size := object(view(result.Sheet)["derived"])["size"]; size != "Medium" {
		t.Error("counting larger for carrying changed the character's size", size)
	}

	// Two such features do not stack, and nothing counts beyond the largest size.
	setCarryingRecord(records, "background", "artisan", Object{"grants": Object{"carryingSizeSteps": 1}})
	result = EvaluateCharacter(input, records, profile)
	if result.Sheet.Carrying.CapacityLb != 450 || len(result.Explanations["carrying"].Sources) != 2 {
		t.Error("two grants of one size larger stacked", result.Sheet.Carrying.CapacityLb, result.Explanations["carrying"].Sources)
	}
	setCarryingRecord(records, "species", "dwarf", Object{"size": "Gargantuan"})
	if capacity := EvaluateCharacter(input, records, profile).Sheet.Carrying.CapacityLb; capacity != 1800 {
		t.Error("a Gargantuan character counted larger than the largest size", capacity)
	}

	// An unknown size multiplies by nothing and shows no size terms.
	setCarryingRecord(records, "species", "dwarf", Object{"size": "Medium or Small", "sizeOptions": []any{"Small", "Medium"}})
	result = EvaluateCharacter(input, records, profile)
	terms := result.Explanations["carrying"].Terms
	if result.Sheet.Carrying.CapacityLb != 225 || terms[len(terms)-1].LabelKey != "Capacity: Strength {0} × {1}" {
		t.Error("an unchosen size was guessed for carrying", result.Sheet.Carrying.CapacityLb, terms)
	}
	if err := validateGrants(Object{"carryingSizeSteps": 1}); err != nil {
		t.Error("the grant is not in the vocabulary", err)
	}
}

func TestCarryingSizeMultipliersMustNameEverySize(t *testing.T) {
	for name, multipliers := range map[string]map[string]float64{
		"missing size": {"Tiny": 0.5, "Small": 1, "Medium": 1, "Large": 2, "Huge": 4},
		"unknown size": {"Tiny": 0.5, "Small": 1, "Medium": 1, "Large": 2, "Huge": 4, "Gargantuan": 8, "Colossal": 16},
		"zero":         {"Tiny": 0, "Small": 1, "Medium": 1, "Large": 2, "Huge": 4, "Gargantuan": 8},
		"empty":        {},
	} {
		ruleset := syntheticRuleset(t)
		ruleset.Constants.Carrying = &CarryingPolicy{CapacityPerStrength: 15, CoinsPerLb: 50, SizeMultipliers: multipliers}
		if ruleset.Validate() == nil {
			t.Errorf("%s accepted", name)
		}
	}
	ruleset := syntheticRuleset(t)
	ruleset.Constants.Carrying = &CarryingPolicy{CapacityPerStrength: 15, CoinsPerLb: 50, SizeMultipliers: standardSizeMultipliers}
	if err := ruleset.Validate(); err != nil {
		t.Error(err)
	}
}
