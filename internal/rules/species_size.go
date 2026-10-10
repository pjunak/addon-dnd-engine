package rules

// A size choice belongs to its species, so changing species cannot transfer an
// unrelated selection. Display prose is never parsed into mechanical options.
func speciesSizeChoice(species Object) Object {
	if species["sizeOptions"] == nil {
		return nil
	}
	return Object{
		"id": "species:" + text(species["id"]) + ":size", "kind": "size", "prompt": "Size", "count": 1,
		"from":   anyStrings(stringsOf(species["sizeOptions"])),
		"source": Object{"type": "species", "id": text(species["id"]), "level": 1},
	}
}

func selectedSpeciesSize(decisions, species Object) any {
	if choice := speciesSizeChoice(species); choice != nil {
		selected := text(object(decisions["featureChoices"])[text(choice["id"])])
		if selected != "" && contains(stringsOf(choice["from"]), selected) {
			return selected
		}
		return nil
	}
	// A species without size options has one fixed size. A compound display
	// string such as "Medium or Small" is not a selection and never a default.
	switch size := text(species["size"]); size {
	case "Tiny", "Small", "Medium", "Large", "Huge", "Gargantuan":
		return size
	}
	return nil
}
