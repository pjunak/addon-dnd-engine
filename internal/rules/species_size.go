package rules

import "github.com/pjunak/addon-dnd-engine/character"

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
	switch size := text(species["size"]); size {
	case "Tiny", "Small", "Medium", "Large", "Huge", "Gargantuan":
		return size
	}
	return ""
}
