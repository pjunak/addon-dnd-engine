package rules

type grantSource struct {
	Record Object
	Grants Object
	Source Object
	Level  int
	// Inactive marks an owned item that is not in effect; it carries only its
	// resources, so spent uses stay listed and rests still restore them.
	Inactive bool
}

func collectGrantSources(
	classes []resolvedClass,
	species Object,
	lineage Object,
	background Object,
	feats []Object,
	acquisitions []Object,
	items []Object,
	records Records,
	totalLevel int,
	selections Object,
) []grantSource {
	result := make([]grantSource, 0)
	add := func(record, source Object, level int) {
		if record == nil {
			return
		}
		source["level"] = level
		result = append(result, grantSource{
			Record: record, Grants: object(record["grants"]), Source: source, Level: level,
		})
	}
	features := recordCatalog(records, "feature")
	for _, current := range classes {
		add(current.Record, Object{"type": "class", "id": current.ID}, current.Level)
		subclass := recordByID(records, "subclass", current.Subclass)
		add(subclass, Object{"type": "subclass", "id": current.Subclass}, current.Level)
		for _, feature := range features {
			if text(feature["classId"]) != current.ID || text(feature["category"]) != "" || integer(feature["level"], 0) > current.Level {
				continue
			}
			if subclassID := text(feature["subclassId"]); subclassID != "" && subclassID != current.Subclass {
				continue
			}
			add(feature, Object{"type": "feature", "id": text(feature["id"])}, current.Level)
		}
		// Unchosen options are reference text; only chosen ones grant anything.
		for _, option := range chosenOptions(current.ID, current.Level, current.Subclass, selections, records) {
			add(option.feature, Object{"type": "feature", "id": text(option.feature["id"])}, current.Level)
		}
	}
	add(species, Object{"type": "species", "id": text(species["id"])}, totalLevel)
	if lineage != nil {
		add(lineage, Object{"type": "species", "id": text(species["id"])}, totalLevel)
	}
	add(background, Object{"type": "background", "id": text(background["id"])}, totalLevel)
	if acquisitions == nil {
		for _, feat := range feats {
			add(feat, Object{"type": "feat", "id": text(feat["id"])}, totalLevel)
		}
	} else {
		for _, source := range acquisitions {
			add(recordByID(records, "feat", text(source["id"])), source, totalLevel)
		}
	}
	// Item grants were read through their allowlist; the stand-in record names
	// the item, so no other item record field is read as a feature field.
	for _, item := range items {
		source := cloneObjectDeep(object(item["source"]))
		result = append(result, grantSource{
			Record: Object{"id": source["id"], "name": source["name"]}, Grants: cloneObjectDeep(object(item["grants"])),
			Source: source, Level: totalLevel, Inactive: !truth(item["active"]),
		})
	}
	return result
}

func applyChoicePackages(sources []grantSource, selected Object) []grantSource {
	result := make([]grantSource, 0, len(sources))
	for _, source := range sources {
		source.Grants = selectedChoiceGrants(source.Grants, source.Source, selected)
		result = append(result, source)
	}
	return result
}

// Builder discovery and hydration must resolve the same selected package.
// Only the declared top-level packages are interpreted; packages cannot recurse.
func selectedChoiceGrants(grants, source, selected Object) Object {
	for _, declaration := range objects(grants["choicePackages"]) {
		choiceID := text(declaration["choiceId"])
		if choiceID == "" {
			continue
		}
		selectedValue := selected[grantOwner(source)+":"+choiceID]
		if selectedValue == nil {
			selectedValue = selected[choiceID]
		}
		option := object(object(declaration["options"])[text(selectedValue)])
		if option == nil {
			continue
		}
		merged := cloneObject(grants)
		for field, value := range option {
			if additions := values(value); additions != nil {
				merged[field] = append(append([]any(nil), values(merged[field])...), additions...)
			} else {
				merged[field] = value
			}
		}
		grants = merged
	}
	return grants
}

func grantValues(sources []grantSource, field string) []string {
	result := make([]string, 0)
	for _, source := range sources {
		result = append(result, stringsOf(source.Grants[field])...)
		result = append(result, stringsOf(object(source.Grants["proficiencies"])[field])...)
	}
	return result
}

func activeGrantModifiers(
	decisions Object,
	sheet Object,
	sources []grantSource,
	classes []resolvedClass,
	records Records,
) []Object {
	active := object(decisions["activeFeatures"])
	armor := equippedArmor(decisions, records)
	activations := make([]any, 0)
	modifiers := make([]Object, 0)
	enabledIDs := map[string]bool{}
	type deferred struct {
		activation Object
		source     grantSource
		row        Object
	}
	extensions := []deferred{}
	dependents := []deferred{}
	applyModifiers := func(activation Object, source grantSource, name string) {
		for _, modifier := range objects(activation["modifiers"]) {
			if text(modifier["target"]) == "attack" {
				modifier = resolveAttackColumns(modifier, source, classes)
			} else {
				modifier = cloneObject(modifier)
			}
			modifier["sourceName"] = name
			modifier["source"] = source.Source
			if classID := sourceClassID(source); classID != "" {
				modifier["classId"] = classID
			}
			modifiers = append(modifiers, modifier)
		}
	}
	for _, source := range sources {
		declared := append(append([]Object(nil), objects(source.Record["activations"])...),
			objects(source.Grants["activations"])...)
		for _, activation := range declared {
			id := text(activation["id"])
			if integer(activation["minLevel"], 1) > source.Level {
				continue
			}
			// An extension adds modifiers to another feature's activation while
			// that one is on; it has no toggle of its own.
			if activation["extends"] != nil {
				extensions = append(extensions, deferred{activation, source, nil})
				continue
			}
			if id == "" {
				continue
			}
			key := grantOwner(source.Source) + ":" + id
			available := armorRequirementMet(activation["restrictions"], armor)
			enabled := truth(active[key]) && available
			row := Object{
				"key": key, "id": id, "name": firstText(activation["name"], id),
				"source": source.Source, "resource": grantActivationResource(source.Source, activation["resource"]),
				"exclusiveGroup": nullableText(activation["exclusiveGroup"]),
				"active":         enabled, "available": available,
			}
			if condition := text(activation["condition"]); condition != "" {
				row["condition"] = condition
			}
			activations = append(activations, row)
			// An option chosen for each use of another activation keeps its own
			// toggle, so the choice is remembered, but is in effect only while
			// one of its `while` activations is on.
			if enabled && activation["while"] != nil {
				row["active"] = false
				dependents = append(dependents, deferred{activation, source, row})
			} else if enabled {
				// An item's toggle is its own: another source's extends or
				// while never follows an item activation that shares its ID.
				if text(source.Source["type"]) != "item" {
					enabledIDs[id] = true
				}
				applyModifiers(activation, source, firstText(activation["name"], id))
			}
		}
	}
	for _, current := range dependents {
		for _, id := range activationIDs(current.activation["while"]) {
			if enabledIDs[id] {
				current.row["active"] = true
				enabledIDs[text(current.row["id"])] = true
				applyModifiers(current.activation, current.source, text(current.row["name"]))
				break
			}
		}
	}
	for _, current := range extensions {
		for _, id := range activationIDs(current.activation["extends"]) {
			if enabledIDs[id] && armorRequirementMet(current.activation["restrictions"], armor) {
				applyModifiers(current.activation, current.source, firstText(current.activation["name"], current.source.Record["name"], id))
				break
			}
		}
	}
	sheet["activations"] = activations
	return modifiers
}

// activationIDs reads an activation reference: one ID or a list of them.
func activationIDs(value any) []string {
	if id := text(value); id != "" {
		return []string{id}
	}
	return stringsOf(value)
}

var movementModes = []string{"fly", "swim", "climb", "burrow"}

// movementRows lists special movement: species speeds, passive grants.speeds
// and active flySpeed/swimSpeed/climbSpeed/burrowSpeed modifiers. A value is
// feet or "speed", which follows the final walking speed (times multiply).
func movementRows(species Object, sources []grantSource, modifiers []Object) []any {
	rows := []any{}
	add := func(mode string, value any, multiply any, name string, source Object) {
		if text(value) != "speed" && integer(value, 0) < 1 {
			return
		}
		row := Object{"mode": mode, "value": value, "name": name, "source": source}
		if factor := integer(multiply, 1); factor > 1 {
			row["multiply"] = factor
		}
		rows = append(rows, row)
	}
	for _, mode := range movementModes {
		add(mode, object(species["speeds"])[mode], nil, text(species["name"]), Object{"type": "species", "id": text(species["id"])})
	}
	for _, source := range sources {
		for _, mode := range movementModes {
			if value, exists := object(source.Grants["speeds"])[mode]; exists {
				add(mode, value, nil, firstText(source.Record["name"], source.Source["id"]), source.Source)
			}
		}
	}
	for _, modifier := range modifiers {
		for _, mode := range movementModes {
			if text(modifier["target"]) == mode+"Speed" {
				add(mode, modifier["value"], modifier["multiply"], text(modifier["sourceName"]), object(modifier["source"]))
			}
		}
	}
	return rows
}

// applyMovement resolves each special speed as its greatest row against the
// current walking speed. Fly speed is always reported; the others only when
// granted.
func applyMovement(sheet Object) {
	walking := integer(sheet["speed"], 0)
	best := map[string]int{}
	for _, row := range objects(sheet["movement"]) {
		value := integer(row["value"], 0)
		if text(row["value"]) == "speed" {
			value = walking * max(1, integer(row["multiply"], 1))
		}
		best[text(row["mode"])] = max(best[text(row["mode"])], value)
	}
	sheet["flySpeed"] = best["fly"]
	for _, mode := range movementModes[1:] {
		if best[mode] > 0 {
			sheet[mode+"Speed"] = best[mode]
		} else {
			delete(sheet, mode+"Speed")
		}
	}
}

func applyGenericGrants(
	decisions Object,
	sheet Object,
	sources []grantSource,
	modifiers []Object,
) {
	proficiencies := object(sheet["proficiencies"])
	proficiencies["languages"] = anyStrings(unique(append(
		grantValues(sources, "languages"), stringsOf(decisions["languageProficiencies"])...,
	)))
	sheet["languages"] = proficiencies["languages"]

	resistances := append(stringsOf(sheet["resistances"]), grantValues(sources, "resistances")...)
	resistances = append(resistances, stringsOf(decisions["damageResistances"])...)
	conditionImmunities := append(grantValues(sources, "conditionImmunities"),
		stringsOf(decisions["conditionImmunities"])...)
	damageImmunities := grantValues(sources, "damageImmunities")
	for _, modifier := range modifiers {
		switch text(modifier["target"]) {
		case "resistance":
			resistances = append(resistances, text(modifier["value"]))
		case "conditionImmunity":
			conditionImmunities = append(conditionImmunities, text(modifier["value"]))
		case "damageImmunity":
			damageImmunities = append(damageImmunities, text(modifier["value"]))
		}
	}
	sheet["resistances"] = anyStrings(unique(resistances))
	sheet["conditionImmunities"] = anyStrings(unique(conditionImmunities))
	sheet["damageImmunities"] = anyStrings(unique(damageImmunities))

	senses := object(sheet["senses"])
	for _, source := range sources {
		for id, rawDistance := range object(source.Grants["senses"]) {
			senses[id] = max(integer(senses[id], 0), integer(rawDistance, 0))
		}
	}
	// senseBonuses and active senseBonus modifiers extend a range, or grant it
	// when absent (Umbral Sight), before sense minimums apply.
	for _, source := range sources {
		for id, rawDistance := range object(source.Grants["senseBonuses"]) {
			senses[id] = integer(senses[id], 0) + max(0, integer(rawDistance, 0))
		}
	}
	for _, modifier := range modifiers {
		if key := text(modifier["key"]); key != "" && text(modifier["target"]) == "senseBonus" {
			senses[key] = integer(senses[key], 0) + max(0, integer(modifier["add"], 0))
		}
	}
	speedBonus := 0
	concentrationBonus := 0
	abilities := object(sheet["abilities"])
	for _, modifier := range modifiers {
		switch text(modifier["target"]) {
		case "speed":
			speedBonus += integer(modifier["add"], 0)
		case "sense":
			if key := text(modifier["key"]); key != "" {
				senses[key] = max(integer(senses[key], 0), integer(modifier["value"], 0))
			}
		case "concentrationSave":
			concentrationBonus += integer(modifier["add"], 0)
			concentrationBonus += integer(object(abilities[text(modifier["addAbility"])])["mod"], 0)
		}
	}
	if speedBonus != 0 {
		sheet["speed"] = integer(sheet["speed"], 0) + speedBonus
		object(sheet["derived"])["speed"] = sheet["speed"]
	}
	if rows := movementRows(object(sheet["species"]), sources, modifiers); len(rows) > 0 {
		sheet["movement"] = rows
	}
	applyMovement(sheet)
	sheet["concentrationSaveBonus"] = concentrationBonus
}

func cloneObject(source Object) Object {
	if source == nil {
		return Object{}
	}
	result := make(Object, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
