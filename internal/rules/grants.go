package rules

import "github.com/pjunak/addon-dnd-engine/character"

type grantSource struct {
	Record Object
	Grants Object
	Source character.Source
	Level  int
	// Inactive marks an owned item that is not in effect; it carries only its
	// resources, so spent uses stay listed and rests still restore them.
	Inactive bool
}

// activeModifier is an enabled activation's modifier: the record's fields,
// with the activation's name, source and owning class.
type activeModifier struct {
	fields  Object
	name    string
	source  character.Source
	classID string
}

func (modifier activeModifier) target() string { return text(modifier.fields["target"]) }

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
	add := func(record Object, source character.Source, level int) {
		if record == nil {
			return
		}
		source.Level = level
		result = append(result, grantSource{
			Record: record, Grants: object(record["grants"]), Source: source, Level: level,
		})
	}
	features := recordCatalog(records, "feature")
	for _, current := range classes {
		add(current.Record, character.Source{Type: character.SourceClass, ID: current.ID}, current.Level)
		subclass := recordByID(records, "subclass", current.Subclass)
		add(subclass, character.Source{Type: character.SourceSubclass, ID: current.Subclass}, current.Level)
		for _, feature := range features {
			if text(feature["classId"]) != current.ID || text(feature["category"]) != "" || integer(feature["level"], 0) > current.Level {
				continue
			}
			if subclassID := text(feature["subclassId"]); subclassID != "" && subclassID != current.Subclass {
				continue
			}
			add(feature, character.Source{Type: character.SourceFeature, ID: text(feature["id"])}, current.Level)
		}
		// Unchosen options are reference text; only chosen ones grant anything.
		for _, option := range chosenOptions(current.ID, current.Level, current.Subclass, selections, records) {
			add(option.feature, character.Source{Type: character.SourceFeature, ID: text(option.feature["id"])}, current.Level)
		}
	}
	add(species, character.Source{Type: character.SourceSpecies, ID: text(species["id"])}, totalLevel)
	if lineage != nil {
		add(lineage, character.Source{Type: character.SourceSpecies, ID: text(species["id"])}, totalLevel)
	}
	add(background, character.Source{Type: character.SourceBackground, ID: text(background["id"])}, totalLevel)
	if acquisitions == nil {
		for _, feat := range feats {
			add(feat, character.Source{Type: character.SourceFeat, ID: text(feat["id"])}, totalLevel)
		}
	} else {
		for _, acquired := range acquisitions {
			add(recordByID(records, "feat", text(acquired["id"])), sourceFromObject(acquired), totalLevel)
		}
	}
	// Item grants were read through their allowlist; the stand-in record names
	// the item, so no other item record field is read as a feature field.
	for _, item := range items {
		source := sourceFromObject(object(item["source"]))
		result = append(result, grantSource{
			Record: Object{"id": source.ID, "name": source.Name}, Grants: cloneObjectDeep(object(item["grants"])),
			Source: source, Level: totalLevel, Inactive: !truth(item["active"]),
		})
	}
	return result
}

// sourceFromObject reads a source the Builder decisions carry as a map.
func sourceFromObject(value Object) character.Source {
	source := character.Source{
		Type: character.SourceType(text(value["type"])), ID: text(value["id"]), Level: integer(value["level"], 0),
		Name: text(value["name"]), ClassID: text(value["classId"]),
	}
	if ref := object(value["ref"]); ref != nil {
		source.Ref = &character.Reference{Kind: text(ref["kind"]), ID: text(ref["id"])}
	}
	if acquisition := object(value["acquisition"]); acquisition != nil {
		source.Acquisition = &character.Acquisition{ID: text(acquisition["id"]), Name: text(acquisition["name"]), ClassID: text(acquisition["classId"]), Level: integer(acquisition["level"], 0)}
	}
	return source
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
func selectedChoiceGrants(grants Object, source character.Source, selected Object) Object {
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
	sheet *character.Calculation,
	sources []grantSource,
	classes []resolvedClass,
	records Records,
) []activeModifier {
	active := object(decisions["activeFeatures"])
	armor := equippedArmor(decisions, records)
	activations := make([]character.Activation, 0)
	modifiers := make([]activeModifier, 0)
	enabledIDs := map[string]bool{}
	type deferred struct {
		activation Object
		source     grantSource
		row        int
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
			modifiers = append(modifiers, activeModifier{fields: modifier, name: name, source: source.Source, classID: sourceClassID(source)})
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
				extensions = append(extensions, deferred{activation, source, -1})
				continue
			}
			if id == "" {
				continue
			}
			key := grantOwner(source.Source) + ":" + id
			available := armorRequirementMet(activation["restrictions"], armor)
			enabled := truth(active[key]) && available
			activations = append(activations, character.Activation{
				Key: key, ID: id, Name: firstText(activation["name"], id),
				Source: source.Source, Resource: grantActivationResource(source.Source, activation["resource"]),
				ExclusiveGroup: nullableText(activation["exclusiveGroup"]),
				Active:         enabled, Available: available, Condition: text(activation["condition"]),
			})
			row := len(activations) - 1
			// An option chosen for each use of another activation keeps its own
			// toggle, so the choice is remembered, but is in effect only while
			// one of its `while` activations is on.
			if enabled && activation["while"] != nil {
				activations[row].Active = false
				dependents = append(dependents, deferred{activation, source, row})
			} else if enabled {
				// An item's toggle is its own: another source's extends or
				// while never follows an item activation that shares its ID.
				if source.Source.Type != character.SourceItem {
					enabledIDs[id] = true
				}
				applyModifiers(activation, source, firstText(activation["name"], id))
			}
		}
	}
	for _, current := range dependents {
		for _, id := range activationIDs(current.activation["while"]) {
			if enabledIDs[id] {
				row := &activations[current.row]
				row.Active = true
				enabledIDs[row.ID] = true
				applyModifiers(current.activation, current.source, row.Name)
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
	sheet.Activations = activations
	return modifiers
}

// activationIDs reads an activation reference: one ID or a list of them.
func activationIDs(value any) []string {
	if id := text(value); id != "" {
		return []string{id}
	}
	return stringsOf(value)
}

// movementRows lists special movement: species speeds, passive grants.speeds
// and active flySpeed/swimSpeed/climbSpeed/burrowSpeed modifiers. A value is
// feet or "speed", which follows the final walking speed (times multiply).
func movementRows(species Object, sources []grantSource, modifiers []activeModifier) []character.MovementRow {
	rows := []character.MovementRow{}
	add := func(mode character.MovementMode, value any, multiply any, name string, source character.Source) {
		if text(value) != "speed" && integer(value, 0) < 1 {
			return
		}
		row := character.MovementRow{Mode: mode, Value: character.MovementValue{Distance: integer(value, 0), FollowsSpeed: text(value) == "speed"}, Name: name, Source: source}
		if factor := integer(multiply, 1); factor > 1 {
			row.Multiply = factor
		}
		rows = append(rows, row)
	}
	for _, mode := range character.MovementModes() {
		add(mode, object(species["speeds"])[string(mode)], nil, text(species["name"]), character.Source{Type: character.SourceSpecies, ID: text(species["id"])})
	}
	for _, source := range sources {
		for _, mode := range character.MovementModes() {
			if value, exists := object(source.Grants["speeds"])[string(mode)]; exists {
				add(mode, value, nil, firstText(source.Record["name"], source.Source.ID), source.Source)
			}
		}
	}
	for _, modifier := range modifiers {
		for _, mode := range character.MovementModes() {
			if modifier.target() == string(mode)+"Speed" {
				add(mode, modifier.fields["value"], modifier.fields["multiply"], modifier.name, modifier.source)
			}
		}
	}
	return rows
}

// applyMovement resolves each special speed as its greatest row against the
// current walking speed. Fly speed is always reported; the others only when
// granted.
func applyMovement(sheet *character.Calculation) {
	best := make(map[character.MovementMode]int)
	for _, row := range sheet.Movement {
		value := row.Value.Distance
		if row.Value.FollowsSpeed {
			value = sheet.Speed * max(1, row.Multiply)
		}
		best[row.Mode] = max(best[row.Mode], value)
	}
	sheet.FlySpeed = best[character.MoveFly]
	sheet.SwimSpeed = best[character.MoveSwim]
	sheet.ClimbSpeed = best[character.MoveClimb]
	sheet.BurrowSpeed = best[character.MoveBurrow]
}

func applyGenericGrants(
	decisions Object,
	sheet *character.Calculation,
	species Object,
	sources []grantSource,
	modifiers []activeModifier,
) {
	languages := unique(append(grantValues(sources, "languages"), stringsOf(decisions["languageProficiencies"])...))
	sheet.Proficiencies.Languages = languages
	sheet.Languages = append([]string{}, languages...)

	resistances := append(append([]string{}, sheet.Resistances...), grantValues(sources, "resistances")...)
	resistances = append(resistances, stringsOf(decisions["damageResistances"])...)
	conditionImmunities := append(grantValues(sources, "conditionImmunities"),
		stringsOf(decisions["conditionImmunities"])...)
	damageImmunities := grantValues(sources, "damageImmunities")
	for _, modifier := range modifiers {
		switch modifier.target() {
		case "resistance":
			resistances = append(resistances, text(modifier.fields["value"]))
		case "conditionImmunity":
			conditionImmunities = append(conditionImmunities, text(modifier.fields["value"]))
		case "damageImmunity":
			damageImmunities = append(damageImmunities, text(modifier.fields["value"]))
		}
	}
	sheet.Resistances = unique(resistances)
	sheet.ConditionImmunities = unique(conditionImmunities)
	sheet.DamageImmunities = unique(damageImmunities)

	senses := sheet.Senses
	for _, source := range sources {
		for id, rawDistance := range object(source.Grants["senses"]) {
			senses[id] = max(senses[id], integer(rawDistance, 0))
		}
	}
	// senseBonuses and active senseBonus modifiers extend a range, or grant it
	// when absent (Umbral Sight), before sense minimums apply.
	for _, source := range sources {
		for id, rawDistance := range object(source.Grants["senseBonuses"]) {
			senses[id] += max(0, integer(rawDistance, 0))
		}
	}
	for _, modifier := range modifiers {
		if key := text(modifier.fields["key"]); key != "" && modifier.target() == "senseBonus" {
			senses[key] += max(0, integer(modifier.fields["add"], 0))
		}
	}
	speedBonus := 0
	concentrationBonus := 0
	for _, modifier := range modifiers {
		switch modifier.target() {
		case "speed":
			speedBonus += integer(modifier.fields["add"], 0)
		case "sense":
			if key := text(modifier.fields["key"]); key != "" {
				senses[key] = max(senses[key], integer(modifier.fields["value"], 0))
			}
		case "concentrationSave":
			concentrationBonus += integer(modifier.fields["add"], 0)
			concentrationBonus += sheet.Abilities[text(modifier.fields["addAbility"])].Mod
		}
	}
	if speedBonus != 0 {
		sheet.Speed += speedBonus
		sheet.Derived.Speed = sheet.Speed
	}
	if rows := movementRows(species, sources, modifiers); len(rows) > 0 {
		sheet.Movement = rows
	}
	applyMovement(sheet)
	sheet.ConcentrationSaveBonus = concentrationBonus
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
