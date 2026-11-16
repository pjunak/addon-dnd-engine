package rules

import (
	"slices"

	"github.com/pjunak/addon-dnd-engine/character"
)

// creatureSizes are the size categories from smallest to largest.
var creatureSizes = []string{"Tiny", "Small", "Medium", "Large", "Huge", "Gargantuan"}

// A size choice belongs to its species, so changing species cannot transfer an
// unrelated selection. Display prose is never parsed into mechanical options.
func speciesSizeChoice(species Object) *character.PlanChoice {
	if species["sizeOptions"] == nil {
		return nil
	}
	return &character.PlanChoice{
		ID: "species:" + text(species["id"]) + ":size", Kind: character.ChoiceSize, Prompt: "Size", Count: 1,
		From:   nonNilStrings(species["sizeOptions"]),
		Source: &character.Source{Type: character.SourceSpecies, ID: text(species["id"]), Level: 1},
	}
}

// selectedSpeciesSize is the chosen or fixed size, or "" while unknown.
func selectedSpeciesSize(decisions, species Object) string {
	if choice := speciesSizeChoice(species); choice != nil {
		selected := text(object(decisions["featureChoices"])[choice.ID])
		if selected != "" && contains(choice.From, selected) {
			return selected
		}
		return ""
	}
	// A species without size options has one fixed size. A compound display
	// string such as "Medium or Small" is not a selection and never a default.
	if size := text(species["size"]); slices.Contains(creatureSizes, size) {
		return size
	}
	return ""
}
