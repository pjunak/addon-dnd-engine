package rules

import (
	"encoding/json"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var builderSlotSuffix = regexp.MustCompile(`#\d+$`)
var builderModeSuffix = regexp.MustCompile(`:(ability|feat|featability)$`)

func BuilderPlan(decisions Object, records Records, ruleset Ruleset) Object {
	modelBase, modelClasses := builderModel(decisions, records)
	return Object{
		"schemaVersion": 1,
		"edition":       ruleset.Edition,
		"baseStats":     modelBase,
		"classes":       modelClasses,
		"pointBuy": Object{
			"budget": ruleset.Constants.PointBuy.Budget,
			"min":    ruleset.Constants.PointBuy.Min,
			"max":    ruleset.Constants.PointBuy.Max,
			"cost":   stringIntObject(ruleset.Constants.PointBuy.Cost),
		},
		"abilityScoreRange": Object{
			"min": 1, "max": ruleset.Constants.AbilityCapHard,
		},
		"classChoices":           collectClassChoices(modelClasses, records, ruleset),
		"creationChoices":        collectCreationChoices(decisions, records),
		"creationAbilityChoices": collectCreationAbilityChoices(decisions, records, ruleset),
	}
}

func NormalizeBuilderDecisions(decisions Object, records Records, ruleset Ruleset) Object {
	copy := cloneObjectDeep(decisions)
	plan := BuilderPlan(copy, records, ruleset)
	manualSaves := object(copy["manualSaveProf"])
	if manualSaves == nil {
		manualSaves = object(copy["saveProf"])
	}
	copy["saveProf"] = cloneObject(manualSaves)
	copy["classes"] = cloneArray(values(plan["classes"]))
	copy["baseStats"] = cloneObjectDeep(object(plan["baseStats"]))
	for key, value := range resolveBuilderChoices(copy, plan, records) {
		copy[key] = value
	}
	return copy
}

func ReconcileBuilderDecisions(decisions Object, records Records, ruleset Ruleset) Object {
	copy := cloneObjectDeep(decisions)
	plan := BuilderPlan(copy, records, ruleset)
	valid := make(map[string]struct{})
	for _, group := range []any{plan["classChoices"], plan["creationChoices"], plan["creationAbilityChoices"]} {
		for _, choice := range objects(group) {
			valid[text(choice["id"])] = struct{}{}
		}
	}
	choices := object(copy["featureChoices"])
	if choices == nil {
		choices = Object{}
	}
	for key := range choices {
		if _, exists := valid[builderBaseID(key)]; !exists {
			delete(choices, key)
		}
	}
	copy["featureChoices"] = choices
	grants := make([]any, 0)
	for _, grant := range objects(copy["abilityGrants"]) {
		if _, exists := valid[builderBaseID(text(grant["id"]))]; exists {
			grants = append(grants, grant)
		}
	}
	copy["abilityGrants"] = grants
	return copy
}

func ApplyBuilderChoice(
	decisions Object,
	change Object,
	records Records,
	ruleset Ruleset,
) Object {
	copy := cloneObjectDeep(decisions)
	choiceID := text(change["choiceId"])
	if choiceID == "" {
		return copy
	}
	plan := BuilderPlan(copy, records, ruleset)
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
	var descriptor Object
	for _, choice := range append(objects(plan["classChoices"]), objects(plan["creationChoices"])...) {
		if text(choice["id"]) == baseID || text(choice["id"]) == choiceID {
			descriptor = choice
			break
		}
	}
	if descriptor == nil {
		return copy
	}
	value := text(change["value"])
	key := choiceIDForSlot(choiceID, integer(change["slot"], 0), integer(descriptor["count"], 1))
	setChoiceValue(object(copy["featureChoices"]), key, value)
	if text(descriptor["kind"]) != "asiMode" {
		return copy
	}
	ability := object(descriptor["ability"])
	feat := object(descriptor["feat"])
	featAbility := object(feat["ability"])
	if choiceID == text(descriptor["id"]) {
		if value != "asi" {
			removeGrant(copy, text(ability["id"]))
		}
		if value != "feat" {
			delete(object(copy["featureChoices"]), text(feat["id"]))
			removeGrant(copy, text(featAbility["id"]))
		}
		return copy
	}
	if choiceID == text(feat["id"]) {
		removeGrant(copy, text(featAbility["id"]))
		selectedFeat := recordByID(records, "feat", value)
		increase := object(object(selectedFeat["grants"])["abilityScoreIncrease"])
		eligible := FeatASIFrom(increase)
		if increase != nil && len(eligible) == 1 {
			upsertGrant(copy, text(featAbility["id"]), Object{"type": "feat"},
				Object{eligible[0]: max(1, integer(increase["amount"], 1))},
				builderAbilityCap(selectedFeat, descriptor))
		}
	}
	return copy
}

func builderModel(source Object, records Records) (Object, []any) {
	baseStats := object(source["baseStats"])
	if len(baseStats) == 0 {
		baseStats = object(source["abilities"])
	}
	classes := cloneArray(values(source["classes"]))
	if len(classes) == 0 {
		if className := text(source["className"]); className != "" {
			record := recordByName(records, "class", className)
			classes = []any{Object{
				"classId": text(record["id"]), "level": max(1, integer(source["level"], 1)),
				"subclass": text(source["subclass"]),
			}}
		} else {
			classes = []any{Object{"classId": "", "level": 1, "subclass": ""}}
		}
	}
	return cloneObjectDeep(baseStats), classes
}

func collectClassChoices(classes []any, records Records, ruleset Ruleset) []any {
	result := make([]any, 0)
	masteryEnabled := ruleset.Capabilities.WeaponMastery != nil && *ruleset.Capabilities.WeaponMastery
	advancement := ruleset.Builder.AbilityScoreAdvancement
	for classIndex, rawSelected := range classes {
		selected := object(rawSelected)
		classID := text(selected["classId"])
		record := recordByID(records, "class", classID)
		if record == nil {
			continue
		}
		classLevel := max(1, integer(selected["level"], 1))
		starting := object(record["startingProficiencies"])
		reduced := Object(nil)
		if classIndex > 0 {
			reduced = object(record["multiclassProficiencies"])
		}
		skills := object(starting["skills"])
		if reduced != nil && reduced["skills"] != nil {
			skills = object(reduced["skills"])
		}
		if integer(skills["choose"], 0) > 0 {
			result = append(result, Object{
				"id": "skills:" + classID, "kind": "skills", "count": max(1, integer(skills["choose"], 1)),
				"from": anyStrings(stringsOf(skills["from"])), "classId": classID,
				"source": Object{"type": "class", "id": classID, "level": 1},
			})
		}
		context := choiceContext{
			owner: classID, classID: classID, classLevel: classLevel,
			sourceType: "class", records: records, masteryEnabled: masteryEnabled,
		}
		appendRecordChoices(&result, record["grants"], context)
		subclassID := text(selected["subclass"])
		subclass := recordByID(records, "subclass", subclassID)
		context = choiceContext{
			owner: subclassID, classID: classID, classLevel: classLevel, sourceType: "subclass",
			fallbackLevel: integer(subclass["subclassLevel"], 3), records: records, masteryEnabled: masteryEnabled,
		}
		appendRecordChoices(&result, subclass["grants"], context)
		for _, feature := range recordList(records, "feature") {
			if text(feature["classId"]) != classID || integer(feature["level"], 0) > classLevel ||
				text(feature["subclassId"]) != "" && text(feature["subclassId"]) != subclassID {
				continue
			}
			appendRecordChoices(&result, feature["grants"], choiceContext{
				owner: text(feature["id"]), classID: classID, classLevel: classLevel, sourceType: "feature",
				fallbackLevel: integer(feature["level"], 1), records: records, masteryEnabled: masteryEnabled,
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
			result = append(result, Object{
				"id": id, "kind": "asiMode", "classId": classID, "level": level,
				"source": Object{"type": "class", "id": classID, "level": level},
				"ability": Object{
					"id": id + ":ability", "kind": "abilityBudget", "eligible": anyStrings(Abilities[:]),
					"budget": advancement.Budget, "perAbilityMax": advancement.PerAbilityMax,
				},
				"feat": Object{
					"id": id + ":feat", "categories": anyStrings(unique(categories)),
					"ability":             Object{"id": id + ":featability", "kind": "abilityBudget"},
					"categoryAbilityCaps": stringIntObject(advancement.CategoryAbilityCaps),
				},
			})
		}
	}
	seen := make(map[string]struct{})
	filtered := make([]any, 0, len(result))
	for _, raw := range result {
		id := text(object(raw)["id"])
		if id == "" {
			continue
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		filtered = append(filtered, raw)
	}
	return filtered
}

type choiceContext struct {
	owner, classID, sourceType string
	classLevel, fallbackLevel  int
	records                    Records
	masteryEnabled             bool
}

func appendRecordChoices(result *[]any, rawGrants any, context choiceContext) {
	grants := object(rawGrants)
	for _, choice := range objects(grants["choices"]) {
		if text(choice["id"]) == "" {
			continue
		}
		sourceLevel := sourceLevelOf(choice, max(1, context.fallbackLevel))
		if sourceLevel > context.classLevel {
			continue
		}
		from := choice["from"]
		if values(from) == nil && text(choice["fromCategory"]) != "" {
			ids := make([]string, 0)
			for _, option := range recordList(context.records, "feature") {
				if text(option["category"]) == text(choice["fromCategory"]) {
					ids = append(ids, text(option["id"]))
				}
			}
			from = anyStrings(ids)
		}
		kind := choiceKind(choice, from)
		if kind == "weaponMastery" && !context.masteryEnabled {
			continue
		}
		count := max(1, integer(choice["count"], 1))
		selectedLevel := -1
		for level, amount := range object(choice["countByLevel"]) {
			parsed := integer(level, 0)
			if parsed <= context.classLevel && parsed > selectedLevel {
				selectedLevel = parsed
				count = max(1, integer(amount, count))
			}
		}
		*result = append(*result, Object{
			"id": text(choice["id"]), "kind": kind, "count": count, "from": cloneValue(from),
			"category": nullableText(choice["category"]), "prompt": nullableText(choice["prompt"]),
			"default": nullableText(choice["default"]), "changeOn": nullableText(choice["changeOn"]),
			"classId": context.classID,
			"source":  Object{"type": context.sourceType, "id": context.owner, "level": sourceLevel},
		})
	}
}

func collectCreationChoices(source Object, records Records) []any {
	result := make([]any, 0)
	appendChoice := func(choice Object, owner string, recordSource Object) {
		if text(choice["id"]) == "" {
			return
		}
		result = append(result, Object{
			"id": owner + ":" + text(choice["id"]), "kind": choiceKind(choice, choice["from"]),
			"count": max(1, integer(choice["count"], 1)), "from": cloneValue(choice["from"]),
			"category": nullableText(choice["category"]), "prompt": nullableText(choice["prompt"]),
			"default": nullableText(choice["default"]), "changeOn": nullableText(choice["changeOn"]),
			"source": recordSource,
		})
	}
	for _, origin := range selectedOrigins(source, records) {
		record := object(origin["record"])
		typeID := text(origin["type"])
		if choice := object(record["toolProficiencyChoice"]); choice != nil {
			result = append(result, Object{
				"id": typeID + ":" + text(record["id"]) + ":tool", "kind": "tools",
				"count": max(1, integer(choice["count"], 1)), "from": anyStrings(stringsOf(choice["from"])),
				"prompt": nullableText(choice["prompt"]), "source": origin["source"],
			})
		}
		for _, choice := range objects(object(record["grants"])["choices"]) {
			appendChoice(choice, typeID+":"+text(record["id"]), object(origin["source"]))
		}
	}
	for _, featID := range selectedFeatIDs(source, records) {
		feat := recordByID(records, "feat", featID)
		for _, choice := range objects(object(feat["grants"])["choices"]) {
			appendChoice(choice, "feat:"+featID, Object{"type": "feat", "id": featID, "level": 1})
		}
	}
	return result
}

func collectCreationAbilityChoices(source Object, records Records, ruleset Ruleset) []any {
	result := make([]any, 0)
	for _, origin := range selectedOrigins(source, records) {
		typeID := text(origin["type"])
		config := originGrantPolicy(ruleset.Builder.BackgroundAbilityGrant)
		legacyID := "bgasi"
		if typeID == "species" {
			config = originGrantPolicy(ruleset.Builder.SpeciesAbilityGrant)
			legacyID = "speciesasi"
		}
		eligible := stringsOf(object(origin["record"])["abilityScores"])
		if config == nil || len(eligible) == 0 {
			continue
		}
		result = append(result, Object{
			"id": legacyID, "kind": "abilityBudget", "eligible": anyStrings(eligible),
			"budget": integer(config["budget"], 0), "perAbilityMax": integer(config["perAbilityMax"], 0),
			"source": origin["source"],
		})
	}
	return result
}

func selectedOrigins(source Object, records Records) []Object {
	result := make([]Object, 0, 2)
	if selected := text(source["background"]); selected != "" {
		if record := selectedRecord(selected, records, "background"); record != nil {
			result = append(result, Object{
				"type": "background", "record": record,
				"source": Object{"type": "background", "id": text(record["id"]), "level": 1},
			})
		}
	}
	speciesID := firstText(source["species"], source["race"])
	if speciesID != "" {
		if record := selectedRecord(speciesID, records, "species"); record != nil {
			result = append(result, Object{
				"type": "species", "record": record,
				"source": Object{"type": "species", "id": text(record["id"]), "level": 1},
			})
		}
	}
	return result
}

func selectedFeatIDs(source Object, records Records) []string {
	ids := make([]string, 0)
	for _, origin := range selectedOrigins(source, records) {
		ids = append(ids, text(object(origin["record"])["originFeat"]))
	}
	for _, raw := range values(source["feats"]) {
		if id, ok := raw.(string); ok {
			ids = append(ids, id)
		} else {
			ids = append(ids, firstText(object(raw)["featId"], object(raw)["id"]))
		}
	}
	for _, feat := range objects(source["extraFeats"]) {
		ids = append(ids, text(feat["featId"]))
	}
	for key, value := range object(source["featureChoices"]) {
		if strings.HasSuffix(key, ":feat") {
			ids = append(ids, text(value))
		}
	}
	return unique(ids)
}

func resolveBuilderChoices(source, plan Object, records Records) Object {
	resolved := Object{
		"skillProficiencies": []any{}, "toolProficiencies": []any{}, "skillExpertise": Object{},
		"feats": []any{}, "weaponMasteryChoices": []any{}, "languageProficiencies": []any{},
		"saveProficiencies": []any{}, "weaponProficiencies": []any{}, "armorProficiencies": []any{},
		"damageResistances": []any{}, "conditionImmunities": []any{},
	}
	featIDs := make([]string, 0)
	for _, origin := range selectedOrigins(source, records) {
		featIDs = append(featIDs, text(object(origin["record"])["originFeat"]))
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
	choices := append(objects(plan["classChoices"]), objects(plan["creationChoices"])...)
	for _, choice := range choices {
		selected := choiceValues(source, choice)
		switch text(choice["kind"]) {
		case "skills":
			appendResolved(resolved, "skillProficiencies", selected...)
		case "tools":
			appendResolved(resolved, "toolProficiencies", selected...)
		case "proficiencies":
			for _, value := range selected {
				if strings.HasPrefix(value, "skill:") {
					appendResolved(resolved, "skillProficiencies", strings.TrimPrefix(value, "skill:"))
				} else if strings.HasPrefix(value, "tool:") {
					appendResolved(resolved, "toolProficiencies", strings.TrimPrefix(value, "tool:"))
				}
			}
		case "expertise":
			for _, value := range selected {
				object(resolved["skillExpertise"])[value] = true
			}
		case "skillExpertise":
			for _, value := range selected {
				appendResolved(resolved, "skillProficiencies", value)
				object(resolved["skillExpertise"])[value] = true
			}
		case "languages":
			appendResolved(resolved, "languageProficiencies", selected...)
		case "savingThrows":
			appendResolved(resolved, "saveProficiencies", selected...)
		case "weapons":
			appendResolved(resolved, "weaponProficiencies", selected...)
		case "armor":
			appendResolved(resolved, "armorProficiencies", selected...)
		case "resistances":
			appendResolved(resolved, "damageResistances", selected...)
		case "immunities":
			appendResolved(resolved, "conditionImmunities", selected...)
		case "weaponMastery":
			appendResolved(resolved, "weaponMasteryChoices", selected...)
		case "feat":
			featIDs = append(featIDs, selected...)
		case "asiMode":
			if text(object(source["featureChoices"])[text(choice["id"])]) == "feat" {
				featIDs = append(featIDs, text(object(source["featureChoices"])[text(object(choice["feat"])["id"])]))
			}
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

func choiceValues(source, choice Object) []string {
	choices := object(source["featureChoices"])
	count := max(1, integer(choice["count"], 1))
	result := make([]string, 0, count)
	for index := 0; index < count; index++ {
		key := text(choice["id"])
		if count > 1 {
			key += "#" + jsonNumber(index)
		}
		value := text(choices[key])
		if index == 0 && value == "" {
			value = text(choice["default"])
		}
		if value != "" && !contains(result, value) {
			result = append(result, value)
		}
	}
	return result
}

func choiceKind(choice Object, from any) string {
	kinds := map[string]string{
		"skillProficiency": "skills", "expertise": "expertise", "skillExpertise": "skillExpertise",
		"toolProficiency": "tools", "proficiency": "proficiencies", "weaponMastery": "weaponMastery",
		"language": "languages", "savingThrowProficiency": "savingThrows", "weaponProficiency": "weapons",
		"armorProficiency": "armor", "damageResistance": "resistances", "conditionImmunity": "immunities",
	}
	if kind := kinds[text(choice["type"])]; kind != "" {
		return kind
	}
	if text(choice["type"]) == "feat" || values(from) == nil && text(choice["category"]) != "" {
		return "feat"
	}
	return "enumerated"
}

func sourceLevelOf(choice Object, fallback int) int {
	parts := strings.Split(text(choice["source"]), ":")
	if len(parts) > 1 {
		if parsed := integer(parts[1], fallback); parsed > 0 {
			return parsed
		}
	}
	return fallback
}

func findAbilityChoice(plan Object, id string, source Object, records Records) Object {
	for _, choice := range objects(plan["creationAbilityChoices"]) {
		if text(choice["id"]) == id {
			return choice
		}
	}
	for _, choice := range objects(plan["classChoices"]) {
		if text(choice["kind"]) != "asiMode" {
			continue
		}
		ability := object(choice["ability"])
		if text(ability["id"]) == id {
			result := cloneObject(ability)
			result["source"] = Object{"type": "asi"}
			return result
		}
		feat := object(choice["feat"])
		featAbility := object(feat["ability"])
		if text(featAbility["id"]) == id {
			featID := text(object(source["featureChoices"])[text(feat["id"])])
			selected := recordByID(records, "feat", featID)
			increase := object(object(selected["grants"])["abilityScoreIncrease"])
			budget := max(1, integer(increase["amount"], 1))
			return Object{
				"id": text(featAbility["id"]), "kind": "abilityBudget", "eligible": anyStrings(FeatASIFrom(increase)),
				"budget": budget, "perAbilityMax": budget, "cap": builderAbilityCap(selected, choice),
				"source": Object{"type": "feat"},
			}
		}
	}
	return nil
}

func applyAbilityGrant(copy, descriptor, value Object) {
	ability := text(value["ability"])
	eligible := stringsOf(descriptor["eligible"])
	if descriptor["eligible"] != nil && !contains(eligible, ability) {
		return
	}
	var existing Object
	for _, grant := range objects(copy["abilityGrants"]) {
		if text(grant["id"]) == text(descriptor["id"]) {
			existing = grant
			break
		}
	}
	assign := cloneObject(object(existing["assign"]))
	budget := max(1, integer(descriptor["budget"], 1))
	perAbilityMax := max(1, integer(descriptor["perAbilityMax"], budget))
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
	upsertGrant(copy, text(descriptor["id"]), firstObject(descriptor["source"], Object{"type": "ability"}),
		assign, integer(descriptor["cap"], 0))
}

func builderAbilityCap(feat, descriptor Object) int {
	explicit := object(object(feat["grants"])["abilityScoreIncrease"])["cap"]
	if explicit != nil {
		return integer(explicit, 0)
	}
	return integer(object(object(descriptor["feat"])["categoryAbilityCaps"])[text(feat["category"])], 0)
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

func originGrantPolicy(body json.RawMessage) Object {
	if len(body) == 0 || string(body) == "false" {
		return nil
	}
	value, _ := DecodeObject(body)
	return value
}

func builderBaseID(value string) string {
	return builderModeSuffix.ReplaceAllString(builderSlotSuffix.ReplaceAllString(value, ""), "")
}

func appendResolved(result Object, key string, additions ...string) {
	result[key] = append(values(result[key]), anyStrings(additions)...)
}

func cloneObjectDeep(source Object) Object {
	if source == nil {
		return Object{}
	}
	body, _ := json.Marshal(source)
	result, _ := DecodeObject(body)
	return result
}

func cloneArray(source []any) []any {
	if source == nil {
		return []any{}
	}
	body, _ := json.Marshal(source)
	var result []any
	_ = json.Unmarshal(body, &result)
	return result
}

func cloneValue(source any) any {
	if source == nil {
		return nil
	}
	body, _ := json.Marshal(source)
	var result any
	_ = json.Unmarshal(body, &result)
	return result
}

func stringIntObject(source map[string]int) Object {
	result := make(Object, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func jsonNumber(value int) string {
	return strconv.Itoa(value)
}
