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
	// Older providers may still use a compound display string. An unresolved
	// alternative is not a selected size, and must not become a hidden default.
	switch size := text(species["size"]); size {
	case "Tiny", "Small", "Medium", "Large", "Huge", "Gargantuan":
		return size
	}
	return nil
}
