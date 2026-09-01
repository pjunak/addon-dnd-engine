package rules

import "fmt"

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
	records Records,
	totalLevel int,
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
	features := recordList(records, "feature")
	for _, current := range classes {
		add(current.Record, Object{"type": "class", "id": current.ID}, current.Level)
		subclass := recordByID(records, "subclass", current.Subclass)
		add(subclass, Object{"type": "subclass", "id": current.Subclass}, current.Level)
		for _, feature := range features {
			if text(feature["classId"]) != current.ID || integer(feature["level"], 0) > current.Level {
				continue
			}
			if subclassID := text(feature["subclassId"]); subclassID != "" && subclassID != current.Subclass {
				continue
			}
			add(feature, Object{"type": "feature", "id": text(feature["id"])}, current.Level)
		}
	}
	add(species, Object{"type": "species", "id": text(species["id"])}, totalLevel)
	if lineage != nil {
		add(lineage, Object{"type": "species", "id": text(species["id"])}, totalLevel)
	}
	add(background, Object{"type": "background", "id": text(background["id"])}, totalLevel)
	for _, feat := range feats {
		add(feat, Object{"type": "feat", "id": text(feat["id"])}, totalLevel)
	}
	return result
}

func applyChoicePackages(sources []grantSource, selected Object) []grantSource {
	result := make([]grantSource, 0, len(sources))
	for _, source := range sources {
		grants := source.Grants
		for _, declaration := range objects(grants["choicePackages"]) {
			choiceID := text(declaration["choiceId"])
			if choiceID == "" {
				continue
			}
			scopedID := fmt.Sprintf("%s:%s:%s", text(source.Source["type"]), text(source.Source["id"]), choiceID)
			selectedValue := selected[scopedID]
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
		source.Grants = grants
		result = append(result, source)
	}
	return result
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
	records Records,
) []Object {
	active := object(decisions["activeFeatures"])
	wearingArmor := false
	usingShield := false
	for _, item := range objects(decisions["inventory"]) {
		if text(item["location"]) != "equipped" {
			continue
		}
		armor := inventoryRecord(item, records, "armor")
		switch text(armor["armorType"]) {
		case "shield":
			usingShield = true
		case "light", "medium", "heavy":
			wearingArmor = true
		}
	}
	activations := make([]any, 0)
	modifiers := make([]Object, 0)
	for _, source := range sources {
		declared := append(append([]Object(nil), objects(source.Record["activations"])...),
			objects(source.Grants["activations"])...)
		for _, activation := range declared {
			id := text(activation["id"])
			if id == "" || integer(activation["minLevel"], 1) > source.Level {
				continue
			}
			key := fmt.Sprintf("%s:%s:%s", text(source.Source["type"]), text(source.Source["id"]), id)
			restrictions := object(activation["restrictions"])
			available := !(truth(restrictions["noArmor"]) && wearingArmor) &&
				!(truth(restrictions["noShield"]) && usingShield)
			enabled := truth(active[key]) && available
			activations = append(activations, Object{
				"key": key, "id": id, "name": firstText(activation["name"], id),
				"source": source.Source, "resource": nullableObject(activation["resource"]),
				"exclusiveGroup": nullableText(activation["exclusiveGroup"]),
				"active":         enabled, "available": available,
			})
			if enabled {
				modifiers = append(modifiers, objects(activation["modifiers"])...)
			}
		}
	}
	sheet["activations"] = activations
	return modifiers
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
	speedBonus := 0
	flySpeed := 0
	concentrationBonus := 0
	abilities := object(sheet["abilities"])
	for _, modifier := range modifiers {
		switch text(modifier["target"]) {
		case "speed":
			speedBonus += integer(modifier["add"], 0)
		case "flySpeed":
			candidate := integer(modifier["value"], 0)
			if text(modifier["value"]) == "speed" {
				candidate = integer(sheet["speed"], 0) + speedBonus
			}
			flySpeed = max(flySpeed, candidate)
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

func nullableObject(value any) any {
	if current := object(value); current != nil {
		return current
	}
	return nil
}
