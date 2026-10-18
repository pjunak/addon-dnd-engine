package rules

import "strings"

var advantageRolls = []string{"save", "check", "attack", "initiative", "concentration", "deathSave"}

// hydrateAdvantages lists the rolls passive grants.advantages and active
// "advantage" modifiers give Advantage on, plus Disadvantage on Stealth from
// worn armor that imposes it: on is save, check, attack, initiative,
// concentration or deathSave, optionally narrowed by abilities or skills. An
// entry with a when text applies only in that situation, so it is listed
// without marking rows. Conditions add their own entries later. This is sheet
// guidance; rolls are not resolved.
func hydrateAdvantages(sheet Object, sources []grantSource, modifiers []Object, armor armorState) {
	entries := []any{}
	add := func(advantage Object, name string, source Object) {
		if entry := rollEntry(advantage, name, source); entry != nil && armorRequirementMet(advantage["requires"], armor) {
			entries = append(entries, entry)
		}
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
	if restrictions := object(sheet["armorRestrictions"]); truth(restrictions["stealthDisadvantage"]) {
		entries = append(entries, Object{
			"on": "check", "mode": "disadvantage", "skills": []any{"stealth"},
			"name":   firstText(restrictions["armorName"], restrictions["armorId"]),
			"source": Object{"type": "armor", "id": text(restrictions["armorId"])},
		})
	}
	if len(entries) > 0 {
		sheet["advantages"] = entries
	}
	applyRollMarks(sheet)
}

// rollEntry validates one declared roll effect; an unknown roll or mode
// declares nothing rather than guessing.
func rollEntry(advantage Object, name string, source Object) Object {
	on, mode := text(advantage["on"]), text(advantage["mode"])
	if !contains(advantageRolls, on) || (mode != "" && mode != "advantage" && mode != "disadvantage") {
		return nil
	}
	entry := Object{"on": on, "name": name, "source": source}
	if mode == "disadvantage" {
		entry["mode"] = mode
	}
	for _, key := range []string{"abilities", "skills"} {
		if list := stringsOf(advantage[key]); len(list) > 0 {
			entry[key] = anyStrings(list)
		}
	}
	if when := text(advantage["when"]); when != "" {
		entry["when"] = when
	}
	return entry
}

// applyRollMarks recomputes every roll mark from sheet.advantages. Advantage
// and Disadvantage on the same roll cancel, so a row is marked only with the
// one that remains: advantage or disadvantage on save and skill rows,
// checkAdvantage or checkDisadvantage on ability rows, and
// <roll>Advantage or <roll>Disadvantage for attack, initiative, concentration
// and death save rolls. Situational entries mark nothing.
func applyRollMarks(sheet Object) {
	type marks struct{ advantage, disadvantage bool }
	found := map[string]*marks{}
	mark := func(key, mode string) {
		if found[key] == nil {
			found[key] = &marks{}
		}
		if mode == "disadvantage" {
			found[key].disadvantage = true
		} else {
			found[key].advantage = true
		}
	}
	for _, entry := range objects(sheet["advantages"]) {
		if text(entry["when"]) != "" {
			continue
		}
		on, mode := text(entry["on"]), text(entry["mode"])
		abilities, skills := stringsOf(entry["abilities"]), stringsOf(entry["skills"])
		switch on {
		case "save":
			if len(abilities) == 0 {
				abilities = Abilities[:]
			}
			for _, ability := range abilities {
				mark("save:"+ability, mode)
			}
		case "check":
			for id, raw := range object(sheet["skills"]) {
				if contains(skills, id) || contains(abilities, text(object(raw)["ability"])) || len(skills)+len(abilities) == 0 {
					mark("skill:"+id, mode)
				}
			}
			for _, ability := range abilities {
				mark("ability:"+ability, mode)
			}
		default:
			mark(on, mode)
		}
	}
	// Clear earlier marks so a later pass (conditions) can recompute the net result.
	for _, group := range []string{"saves", "skills"} {
		for _, raw := range object(sheet[group]) {
			delete(object(raw), "advantage")
			delete(object(raw), "disadvantage")
		}
	}
	for _, raw := range object(sheet["abilities"]) {
		delete(object(raw), "checkAdvantage")
		delete(object(raw), "checkDisadvantage")
	}
	for _, on := range advantageRolls[2:] {
		delete(sheet, on+"Advantage")
		delete(sheet, on+"Disadvantage")
	}
	for key, value := range found {
		if value.advantage == value.disadvantage {
			continue
		}
		suffix := "Advantage"
		if value.disadvantage {
			suffix = "Disadvantage"
		}
		var row Object
		field := "advantage"
		if value.disadvantage {
			field = "disadvantage"
		}
		switch kind, id, _ := strings.Cut(key, ":"); kind {
		case "save":
			row = object(object(sheet["saves"])[id])
		case "skill":
			row = object(object(sheet["skills"])[id])
		case "ability":
			row, field = object(object(sheet["abilities"])[id]), "check"+suffix
		default:
			sheet[key+suffix] = true
			continue
		}
		if row != nil {
			row[field] = true
		}
	}
}
