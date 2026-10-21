package rules

import (
	"encoding/json"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/pjunak/addon-dnd-engine/character"
)

var builderModeSuffix = regexp.MustCompile(`:(ability|feat|featability)$`)

func BuilderPlan(decisions Object, records Records, ruleset Ruleset) character.Plan {
	modelBase, modelClasses := builderModel(decisions)
	classChoices := collectClassChoices(modelClasses, object(decisions["featureChoices"]), records, ruleset)
	plan := character.Plan{
		SchemaVersion: 1,
		Edition:       ruleset.Edition,
		BaseStats:     modelBase,
		Classes:       modelClasses,
		PointBuy: character.PointBuy{
			Budget: ruleset.Constants.PointBuy.Budget,
			Min:    ruleset.Constants.PointBuy.Min,
			Max:    ruleset.Constants.PointBuy.Max,
			Cost:   copyCounts(ruleset.Constants.PointBuy.Cost),
		},
		AbilityScoreRange:      character.Range{Min: 1, Max: ruleset.Constants.AbilityCapHard},
		ClassChoices:           classChoices,
		CreationChoices:        collectCreationChoices(decisions, records, classChoices),
		CreationAbilityChoices: collectCreationAbilityChoices(decisions, records, ruleset),
	}
	for index := range plan.ClassChoices {
		choice := &plan.ClassChoices[index]
		if choice.Kind != character.ChoiceASIMode {
			continue
		}
		feat := choice.Feat
		selected := recordView(records, "feat", text(object(decisions["featureChoices"])[feat.ID]))
		if object(object(selected["grants"])["abilityScoreIncrease"]) != nil {
			feat.Ability = findAbilityChoice(plan, feat.Ability.ID, decisions, records)
		}
	}
	return plan
}

func NormalizeBuilderDecisions(decisions Object, records Records, ruleset Ruleset) Object {
	copy := cloneObjectDeep(decisions)
	plan := BuilderPlan(copy, records, ruleset)
	manualSaves := object(copy["manualSaveProf"])
	if manualSaves == nil {
		manualSaves = object(copy["saveProf"])
	}
	copy["saveProf"] = cloneObject(manualSaves)
	classes := make([]any, 0, len(plan.Classes))
	for _, current := range plan.Classes {
		classes = append(classes, Object{"classId": current.ClassID, "level": current.Level, "subclass": current.Subclass})
	}
	copy["classes"] = classes
	baseStats := Object{}
	for ability, score := range plan.BaseStats {
		baseStats[ability] = score
	}
	copy["baseStats"] = baseStats
	for key, value := range resolveBuilderChoices(copy, plan, records) {
		copy[key] = value
	}
	acquisitions := []any{}
	for _, acquired := range selectedFeatAcquisitions(copy, records, plan.ClassChoices) {
		record := recordView(records, "feat", acquired.featID)
		source := Object{"type": "feat", "id": acquired.featID}
		if featChoiceOwner(acquired, record) != "feat:"+acquired.featID {
			source["name"] = firstText(record["name"], acquired.featID)
			source["acquisition"] = Object{"id": acquired.id, "name": acquired.name, "classId": acquired.classID, "level": acquired.level}
		}
		acquisitions = append(acquisitions, source)
	}
	copy["featAcquisitions"] = acquisitions
	applyFeatureAbilityBonuses(copy, records)
	for _, choice := range plan.CreationChoices {
		if choice.Default != "" && text(object(copy["featureChoices"])[choice.ID]) == "" {
			playMap(copy, "featureChoices")[choice.ID] = choice.Default
		}
	}
	return copy
}

// Fixed class-feature increases, such as Primal Champion, become ordinary
// ability grants with their printed cap once the class reaches the feature.
func applyFeatureAbilityBonuses(copy Object, records Records) {
	features := recordCatalog(records, "feature")
	for _, raw := range values(copy["classes"]) {
		current := object(raw)
		classID, level, subclass := text(current["classId"]), integer(current["level"], 1), text(current["subclass"])
		for _, feature := range features {
			bonus := object(object(feature["grants"])["abilityScoreBonus"])
			if bonus == nil || text(feature["classId"]) != classID || integer(feature["level"], 0) > level ||
				text(feature["subclassId"]) != "" && text(feature["subclassId"]) != subclass {
				continue
			}
			id := text(feature["id"])
			upsertGrant(copy, "feature:"+id, Object{"type": "feature", "id": id, "level": integer(feature["level"], 1)},
				cloneObject(object(bonus["assign"])), integer(bonus["cap"], 0))
		}
	}
}

func ApplyBuilderChoice(
	decisions Object,
	change Object,
	records Records,
	ruleset Ruleset,
) Object {
	return applyOwnedBuilderChoice(cloneObjectDeep(decisions), change, records, BuilderPlan(decisions, records, ruleset))
}

// The caller owns this decision map. Character input decoding already creates
// one; the public choice operation detaches its caller's map before entering.
func applyOwnedBuilderChoice(copy, change Object, records Records, plan character.Plan) Object {
	choiceID := text(change["choiceId"])
	if choiceID == "" {
		return copy
	}
	if object(copy["featureChoices"]) == nil {
		copy["featureChoices"] = Object{}
	}
	if values(copy["abilityGrants"]) == nil {
		copy["abilityGrants"] = []any{}
	}
	if descriptor := findAbilityChoice(plan, choiceID, copy, records); descriptor != nil && object(change["value"]) != nil {
		applyAbilityGrant(copy, descriptor, object(change["value"]))
		return copy
	}

	baseID := builderModeSuffix.ReplaceAllString(choiceID, "")
	var descriptor *character.PlanChoice
	choices := append(append([]character.PlanChoice{}, plan.ClassChoices...), plan.CreationChoices...)
	for index := range choices {
		if choices[index].ID == baseID || choices[index].ID == choiceID {
			descriptor = &choices[index]
			break
		}
	}
	if descriptor == nil {
		return copy
	}
	value := text(change["value"])
	key := choiceIDForSlot(choiceID, integer(change["slot"], 0), max(1, descriptor.Count))
	setChoiceValue(object(copy["featureChoices"]), key, value)
	if descriptor.Kind != character.ChoiceASIMode {
		return copy
	}
	ability, feat := descriptor.Ability, descriptor.Feat
	if choiceID == descriptor.ID {
		if value != "asi" {
			removeGrant(copy, ability.ID)
		}
		if value != "feat" {
			delete(object(copy["featureChoices"]), feat.ID)
			removeGrant(copy, feat.Ability.ID)
		}
		return copy
	}
	if choiceID == feat.ID {
		removeGrant(copy, feat.Ability.ID)
		selectedFeat := recordView(records, "feat", value)
		increase := object(object(selectedFeat["grants"])["abilityScoreIncrease"])
		eligible := FeatASIFrom(increase)
		if increase != nil && len(eligible) == 1 {
			upsertGrant(copy, feat.Ability.ID, Object{"type": "feat"},
				Object{eligible[0]: max(1, integer(increase["amount"], 1))},
				builderAbilityCap(selectedFeat, *descriptor))
		}
	}
	return copy
}

func builderModel(source Object) (map[string]int, []character.PlanClass) {
	classes := []character.PlanClass{}
	for _, current := range objects(source["classes"]) {
		classes = append(classes, character.PlanClass{ClassID: text(current["classId"]), Level: integer(current["level"], 0), Subclass: text(current["subclass"])})
	}
	if len(classes) == 0 {
		classes = []character.PlanClass{{Level: 1}}
	}
	base := map[string]int{}
	for ability, score := range object(source["baseStats"]) {
		base[ability] = integer(score, 0)
	}
	return base, classes
}

func collectClassChoices(classes []character.PlanClass, selections Object, records Records, ruleset Ruleset) []character.PlanChoice {
	result := make([]character.PlanChoice, 0)
	masteryEnabled := ruleset.Capabilities.WeaponMastery != nil && *ruleset.Capabilities.WeaponMastery
	advancement := ruleset.Builder.AbilityScoreAdvancement
	for classIndex, selected := range classes {
		classID := selected.ClassID
		record := recordView(records, "class", classID)
		if record == nil {
			continue
		}
		classLevel := max(1, selected.Level)
		proficiencies := classProficiencies(record, classIndex == 0)
		classSource := &character.Source{Type: character.SourceClass, ID: classID, Level: 1}
		skills := object(proficiencies["skills"])
		if integer(skills["choose"], 0) > 0 {
			result = append(result, character.PlanChoice{
				ID: "skills:" + classID, Kind: character.ChoiceSkills, Count: max(1, integer(skills["choose"], 1)),
				From: nonNilStrings(skills["from"]), ClassID: classID, Source: classSource,
			})
		}
		if tools := object(proficiencies["toolChoice"]); integer(tools["count"], 0) > 0 {
			result = append(result, character.PlanChoice{
				ID: "tools:" + classID, Kind: character.ChoiceTools, Count: integer(tools["count"], 1),
				From: nonNilStrings(tools["from"]), ClassID: classID, Source: classSource,
			})
		}
		subclassID := selected.Subclass
		subclass := recordView(records, "subclass", subclassID)
		overrides := classChoiceOverrides(classID, classLevel, subclassID, subclass, records)
		context := choiceContext{
			owner: classID, classID: classID, classLevel: classLevel,
			sourceType: character.SourceClass, records: records, masteryEnabled: masteryEnabled, selections: selections, overrides: overrides,
		}
		appendRecordChoices(&result, record["grants"], context)
		context = choiceContext{
			owner: subclassID, classID: classID, classLevel: classLevel, sourceType: character.SourceSubclass,
			fallbackLevel: integer(subclass["subclassLevel"], classSubclassLevel(record)), records: records, masteryEnabled: masteryEnabled, selections: selections, overrides: overrides,
		}
		appendRecordChoices(&result, subclass["grants"], context)
		for _, feature := range recordCatalog(records, "feature") {
			if text(feature["classId"]) != classID || text(feature["category"]) != "" || integer(feature["level"], 0) > classLevel ||
				text(feature["subclassId"]) != "" && text(feature["subclassId"]) != subclassID {
				continue
			}
			appendRecordChoices(&result, feature["grants"], choiceContext{
				owner: text(feature["id"]), classID: classID, classLevel: classLevel, sourceType: character.SourceFeature,
				fallbackLevel: integer(feature["level"], 1), records: records, masteryEnabled: masteryEnabled, selections: selections, overrides: overrides,
			})
		}
		// A chosen option offers its own choices once per slot that chose it.
		for _, option := range chosenOptions(classID, classLevel, subclassID, selections, records) {
			appendRecordChoices(&result, option.feature["grants"], choiceContext{
				owner: text(option.feature["id"]), classID: classID, classLevel: classLevel, sourceType: character.SourceFeature,
				fallbackLevel: min(classLevel, max(1, integer(option.feature["level"], 1))), records: records,
				masteryEnabled: masteryEnabled, selections: selections, multiplier: option.count,
			})
		}
		levels := make(map[int]struct{})
		for _, level := range advancement.BaseLevels {
			levels[level] = struct{}{}
		}
		for _, rawLevel := range values(record["abilityScoreImprovementLevels"]) {
			levels[integer(rawLevel, 0)] = struct{}{}
		}
		ordered := make([]int, 0, len(levels))
		for level := range levels {
			if level > 0 && level <= classLevel {
				ordered = append(ordered, level)
			}
		}
		sort.Ints(ordered)
		for _, level := range ordered {
			id := "asi:" + classID + ":" + jsonNumber(level)
			categories := append([]string(nil), advancement.FeatCategories...)
			categories = append(categories, advancement.CategoriesByLevel[jsonNumber(level)]...)
			result = append(result, character.PlanChoice{
				ID: id, Kind: character.ChoiceASIMode, ClassID: classID, Level: level,
				Source: &character.Source{Type: character.SourceClass, ID: classID, Level: level},
				Ability: &character.PlanChoice{
					ID: id + ":ability", Kind: character.ChoiceAbilityBudget, Eligible: append([]string{}, Abilities[:]...),
					Budget: advancement.Budget, PerAbilityMax: advancement.PerAbilityMax,
				},
				Feat: &character.FeatChoice{
					ID: id + ":feat", Categories: unique(categories),
					Ability:             &character.PlanChoice{ID: id + ":featability", Kind: character.ChoiceAbilityBudget},
					CategoryAbilityCaps: copyCounts(advancement.CategoryAbilityCaps),
				},
			})
		}
	}
	seen := make(map[string]struct{})
	filtered := make([]character.PlanChoice, 0, len(result))
	for _, choice := range result {
		if choice.ID == "" {
			continue
		}
		if _, duplicate := seen[choice.ID]; duplicate {
			continue
		}
		seen[choice.ID] = struct{}{}
		filtered = append(filtered, choice)
	}
	return filtered
}

type choiceContext struct {
	owner, classID            string
	sourceType                character.SourceType
	classLevel, fallbackLevel int
	records                   Records
	masteryEnabled            bool
	selections                Object
	// multiplier scales the slot counts of a repeatable option's own choices.
	multiplier int
	// overrides replace option limits of the class's choices by choice ID.
	overrides map[string]Object
}

// overridableChoiceFields are the option limits a later feature may replace,
// such as Circle Forms raising Wild Shape's challenge rating.
var overridableChoiceFields = []string{"from", "fromByLevel", "filter", "filterByLevel", "count", "countByLevel"}

// classChoiceOverrides collects grants.choiceOverrides ([{choiceId, ...fields}])
// from the class's subclass and its acquired features; a later declaration
// replaces an earlier one's fields.
func classChoiceOverrides(classID string, classLevel int, subclassID string, subclass Object, records Records) map[string]Object {
	overrides := map[string]Object{}
	add := func(grants Object) {
		for _, override := range objects(grants["choiceOverrides"]) {
			id := text(override["choiceId"])
			if id == "" {
				continue
			}
			if overrides[id] == nil {
				overrides[id] = Object{}
			}
			for _, field := range overridableChoiceFields {
				if value, exists := override[field]; exists {
					overrides[id][field] = value
				}
			}
		}
	}
	if subclass != nil && classLevel >= integer(subclass["subclassLevel"], 1) {
		add(object(subclass["grants"]))
	}
	for _, feature := range recordCatalog(records, "feature") {
		if text(feature["classId"]) != classID || text(feature["category"]) != "" || integer(feature["level"], 0) > classLevel ||
			text(feature["subclassId"]) != "" && text(feature["subclassId"]) != subclassID {
			continue
		}
		add(object(feature["grants"]))
	}
	return overrides
}

func appendRecordChoices(result *[]character.PlanChoice, rawGrants any, context choiceContext) {
	grants := selectedChoiceGrants(object(rawGrants), character.Source{Type: context.sourceType, ID: context.owner}, context.selections)
	for _, choice := range objects(grants["choices"]) {
		if text(choice["id"]) == "" {
			continue
		}
		if override := context.overrides[text(choice["id"])]; override != nil {
			choice = cloneObject(choice)
			for field, value := range override {
				choice[field] = value
			}
		}
		sourceLevel := sourceLevelOf(choice, max(1, context.fallbackLevel))
		if sourceLevel > context.classLevel {
			continue
		}
		from := stringsOf(choice["from"])
		repeatable := []string{}
		if from == nil && text(choice["fromCategory"]) != "" {
			from = []string{}
			for _, option := range recordCatalog(context.records, "feature") {
				if text(option["category"]) == text(choice["fromCategory"]) {
					from = append(from, text(option["id"]))
					if truth(option["repeatable"]) {
						repeatable = append(repeatable, text(option["id"]))
					}
				}
			}
		}
		if text(choice["optionKind"]) != "" {
			from = recordChoiceOptions(choice, context.classLevel, context.records)
		}
		kind := choiceKind(choice, from)
		if kind == character.ChoiceWeaponMastery && !context.masteryEnabled {
			continue
		}
		descriptor := character.PlanChoice{
			ID: text(choice["id"]), Kind: kind, Count: choiceCountAt(choice, context.classLevel) * max(1, context.multiplier),
			ClassID: context.classID,
			Source:  &character.Source{Type: context.sourceType, ID: context.owner, Level: sourceLevel},
		}
		copyChoiceFields(&descriptor, choice)
		descriptor.From = from
		if category := text(choice["fromCategory"]); category != "" {
			descriptor.FromCategory = category
			if len(repeatable) > 0 {
				descriptor.RepeatableOptions = repeatable
			}
		}
		*result = append(*result, descriptor)
	}
}

func copyChoiceFields(descriptor *character.PlanChoice, choice Object) {
	descriptor.From = stringsOf(choice["from"])
	descriptor.Category = text(choice["category"])
	descriptor.Prompt = text(choice["prompt"])
	descriptor.Default = text(choice["default"])
	descriptor.ChangeOn = text(choice["changeOn"])
	descriptor.LevelReplacements = integer(choice["levelReplacements"], 0)
	descriptor.OptionKind = text(choice["optionKind"])
}

func collectOriginChoices(source Object, records Records) []character.PlanChoice {
	result := make([]character.PlanChoice, 0)
	for _, origin := range selectedOrigins(source, records) {
		record, typeID := origin.record, origin.kind
		if typeID == character.SourceSpecies {
			if choice := speciesSizeChoice(record); choice != nil {
				result = append(result, *choice)
			}
		}
		recordSource := origin.source()
		if choice := object(record["toolProficiencyChoice"]); choice != nil {
			result = append(result, character.PlanChoice{
				ID: string(typeID) + ":" + text(record["id"]) + ":tool", Kind: character.ChoiceTools,
				Count: max(1, integer(choice["count"], 1)), From: nonNilStrings(choice["from"]),
				Prompt: text(choice["prompt"]), Source: &recordSource,
			})
		}
		owner := string(typeID) + ":" + text(record["id"])
		for _, choice := range objects(object(record["grants"])["choices"]) {
			if text(choice["id"]) == "" {
				continue
			}
			descriptor := character.PlanChoice{
				ID: owner + ":" + text(choice["id"]), Kind: choiceKind(choice, stringsOf(choice["from"])),
				Count: max(1, integer(choice["count"], 1)), Source: &recordSource,
			}
			copyChoiceFields(&descriptor, choice)
			result = append(result, descriptor)
		}
	}
	return result
}

func collectCreationChoices(source Object, records Records, classChoices []character.PlanChoice) []character.PlanChoice {
	result := collectOriginChoices(source, records)
	seen := map[string]bool{}
	for _, acquired := range selectedFeatAcquisitions(source, records, classChoices) {
		for _, choice := range featChoices(acquired, recordView(records, "feat", acquired.featID)) {
			if !seen[choice.ID] {
				seen[choice.ID] = true
				result = append(result, choice)
			}
		}
	}
	return result
}

func collectCreationAbilityChoices(source Object, records Records, ruleset Ruleset) []character.PlanChoice {
	result := make([]character.PlanChoice, 0)
	for _, origin := range selectedOrigins(source, records) {
		config := originGrantPolicy(ruleset.Builder.BackgroundAbilityGrant)
		choiceID := "bgasi"
		if origin.kind == character.SourceSpecies {
			config = originGrantPolicy(ruleset.Builder.SpeciesAbilityGrant)
			choiceID = "speciesasi"
		}
		eligible := stringsOf(origin.record["abilityScores"])
		if config == nil || len(eligible) == 0 {
			continue
		}
		recordSource := origin.source()
		result = append(result, character.PlanChoice{
			ID: choiceID, Kind: character.ChoiceAbilityBudget, Eligible: eligible,
			Budget: config.Budget, PerAbilityMax: config.PerAbilityMax, Source: &recordSource,
		})
	}
	return result
}

// characterOrigin is a selected background or species and its shared record.
type characterOrigin struct {
	kind   character.SourceType
	record Object
}

// source describes the origin as the grant source of its choices.
func (origin characterOrigin) source() character.Source {
	return character.Source{Type: origin.kind, ID: text(origin.record["id"]), Level: 1}
}

func selectedOrigins(source Object, records Records) []characterOrigin {
	result := make([]characterOrigin, 0, 2)
	if record := recordByID(records, "background", text(source["background"])); record != nil {
		result = append(result, characterOrigin{character.SourceBackground, record})
	}
	if record := recordByID(records, "species", text(source["species"])); record != nil {
		result = append(result, characterOrigin{character.SourceSpecies, record})
	}
	return result
}

func resolveBuilderChoices(source Object, plan character.Plan, records Records) Object {
	resolved := Object{
		"skillProficiencies": []any{}, "toolProficiencies": []any{}, "skillExpertise": Object{}, "skillUpgrades": []any{},
		"feats": []any{}, "weaponMasteryChoices": []any{}, "languageProficiencies": []any{},
		"saveProficiencies": []any{}, "weaponProficiencies": []any{}, "armorProficiencies": []any{},
		"damageResistances": []any{}, "conditionImmunities": []any{},
	}
	featIDs := make([]string, 0)
	for _, origin := range selectedOrigins(source, records) {
		featIDs = append(featIDs, text(origin.record["originFeat"]))
	}
	for _, raw := range values(source["feats"]) {
		if id, ok := raw.(string); ok {
			featIDs = append(featIDs, id)
		} else {
			featIDs = append(featIDs, firstText(object(raw)["featId"], object(raw)["id"]))
		}
	}
	for _, feat := range objects(source["extraFeats"]) {
		featIDs = append(featIDs, text(feat["featId"]))
	}
	for _, choice := range append(append([]character.PlanChoice{}, plan.ClassChoices...), plan.CreationChoices...) {
		selected := choiceValues(source, choice)
		switch choice.Kind {
		case character.ChoiceProficiencies:
			for _, value := range selected {
				if strings.HasPrefix(value, "skill:") {
					appendResolved(resolved, "skillProficiencies", strings.TrimPrefix(value, "skill:"))
				} else if strings.HasPrefix(value, "tool:") {
					appendResolved(resolved, "toolProficiencies", strings.TrimPrefix(value, "tool:"))
				}
			}
		case character.ChoiceExpertise:
			for _, value := range selected {
				object(resolved["skillExpertise"])[value] = true
			}
		case character.ChoiceSkillExpertise:
			for _, value := range selected {
				appendResolved(resolved, "skillProficiencies", value)
				object(resolved["skillExpertise"])[value] = true
			}
		case character.ChoiceFeat:
			featIDs = append(featIDs, selected...)
		case character.ChoiceASIMode:
			if text(object(source["featureChoices"])[choice.ID]) == "feat" {
				featIDs = append(featIDs, text(object(source["featureChoices"])[choice.Feat.ID]))
			}
		case character.ChoiceSkills, character.ChoiceTools, character.ChoiceSkillUpgrade, character.ChoiceLanguages,
			character.ChoiceSavingThrows, character.ChoiceWeapons, character.ChoiceArmor, character.ChoiceResistances,
			character.ChoiceImmunities, character.ChoiceWeaponMastery:
			appendResolved(resolved, choiceKindSpecs[choice.Kind].resolved, selected...)
		case character.ChoiceEnumerated, character.ChoiceAbilityBudget, character.ChoiceSize:
		}
	}
	for key, raw := range resolved {
		if selected := stringsOf(raw); selected != nil {
			resolved[key] = anyStrings(unique(selected))
		}
	}
	featValues := make([]any, 0)
	for _, id := range unique(featIDs) {
		featValues = append(featValues, Object{"featId": id})
	}
	resolved["feats"] = featValues
	return resolved
}

func choiceValues(source Object, choice character.PlanChoice) []string {
	choices := object(source["featureChoices"])
	count := max(1, choice.Count)
	result := make([]string, 0, count)
	for index := 0; index < count; index++ {
		key := choice.ID
		if count > 1 {
			key += "#" + jsonNumber(index)
		}
		value := text(choices[key])
		if index == 0 && value == "" {
			value = choice.Default
		}
		// A repeatable option may fill several slots of the same choice.
		if value != "" && (!contains(result, value) || contains(choice.RepeatableOptions, value)) {
			result = append(result, value)
		}
	}
	return result
}

// choiceKind reads a record choice's type; without one, a choice with a
// category and no option list is a feat choice.
func choiceKind(choice Object, from []string) character.ChoiceKind {
	if kind, known := choiceTypeKinds[text(choice["type"])]; known {
		return kind
	}
	if from == nil && text(choice["category"]) != "" {
		return character.ChoiceFeat
	}
	return character.ChoiceEnumerated
}

func sourceLevelOf(choice Object, fallback int) int {
	parts := strings.Split(text(choice["source"]), ":")
	if len(parts) > 1 {
		if parsed := wholeNumber(parts[1], fallback); parsed > 0 {
			return parsed
		}
	}
	return fallback
}

func findAbilityChoice(plan character.Plan, id string, source Object, records Records) *character.PlanChoice {
	for index := range plan.CreationAbilityChoices {
		if plan.CreationAbilityChoices[index].ID == id {
			choice := plan.CreationAbilityChoices[index]
			return &choice
		}
	}
	for _, choice := range plan.ClassChoices {
		if choice.Kind != character.ChoiceASIMode {
			continue
		}
		if choice.Ability.ID == id {
			result := *choice.Ability
			result.Source = &character.Source{Type: character.SourceASI}
			return &result
		}
		if choice.Feat.Ability != nil && choice.Feat.Ability.ID == id {
			featID := text(object(source["featureChoices"])[choice.Feat.ID])
			selected := recordView(records, "feat", featID)
			increase := object(object(selected["grants"])["abilityScoreIncrease"])
			if increase == nil || len(FeatASIFrom(increase)) == 0 {
				return nil
			}
			budget := max(1, integer(increase["amount"], 1))
			return &character.PlanChoice{
				ID: id, Kind: character.ChoiceAbilityBudget, Eligible: FeatASIFrom(increase),
				Budget: budget, PerAbilityMax: budget, Cap: builderAbilityCap(selected, choice),
				Source: &character.Source{Type: character.SourceFeat},
			}
		}
	}
	return nil
}

func applyAbilityGrant(copy Object, descriptor *character.PlanChoice, value Object) {
	ability := text(value["ability"])
	if descriptor.Eligible != nil && !contains(descriptor.Eligible, ability) {
		return
	}
	var existing Object
	for _, grant := range objects(copy["abilityGrants"]) {
		if text(grant["id"]) == descriptor.ID {
			existing = grant
			break
		}
	}
	assign := cloneObject(object(existing["assign"]))
	budget := max(1, descriptor.Budget)
	perAbilityMax := budget
	if descriptor.PerAbilityMax != 0 {
		perAbilityMax = max(1, descriptor.PerAbilityMax)
	}
	others := 0
	for _, current := range Abilities {
		if current != ability {
			others += max(0, integer(assign[current], 0))
		}
	}
	amount := max(0, min(perAbilityMax, min(budget-others, integer(value["amount"], 0))))
	if amount > 0 {
		assign[ability] = amount
	} else {
		delete(assign, ability)
	}
	source := Object{"type": "ability"}
	if descriptor.Source != nil {
		source = toObject(descriptor.Source)
	}
	upsertGrant(copy, descriptor.ID, source, assign, descriptor.Cap)
}

func builderAbilityCap(feat Object, descriptor character.PlanChoice) int {
	explicit := object(object(feat["grants"])["abilityScoreIncrease"])["cap"]
	if explicit != nil {
		return integer(explicit, 0)
	}
	if descriptor.Feat == nil {
		return 0
	}
	return descriptor.Feat.CategoryAbilityCaps[text(feat["category"])]
}

func choiceIDForSlot(id string, slot, count int) string {
	if max(1, count) > 1 {
		return id + "#" + jsonNumber(max(0, slot))
	}
	return id
}

func setChoiceValue(choices Object, key, value string) {
	if value == "" {
		delete(choices, key)
	} else {
		choices[key] = value
	}
}

func removeGrant(copy Object, id string) {
	result := make([]any, 0)
	for _, grant := range objects(copy["abilityGrants"]) {
		if text(grant["id"]) != id {
			result = append(result, grant)
		}
	}
	copy["abilityGrants"] = result
}

func upsertGrant(copy Object, id string, source, assign Object, capValue int) {
	removeGrant(copy, id)
	if len(assign) == 0 {
		return
	}
	grant := Object{"id": id, "source": source, "assign": assign}
	if capValue > 0 {
		grant["cap"] = capValue
	}
	copy["abilityGrants"] = append(values(copy["abilityGrants"]), grant)
}

func originGrantPolicy(body json.RawMessage) *OriginAbilityGrant {
	if len(body) == 0 || string(body) == "false" {
		return nil
	}
	var policy OriginAbilityGrant
	if json.Unmarshal(body, &policy) != nil {
		return nil
	}
	return &policy
}

func appendResolved(result Object, key string, additions ...string) {
	result[key] = append(values(result[key]), anyStrings(additions)...)
}

func cloneObjectDeep(source Object) Object {
	if source == nil {
		return Object{}
	}
	return copyJSONObject(source)
}

func cloneValue(source any) any {
	return copyJSONValue(source)
}

func copyCounts(source map[string]int) map[string]int {
	result := make(map[string]int, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

// nonNilStrings reads a list of strings, empty rather than nil when absent.
func nonNilStrings(value any) []string {
	if result := stringsOf(value); result != nil {
		return result
	}
	return []string{}
}

func jsonNumber(value int) string {
	return strconv.Itoa(value)
}
