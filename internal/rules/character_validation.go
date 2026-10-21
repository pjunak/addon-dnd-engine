package rules

import (
	"encoding/json"
	"fmt"
	"github.com/pjunak/addon-dnd-engine/character"
	"sort"
	"strings"
	"time"
)

func validateCharacter(input character.Inputs, decisions Object, records Records, profile Ruleset, result *character.Result) {
	validateCharacterConditions(input, records, profile, result)
	validateCharacterQuickUse(input, result)
	validateCharacterStorage(input, result)
	validateCharacterCarrying(input, result)
	validateCharacterPlacement(input, records, result)
	block := func(id, target, message string) {
		addCharacterIssue(result, id, target, message, character.SeverityBlocker, nil)
	}
	sheet := result.Sheet.Calculation
	if len(input.Build.Levels) == 0 || len(input.Build.Levels) > profile.Constants.Character.MaximumLevel {
		block("levels", "levels", fmt.Sprintf("Choose between 1 and %d ordered class levels.", profile.Constants.Character.MaximumLevel))
	}
	if len(input.Build.BaseScores) != 6 {
		block("base-scores", "abilities", "Assign all six base ability scores.")
	}
	switch input.Build.Method {
	case character.MethodPointBuy:
		spent := 0
		for _, ability := range Abilities {
			score, ok := input.Build.BaseScores[ability]
			cost, exists := profile.Constants.PointBuy.Cost[jsonNumber(score)]
			if !ok || !exists {
				block("base-score:"+ability, "abilities", "Base scores must use the ruleset's point-buy range.")
			}
			spent += cost
		}
		if spent != profile.Constants.PointBuy.Budget {
			block("point-buy", "abilities", fmt.Sprintf("Allocate %d ability points; currently %d.", profile.Constants.PointBuy.Budget, spent))
		}
	case character.MethodArray:
		scores := []int{}
		for _, ability := range Abilities {
			scores = append(scores, input.Build.BaseScores[ability])
		}
		sort.Ints(scores)
		expected := append([]int(nil), profile.Constants.Character.StandardArray...)
		sort.Ints(expected)
		if fmt.Sprint(scores) != fmt.Sprint(expected) {
			block("standard-array", "abilities", fmt.Sprintf("Assign the ruleset standard array once each: %v.", profile.Constants.Character.StandardArray))
		}
	case character.MethodRolled:
		validateCharacterRolls(input, result, *profile.Constants.Character)
	default:
		block("creation-method", "abilities", "Choose point buy, standard array or recorded rolls.")
	}
	seen := map[string]bool{}
	for _, level := range input.Build.Levels {
		if level.ID == "" || seen[level.ID] {
			block("level-id:"+level.ID, "levels", "Level decisions need unique IDs.")
		}
		seen[level.ID] = true
		if class := recordView(records, "class", level.ClassID); class == nil {
			block("class:"+level.ID, level.ID, "Choose an available class.")
		} else if hitDieSize(text(class["hitDie"])) < 1 {
			block("class-hit-die:"+level.ClassID, level.ID, "This class record has no valid hit die.")
		}
	}
	choiceIDs := map[string]bool{}
	choiceValues := map[string]bool{}
	for _, choice := range input.Build.Choices {
		key := characterChoiceKey(choice)
		if choiceIDs[key] {
			block("duplicate-choice:"+key, choice.ID, "This choice slot was selected more than once.")
		}
		choiceIDs[key] = true
		descriptor := findCharacterChoice(result.Plan, choice.ID, decisions, records)
		if descriptor == nil {
			block("unavailable-choice:"+key, choice.ID, "This earlier selection is no longer granted. Remove it or choose a replacement.")
			continue
		}
		count := max(1, descriptor.Count)
		if choice.Slot < 0 || choice.Slot >= count {
			block("choice-count:"+key, choice.ID, "This selection exceeds its allowed choice count.")
		}
		var value any
		_ = json.Unmarshal(choice.Value, &value)
		if descriptor.Kind == character.ChoiceAbilityBudget {
			assignment := object(value)
			total, valid := 0, assignment != nil && choice.Slot == 0
			for ability, raw := range assignment {
				amount, ok := raw.(float64)
				valid = valid && ok && amount == float64(int(amount)) && amount >= 0 && int(amount) <= descriptor.PerAbilityMax && contains(Abilities[:], ability)
				if descriptor.Eligible != nil {
					valid = valid && contains(descriptor.Eligible, ability)
				}
				total += int(amount)
			}
			if !valid || total != descriptor.Budget {
				block("ability-assignment:"+key, choice.ID, "Assign exactly the available increase budget to eligible abilities, within each ability's limit.")
			}
			continue
		}
		if _, ok := value.(string); !ok {
			block("choice-type:"+key, choice.ID, "Choose one eligible option.")
			continue
		}
		if selected, ok := value.(string); ok && selected != "" {
			if valueKey := choice.ID + "/" + selected; choiceValues[valueKey] && !contains(descriptor.RepeatableOptions, selected) {
				block("duplicate-option:"+key, choice.ID, "Choose distinct options for each slot.")
			} else {
				choiceValues[valueKey] = true
			}
			options := builderChoiceOptions(*descriptor, sheet, records)
			if kind := descriptor.Kind; kind == character.ChoiceExpertise || kind == character.ChoiceSkillExpertise || isProficiencyChoice(*descriptor) || descriptor.DistinctAcrossAcquisitions {
				options = result.Guidance.Choices[choice.ID].Options
			}
			valid := descriptor.Kind == character.ChoiceASIMode && (selected == "asi" || selected == "feat")
			for _, option := range options {
				if option.ID == selected {
					valid = true
				}
			}
			if !valid {
				block("invalid-option:"+key, choice.ID, "The selected option is not eligible for this choice.")
			}
		}
	}
	for _, item := range input.Play.Inventory {
		if item.ID == "" || seen["item:"+item.ID] {
			block("item-id:"+item.ID, "inventory", "Inventory instances need unique IDs.")
		}
		seen["item:"+item.ID] = true
		if item.Quantity < 0 {
			block("quantity:"+item.ID, "inventory", "Item quantity cannot be negative.")
		}
		if item.Quantity < 1 {
			if item.Location == "equipped" {
				block("equipped-empty:"+item.ID, "inventory", "An absent item cannot remain equipped.")
			}
			if item.Attuned {
				block("attunement-empty:"+item.ID, "inventory", "An absent item cannot remain attuned.")
			}
		}
		if item.Reference == nil {
			mechanics := characterItemHasGrant(item, input)
			if (item.Attuned || item.Location == "equipped") && !mechanics {
				block("custom-item:"+item.ID, "inventory", "A narrative item needs a catalog definition or explicit DM mechanics before it can be equipped or attuned.")
			}
			continue
		}
		record := recordByID(records, item.Reference.Kind, item.Reference.ID)
		if record == nil {
			block("item-source:"+item.ID, "inventory", "An item source is unavailable. Keep the saved sheet or review a replacement.")
			continue
		}
		validateCharacterItemMechanics(item, record, input, records, result)
		if item.Attuned {
			identity := "attuned:" + item.Reference.Kind + ":" + item.Reference.ID
			if profile.Constants.Character.UniqueAttunement && seen[identity] {
				block("attunement-duplicate:"+item.ID, "inventory", "Only one copy of this item can be attuned.")
			}
			seen[identity] = true
			if !truth(record["attunement"]) {
				block("attunement-ineligible:"+item.ID, "inventory", "This item does not declare an attunement requirement.")
			}
			validateCharacterAttunementPrerequisite(item, record, input, records, profile, sheet, result)
		}
	}
	equipped := map[string]string{}
	for _, item := range input.Play.Inventory {
		if item.Reference == nil || item.Location != "equipped" || item.Quantity < 1 {
			continue
		}
		record := recordByID(records, item.Reference.Kind, item.Reference.ID)
		slot := characterEquipmentSlot(record)
		if slot != "worn" {
			if equipped[slot] != "" {
				block("equipped:"+slot, "inventory", "Only one item may occupy the "+slot+" slot.")
			}
			equipped[slot] = item.ID
		}
	}
	if sheet.Attunement.Count > sheet.Attunement.Limit {
		block("attunement-capacity", "inventory", "Choose which items remain attuned, or record a DM capacity grant.")
	}
	maximum := sheet.Derived.MaxHP
	if input.Play.HP < 0 || input.Play.HP > maximum {
		block("hp-bounds", "hp", fmt.Sprintf("Current HP must be between 0 and %d; review the required correction.", maximum))
	}
	if input.Play.TemporaryHP < 0 {
		block("temporary-hp", "temporaryHp", "Temporary HP cannot be negative.")
	}
	if saved := input.Play.Concentration; saved != nil && (saved.Ref == "" || saved.SaveDC < 0 || saved.SaveDC > 100) {
		block("concentration", "concentration", "End the concentration; its saved spell or save DC is invalid.")
	}
	for _, key := range sortedKeys(input.Play.Currency) {
		value := input.Play.Currency[key]
		if !contains([]string{"cp", "sp", "ep", "gp", "pp"}, key) || value < 0 {
			block("currency:"+key, "currency", "Currency must use supported coins and non-negative amounts.")
		}
	}
	resources := map[string]int{}
	for _, resource := range sheet.Resources {
		resources[resource.Key] = resource.Max
	}
	for _, key := range sortedKeys(input.Play.ResourceUses) {
		used := input.Play.ResourceUses[key]
		maximum, exists := resources[key]
		if used < 0 || !exists && used != 0 || exists && used > maximum {
			block("resource:"+key, "resources", "Resolve the spent resource against its current capacity: "+key)
		}
	}
	for _, grant := range input.Grants {
		if grant.ID == "" || seen["grant:"+grant.ID] {
			block("grant-id:"+grant.ID, "grants", "DM grants need unique IDs.")
		}
		seen["grant:"+grant.ID] = true
		if !grant.Condition.Valid() {
			block("grant-condition:"+grant.ID, "grants", "Choose a supported grant condition.")
		}
		if grant.ActorID == "" || strings.TrimSpace(grant.Reason) == "" || grant.EffectiveLevel < 1 {
			block("grant-provenance:"+grant.ID, "grants", "DM grants require an actor, reason and effective level.")
		}
		if grant.ExpiresAt != "" {
			if _, err := time.Parse(time.RFC3339, grant.ExpiresAt); err != nil {
				block("grant-expiry:"+grant.ID, "grants", "Enter a valid expiration date and time with timezone.")
			}
		}
		for _, effect := range grant.Effects {
			if !effect.Mode.Valid() {
				block("effect-mode:"+grant.ID, "grants", "Choose a supported effect operation.")
			}
			if effect.Value < -100000 || effect.Value > 100000 {
				block("effect-value:"+grant.ID, "grants", "Effect amount exceeds the supported range.")
			}
			if (effect.Target == character.TargetAbilityScore || effect.Target == character.TargetAbilityCap) && !contains(Abilities[:], effect.Key) {
				block("effect-key:"+grant.ID, "grants", "Choose a supported ability.")
			}
			if !validCharacterEffect(effect) {
				block("effect-target:"+grant.ID, "grants", "This DM effect target is not supported.")
			}
		}
		if grant.Feat != nil && characterGrantActive(grant, input) {
			record := recordByID(records, "feat", grant.Feat.ID)
			if grant.Feat.Kind != "feat" || record == nil {
				block("grant-feat:"+grant.ID, "grants", "The granted feat is unavailable.")
			}
		}
	}
	validateCharacterProgression(input, records, profile, result)
	validateCharacterSubclasses(input, records, result)
	validateCharacterSpells(input, records, result)
}

func validateCharacterRolls(input character.Inputs, result *character.Result, policy CharacterPolicy) {
	selected := map[string]bool{}
	for _, roll := range input.Build.Rolls {
		valid := contains(Abilities[:], roll.Ability) && !selected[roll.Ability] && len(roll.Dice) == policy.RollDice && len(roll.Kept) == policy.RollKeep
		selected[roll.Ability] = true
		used := map[int]bool{}
		total := 0
		for _, die := range roll.Dice {
			valid = valid && die >= 1 && die <= policy.RollSides
		}
		for _, index := range roll.Kept {
			if index < 0 || index >= len(roll.Dice) || used[index] {
				valid = false
				continue
			}
			used[index] = true
			total += roll.Dice[index]
		}
		sorted := append([]int(nil), roll.Dice...)
		sort.Sort(sort.Reverse(sort.IntSlice(sorted)))
		if len(sorted) >= policy.RollKeep {
			highest := 0
			for _, die := range sorted[:policy.RollKeep] {
				highest += die
			}
			valid = valid && total == highest
		}
		if !valid || input.Build.BaseScores[roll.Ability] != total {
			addCharacterIssue(result, "roll:"+roll.ID, "abilities", fmt.Sprintf("Record %d d%d and retain the highest %d results without rerolling during recalculation.", policy.RollDice, policy.RollSides, policy.RollKeep), character.SeverityBlocker, nil)
		}
	}
	if len(selected) != 6 {
		addCharacterIssue(result, "roll-count", "abilities", "Record a roll for each ability.", character.SeverityBlocker, nil)
	}
}

func findCharacterChoice(plan character.Plan, id string, decisions Object, records Records) *character.PlanChoice {
	if ability := findAbilityChoice(plan, id, decisions, records); ability != nil {
		return ability
	}
	for _, group := range [][]character.PlanChoice{plan.ClassChoices, plan.CreationChoices, plan.CreationAbilityChoices} {
		for _, choice := range group {
			if choice.ID == id {
				return &choice
			}
			if choice.Feat != nil && choice.Feat.ID == id {
				feat := choice.Feat.Choice()
				return &feat
			}
		}
	}
	return nil
}

type progressionChecks struct {
	classes bool
	feats   bool
	featID  string
	// levels, when set, limits a featID check to these level indexes. Each
	// listed acquisition level must also list the level before it.
	levels map[int]bool
}

func validateCharacterProgression(input character.Inputs, records Records, profile Ruleset, result *character.Result) {
	validateSelectedProgression(input, records, profile, result, progressionChecks{classes: true, feats: true})
}

// Editor options consume only one family of progression issues. Keep the same
// acquisition-order validation, without calculating issues the caller discards.
func validateSelectedProgression(input character.Inputs, records Records, profile Ruleset, result *character.Result, checks progressionChecks) {
	seenClasses := map[string]bool{}
	seenFeats := map[string]bool{}
	seenAcquisitions := map[string]string{}
	for index, level := range input.Build.Levels {
		if checks.levels != nil && !checks.levels[index] {
			continue
		}
		if !checks.feats && (index == 0 || seenClasses[level.ClassID]) {
			seenClasses[level.ClassID] = true
			continue
		}
		prefix := replacementPrefix(input, index+1)
		detached := character.Result{Issues: []character.Issue{}}
		var acquisitions []featAcquisition
		var authored Object
		if checks.feats {
			authored = characterDecisions(prefix, records, profile, &detached)
			_, classes := builderModel(authored)
			acquisitions = selectedFeatAcquisitions(authored, records, collectClassChoices(classes, object(authored["featureChoices"]), records, profile))
		}
		if checks.classes && index > 0 && !seenClasses[level.ClassID] {
			classIDs := []string{level.ClassID}
			for id := range seenClasses {
				classIDs = append(classIDs, id)
			}
			sort.Strings(classIDs)
			beforeLevel := replacementPrefix(input, index)
			priorDecisions := NormalizeBuilderDecisions(characterDecisions(beforeLevel, records, profile, &detached), records, profile)
			applyCharacterAbilityEffects(beforeLevel, priorDecisions, &detached, profile, records)
			beforeSheet := Hydrate(priorDecisions, records, profile).Sheet
			for _, id := range classIDs {
				record := recordByID(records, "class", id)
				requirement := record["multiclassPrerequisites"]
				if requirement == nil && object(record["multiclassRequirements"]) != nil {
					all := []any{}
					for names, score := range object(record["multiclassRequirements"]) {
						anyOf := []any{}
						for _, name := range strings.Split(names, "|") {
							anyOf = append(anyOf, Object{"abilities": Object{name: score}})
						}
						all = append(all, Object{"any": anyOf})
					}
					requirement = Object{"all": all}
				}
				if requirement == nil {
					requirement = Object{"text": "Multiclass prerequisite is not structured."}
				}
				validatePrerequisite(requirement, fmt.Sprintf("multiclass:%s:%s", level.ID, id), level.ID, beforeLevel, beforeSheet, result, &character.Reference{Kind: "class", ID: id})
			}
		}
		seenClasses[level.ClassID] = true
		if !checks.feats {
			continue
		}
		newFeats := map[string]bool{}
		var decisions Object
		var plan *character.Plan
		featCounts := map[string]int{}
		currentAcquisitions := map[string]string{}
		for _, acquired := range acquisitions {
			featCounts[acquired.featID]++
			currentAcquisitions[acquired.id] = acquired.featID
		}
		for _, acquired := range acquisitions {
			id := acquired.featID
			if seenAcquisitions[acquired.id] == id {
				continue
			}
			if checks.featID != "" && checks.featID != id {
				continue
			}
			record := recordByID(records, "feat", id)
			if !featRepetitionAllowed(record, featCounts[id]) {
				message := "This feat cannot be acquired more than once."
				if object(record["repeatable"]) != nil {
					message = "No distinct options remain for another acquisition of this feat."
					if conditionalFeatChoice(record) == nil {
						message = "This feat's repeatability rule is unsupported."
					}
				}
				addCharacterIssue(result, "feat:"+id, "choices", message, character.SeverityBlocker, &character.Reference{Kind: "feat", ID: id})
			}
			if emptyPrerequisite(record["prerequisites"]) {
				continue
			}
			if plan == nil {
				// Earlier levels still discover acquisitions and repetition, but
				// need no normalized sheet unless a new relevant feat requires it.
				decisions = NormalizeBuilderDecisions(authored, records, profile)
				for _, feat := range objects(decisions["feats"]) {
					id := text(feat["featId"])
					if !seenFeats[id] {
						newFeats[id] = true
					}
				}
				built := BuilderPlan(decisions, records, profile)
				plan = &built
			}
			prior := cloneObjectDeep(decisions)
			remaining := []any{}
			for _, current := range objects(prior["feats"]) {
				if !newFeats[text(current["featId"])] {
					remaining = append(remaining, current)
				}
			}
			prior["feats"] = remaining
			grants := []any{}
			for _, grant := range objects(prior["abilityGrants"]) {
				remove := strings.HasSuffix(acquired.id, ":feat") && text(grant["id"]) == strings.TrimSuffix(acquired.id, ":feat")+":featability"
				for featID := range newFeats {
					remove = remove || characterGrantFromFeat(grant, featID, *plan, decisions)
				}
				if !remove {
					grants = append(grants, grant)
				}
			}
			prior["abilityGrants"] = grants
			applyCharacterAbilityEffects(prefix, prior, &detached, profile, records)
			before := Hydrate(prior, records, profile).Sheet
			validatePrerequisite(record["prerequisites"], "feat:"+id, "choices", prefix, before, result, &character.Reference{Kind: "feat", ID: id})
		}
		seenAcquisitions = currentAcquisitions
		seenFeats = map[string]bool{}
		for id := range featCounts {
			seenFeats[id] = true
		}
	}
}

func characterGrantFromFeat(grant Object, featID string, plan character.Plan, decisions Object) bool {
	source := object(grant["source"])
	if text(source["type"]) == "feat" && text(source["id"]) == featID {
		return true
	}
	for _, choice := range plan.ClassChoices {
		feat := choice.Feat
		if feat != nil && feat.Ability != nil && feat.Ability.ID == text(grant["id"]) && text(object(decisions["featureChoices"])[feat.ID]) == featID {
			return true
		}
	}
	return false
}

// Predicates are a small serializable vocabulary. Narrative prerequisites need
// an exact recorded waiver; the engine never parses prose into authority.
func validatePrerequisite(value any, id, target string, input character.Inputs, sheet *character.Calculation, result *character.Result, reference *character.Reference) {
	if emptyPrerequisite(value) {
		return
	}
	for _, grant := range activeCharacterGrants(input) {
		if contains(grant.Waivers, id) {
			addCharacterIssue(result, "waived:"+id, target, "DM given: "+grant.Name+" waives this requirement.", character.SeverityInfo, reference)
			return
		}
	}
	matched, known := prerequisiteMatches(object(value), sheet)
	if matched && known {
		return
	}
	message := "The prerequisite is not met."
	if !known {
		message = "This prerequisite needs recorded DM adjudication: " + firstText(object(value)["text"], "unsupported rule predicate")
	}
	addCharacterIssue(result, id, target, message, character.SeverityBlocker, reference)
}
func emptyPrerequisite(value any) bool {
	return value == nil || object(value) != nil && len(object(value)) == 0
}

func prerequisiteMatches(value Object, sheet *character.Calculation) (bool, bool) {
	if sheet == nil {
		sheet = &character.Calculation{}
	}
	return prerequisiteMatchesDepth(value, sheet, 0)
}
func prerequisiteMatchesDepth(value Object, sheet *character.Calculation, depth int) (bool, bool) {
	if len(value) == 0 || depth > 12 {
		return false, false
	}
	valid := true
	for key, raw := range value {
		switch key {
		case "level":
			if !positivePredicateNumber(raw) {
				return false, false
			}
			valid = valid && sheet.TotalLevel >= integer(raw, 0)
		case "abilities":
			if len(object(raw)) == 0 {
				return false, false
			}
			for ability, minScore := range object(raw) {
				if !contains(Abilities[:], ability) || !positivePredicateNumber(minScore) {
					return false, false
				}
				valid = valid && sheet.Abilities[ability].Score >= integer(minScore, 0)
			}
		case "all", "any":
			children := objects(raw)
			if len(children) == 0 {
				return false, false
			}
			match := key == "all"
			for _, child := range children {
				ok, known := prerequisiteMatchesDepth(child, sheet, depth+1)
				if !known {
					return false, false
				}
				if key == "all" {
					match = match && ok
				} else {
					match = match || ok
				}
			}
			valid = valid && match
		case "classes":
			if len(object(raw)) == 0 {
				return false, false
			}
			for classID, minimum := range object(raw) {
				if classID == "" || !positivePredicateNumber(minimum) {
					return false, false
				}
				found := false
				for _, class := range sheet.Classes {
					found = found || class.ClassID == classID && class.Level >= integer(minimum, 0)
				}
				valid = valid && found
			}
		case "spellcaster":
			if required, ok := raw.(bool); !ok || !required {
				return false, false
			}
			valid = valid && hasIntrinsicSpellcasting(sheet)
		case "spellcastingFeature":
			// A class or subclass Spellcasting or Pact Magic feature; spells
			// granted by feats, species or items do not count.
			if required, ok := raw.(bool); !ok || !required {
				return false, false
			}
			valid = valid && len(sheet.Spellcasting.PerClass) > 0
		case "feat", "featCategory", "withoutFeatCategory":
			wanted := text(raw)
			if wanted == "" {
				return false, false
			}
			found := false
			for _, feat := range sheet.FeatIdentities {
				found = found || key == "feat" && feat.ID == wanted || key != "feat" && feat.Category == wanted
			}
			valid = valid && found == (key != "withoutFeatCategory")
		case "weaponTraining":
			wanted := text(raw)
			if wanted != "simple" && wanted != "martial" {
				return false, false
			}
			valid = valid && contains(sheet.Proficiencies.Weapons, wanted)
		case "armorTraining":
			wanted := strings.TrimSuffix(text(raw), "s")
			if !contains([]string{"light", "medium", "heavy", "shield"}, wanted) {
				return false, false
			}
			found := false
			for _, armor := range sheet.Proficiencies.Armor {
				found = found || strings.TrimSuffix(strings.ToLower(armor), "s") == wanted
			}
			valid = valid && found
		case "feature":
			if text(raw) == "" {
				return false, false
			}
			found := false
			for _, feature := range sheet.Features {
				found = found || feature.ID == text(raw)
			}
			valid = valid && found
		default:
			return false, false
		}
	}
	return valid, true
}
func positivePredicateNumber(value any) bool {
	switch v := value.(type) {
	case int:
		return v > 0 && v <= 1000
	case float64:
		return v > 0 && v <= 1000 && v == float64(int(v))
	}
	return false
}
