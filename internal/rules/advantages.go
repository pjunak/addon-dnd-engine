package rules

var advantageRolls = []string{"save", "check", "initiative", "concentration", "deathSave"}

// hydrateAdvantages lists the rolls passive grants.advantages and active
// "advantage" modifiers give Advantage on: on is save, check, initiative,
// concentration or deathSave, optionally narrowed by abilities or skills. The
// matching save, skill, ability-check and roll rows are marked. An entry with a
// when text applies only in that situation, so it is listed without marking
// rows. This is sheet guidance; rolls are not resolved.
func hydrateAdvantages(sheet Object, sources []grantSource, modifiers []Object, armor armorState) {
	entries := []any{}
	add := func(advantage Object, name string, source Object) {
		on := text(advantage["on"])
		if !contains(advantageRolls, on) || !armorRequirementMet(advantage["requires"], armor) {
			return
		}
		entry := Object{"on": on, "name": name, "source": source}
		for _, key := range []string{"abilities", "skills"} {
			if list := stringsOf(advantage[key]); len(list) > 0 {
				entry[key] = anyStrings(list)
			}
		}
		if when := text(advantage["when"]); when != "" {
			entry["when"] = when
		} else {
			markAdvantage(sheet, on, stringsOf(advantage["abilities"]), stringsOf(advantage["skills"]))
		}
		entries = append(entries, entry)
	}
	for _, source := range sources {
		for _, advantage := range objects(source.Grants["advantages"]) {
			add(advantage, firstText(source.Record["name"], source.Source["id"]), source.Source)
		}
	}
	for _, modifier := range modifiers {
		if text(modifier["target"]) == "advantage" {
			add(modifier, text(modifier["sourceName"]), object(modifier["source"]))
		}
	}
	if len(entries) > 0 {
		sheet["advantages"] = entries
	}
}

func markAdvantage(sheet Object, on string, abilities, skills []string) {
	switch on {
	case "save":
		if len(abilities) == 0 {
			abilities = Abilities[:]
		}
		for _, ability := range abilities {
			if row := object(object(sheet["saves"])[ability]); row != nil {
				row["advantage"] = true
			}
		}
	case "check":
		for id, raw := range object(sheet["skills"]) {
			row := object(raw)
			if contains(skills, id) || contains(abilities, text(row["ability"])) || len(skills)+len(abilities) == 0 {
				row["advantage"] = true
			}
		}
		for _, ability := range abilities {
			if row := object(object(sheet["abilities"])[ability]); row != nil {
				row["checkAdvantage"] = true
			}
		}
	default:
		sheet[on+"Advantage"] = true
	}
}
