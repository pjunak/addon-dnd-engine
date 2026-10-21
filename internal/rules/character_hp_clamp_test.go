package rules

import (
	"reflect"
	"testing"

	"github.com/pjunak/addon-dnd-engine/character"
)

func TestLoweredMaximumClampsCurrentHPWithoutHealing(t *testing.T) {
	input, records, profile := characterFixture(t)
	maximum := EvaluateCharacter(input, records, profile).Sheet.Derived.MaxHP
	input.Play.HP = maximum + 7
	before := cloneCharacter(input)
	result := EvaluateCharacter(input, records, profile)
	if !result.Ready || result.Inputs.Play.HP != maximum {
		t.Fatalf("current HP %d above maximum %d was not lowered: %+v", result.Inputs.Play.HP, maximum, result.Issues)
	}
	if !reflect.DeepEqual(input, before) {
		t.Fatal("the caller's inputs changed")
	}
	if explanation := result.Explanations["play.hp"]; explanation.Value != maximum {
		t.Fatal("the current HP explanation shows the unclamped value", explanation)
	}
	input.Play.HP = maximum - 3
	if healed := EvaluateCharacter(input, records, profile); healed.Inputs.Play.HP != maximum-3 {
		t.Fatal("evaluation healed the character", healed.Inputs.Play.HP)
	}
	// Without a calculated sheet there is no maximum to clamp to.
	input.Play.HP = 40
	input.Build.Species = ""
	if unfinished := EvaluateCharacter(input, records, profile); unfinished.Sheet.Status != character.SheetNeedsChoices || unfinished.Inputs.Play.HP != 40 {
		t.Fatal("an unfinished build changed current HP", unfinished.Inputs.Play.HP)
	}
}
