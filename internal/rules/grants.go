package rules

type grantSource struct {
	Record Object
	Grants Object
	Source Object
	Level  int
}

func collectGrantSources(
	classes []resolvedClass,
	species Object,
	lineage Object,
	background Object,
	feats []Object,
	acquisitions []Object,
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
	type extension struct {
		activation Object
		source     grantSource
	}
	extensions := []extension{}
	applyModifiers := func(activation Object, source grantSource, name string) {
		for _, modifier := range objects(activation["modifiers"]) {
			if text(modifier["target"]) == "attack" {
				modifier = resolveAttackColumns(modifier, source, classes)
				modifier["sourceName"] = name
				modifier["source"] = source.Source
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
				extensions = append(extensions, extension{activation, source})
				continue
			}
			if id == "" {
				continue
			}
			key := grantOwner(source.Source) + ":" + id
			available := armorRequirementMet(activation["restrictions"], armor)
			enabled := truth(active[key]) && available
			activations = append(activations, Object{
				"key": key, "id": id, "name": firstText(activation["name"], id),
				"source": source.Source, "resource": grantActivationResource(source.Source, activation["resource"]),
				"exclusiveGroup": nullableText(activation["exclusiveGroup"]),
				"active":         enabled, "available": available,
			})
			if condition := text(activation["condition"]); condition != "" {
				object(activations[len(activations)-1])["condition"] = condition
			}
			if enabled {
				enabledIDs[id] = true
				applyModifiers(activation, source, firstText(activation["name"], id))
			}
		}
	}
	for _, current := range extensions {
		extended := stringsOf(current.activation["extends"])
		if name := text(current.activation["extends"]); name != "" {
			extended = []string{name}
		}
		for _, id := range extended {
			if enabledIDs[id] && armorRequirementMet(current.activation["restrictions"], armor) {
				applyModifiers(current.activation, current.source, firstText(current.activation["name"], current.source.Record["name"], id))
				break
			}
		}
	}
	sheet["activations"] = activations
	return modifiers
}

// modifierFlySpeed is the flying speed one modifier grants: a number of feet,
// or "speed" for a flying speed equal to the walking speed.
func modifierFlySpeed(modifier Object, walking int) int {
	if text(modifier["value"]) == "speed" {
		return walking
	}
	return integer(modifier["value"], 0)
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
	// senseBonuses extend a range, or grant it when absent (Umbral Sight).
	for _, source := range sources {
		for id, rawDistance := range object(source.Grants["senseBonuses"]) {
			senses[id] = integer(senses[id], 0) + max(0, integer(rawDistance, 0))
		}
	}
	speedBonus := 0
	flySpeed := 0
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
		case "flySpeed":
			flySpeed = max(flySpeed, modifierFlySpeed(modifier, integer(sheet["speed"], 0)+speedBonus))
		case "concentrationSave":
			concentrationBonus += integer(modifier["add"], 0)
			concentrationBonus += integer(object(abilities[text(modifier["addAbility"])])["mod"], 0)
		}
	}
	if speedBonus != 0 {
		sheet["speed"] = integer(sheet["speed"], 0) + speedBonus
		object(sheet["derived"])["speed"] = sheet["speed"]
	}
	sheet["flySpeed"] = flySpeed
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
