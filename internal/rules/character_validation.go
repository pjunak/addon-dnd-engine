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
	block := func(id, target, message string) { addCharacterIssue(result, id, target, message, "blocker", nil) }
	if len(input.Build.Levels) == 0 || len(input.Build.Levels) > profile.Constants.Character.MaximumLevel {
		block("levels", "levels", fmt.Sprintf("Choose between 1 and %d ordered class levels.", profile.Constants.Character.MaximumLevel))
	}
	if len(input.Build.BaseScores) != 6 {
		block("base-scores", "abilities", "Assign all six base ability scores.")
	}
	switch input.Build.Method {
	case "point-buy":
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
	case "array":
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
	case "rolled":
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
		if recordByID(records, "class", level.ClassID) == nil {
			block("class:"+level.ID, level.ID, "Choose an available class.")
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
		plan := Object(result.Plan)
		descriptor := findCharacterChoice(plan, choice.ID, decisions, records, profile)
		if descriptor == nil {
			block("unavailable-choice:"+key, choice.ID, "This earlier selection is no longer granted. Remove it or choose a replacement; it remains in history.")
			continue
		}
		count := max(1, integer(descriptor["count"], 1))
		if choice.Slot < 0 || choice.Slot >= count {
			block("choice-count:"+key, choice.ID, "This selection exceeds its allowed choice count.")
		}
		var value any
		_ = json.Unmarshal(choice.Value, &value)
		if text(descriptor["kind"]) == "abilityBudget" {
			assignment := object(value)
			total, valid := 0, assignment != nil && choice.Slot == 0
			for ability, raw := range assignment {
				amount, ok := raw.(float64)
				valid = valid && ok && amount == float64(int(amount)) && amount >= 0 && int(amount) <= integer(descriptor["perAbilityMax"], 0) && contains(Abilities[:], ability)
				if descriptor["eligible"] != nil {
					valid = valid && contains(stringsOf(descriptor["eligible"]), ability)
				}
				total += int(amount)
			}
			if !valid || total != integer(descriptor["budget"], 0) {
				block("ability-assignment:"+key, choice.ID, "Assign exactly the available increase budget to eligible abilities, within each ability's limit.")
			}
			continue
		}
		if _, ok := value.(string); !ok {
			block("choice-type:"+key, choice.ID, "Choose one eligible option.")
			continue
		}
		if selected, ok := value.(string); ok && selected != "" {
			if valueKey := choice.ID + "/" + selected; choiceValues[valueKey] {
				block("duplicate-option:"+key, choice.ID, "Choose distinct options for each slot.")
			} else {
				choiceValues[valueKey] = true
			}
			options := builderChoiceOptions(descriptor, Object(result.Sheet), records)
			valid := text(descriptor["kind"]) == "asiMode" && (selected == "asi" || selected == "feat")
			for _, option := range objects(options) {
				if text(option["id"]) == selected {
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
		if item.Reference == nil {
			mechanics := false
			for _, grant := range input.Grants {
				mechanics = mechanics || grant.ID == item.GrantID && grant.ItemID == item.ID && grant.Active && len(grant.Effects) > 0
			}
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
		validateCharacterItemMechanics(item, record, input, result)
		if item.Attuned {
			identity := "attuned:" + item.Reference.Kind + ":" + item.Reference.ID
			if profile.Constants.Character.UniqueAttunement && seen[identity] {
				block("attunement-duplicate:"+item.ID, "inventory", "Only one copy of this item can be attuned.")
			}
			seen[identity] = true
			if item.Quantity < 1 {
				block("attunement-empty:"+item.ID, "inventory", "An absent item cannot remain attuned.")
			}
			if !truth(record["attunement"]) {
				block("attunement-ineligible:"+item.ID, "inventory", "This item does not declare an attunement requirement.")
			}
			validatePrerequisite(record["attunementPrerequisites"], "attunement:"+item.ID, "inventory", input, Object(result.Sheet), result, item.Reference)
		}
	}
	equipped := map[string]string{}
	for _, item := range input.Play.Inventory {
		if item.Reference == nil || item.Location != "equipped" || item.Quantity < 1 {
			continue
		}
		record := recordByID(records, item.Reference.Kind, item.Reference.ID)
		slot := text(record["armorType"])
		if slot != "" {
			if slot != "shield" {
				slot = "armor"
			}
			if equipped[slot] != "" {
				block("equipped:"+slot, "inventory", "Only one item may occupy the "+slot+" slot.")
			}
			equipped[slot] = item.ID
		}
	}
	attunement := object(result.Sheet["attunement"])
	if integer(attunement["count"], 0) > integer(attunement["limit"], 0) {
		block("attunement-capacity", "inventory", "Choose which items remain attuned, or record a DM capacity grant.")
	}
	maximum := integer(object(result.Sheet["derived"])["maxHp"], 0)
	if input.Play.HP < 0 || input.Play.HP > maximum {
		block("hp-bounds", "hp", fmt.Sprintf("Current HP must be between 0 and %d; review the required correction.", maximum))
	}
	if input.Play.TemporaryHP < 0 {
		block("temporary-hp", "temporaryHp", "Temporary HP cannot be negative.")
	}
	for key, value := range input.Play.Currency {
		if !contains([]string{"cp", "sp", "ep", "gp", "pp"}, key) || value < 0 {
			block("currency:"+key, "currency", "Currency must use supported coins and non-negative amounts.")
		}
	}
	resources := map[string]int{}
	for _, resource := range objects(result.Sheet["resources"]) {
		resources[text(resource["key"])] = integer(resource["max"], 0)
	}
	for key, used := range input.Play.ResourceUses {
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
		if !contains([]string{"always", "equipped", "attuned"}, grant.Condition) {
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
			if !contains([]string{"set", "add", "minimum", "maximum"}, effect.Mode) {
				block("effect-mode:"+grant.ID, "grants", "Choose a supported effect operation.")
			}
			if effect.Value < -100000 || effect.Value > 100000 {
				block("effect-value:"+grant.ID, "grants", "Effect amount exceeds the supported range.")
			}
			if (effect.Target == "abilityScore" || effect.Target == "abilityCap") && !contains(Abilities[:], effect.Key) {
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
			addCharacterIssue(result, "roll:"+roll.ID, "abilities", fmt.Sprintf("Record %d d%d and retain the highest %d results without rerolling during recalculation.", policy.RollDice, policy.RollSides, policy.RollKeep), "blocker", nil)
		}
	}
	if len(selected) != 6 {
		addCharacterIssue(result, "roll-count", "abilities", "Record a roll for each ability.", "blocker", nil)
	}
}

func findCharacterChoice(plan Object, id string, decisions Object, records Records, profile Ruleset) Object {
	if ability := findAbilityChoice(plan, id, decisions, records); ability != nil {
		return ability
	}
	for _, group := range []string{"classChoices", "creationChoices", "creationAbilityChoices"} {
		for _, choice := range objects(plan[group]) {
			if text(choice["id"]) == id {
				return choice
			}
			if feat := object(choice["feat"]); text(feat["id"]) == id {
				copy := cloneObjectDeep(feat)
				copy["kind"] = "feat"
				return copy
			}
		}
	}
	return nil
}

func validateCharacterProgression(input character.Inputs, records Records, profile Ruleset, result *character.Result) {
	seenClasses := map[string]bool{}
	seenFeats := map[string]bool{}
	for index, level := range input.Build.Levels {
		prefix := cloneCharacter(input)
		prefix.Build.Levels = prefix.Build.Levels[:index+1]
		detached := character.Result{Issues: []character.Issue{}}
		decisions := NormalizeBuilderDecisions(characterDecisions(prefix, records, profile, &detached), records, profile)
		if index > 0 && !seenClasses[level.ClassID] {
			classIDs := []string{level.ClassID}
			for id := range seenClasses {
				classIDs = append(classIDs, id)
			}
			sort.Strings(classIDs)
			beforeLevel := cloneCharacter(prefix)
			beforeLevel.Build.Levels = beforeLevel.Build.Levels[:index]
			priorDecisions := NormalizeBuilderDecisions(characterDecisions(beforeLevel, records, profile, &detached), records, profile)
			applyCharacterAbilityEffects(beforeLevel, priorDecisions, &detached, profile, records)
			beforeSheet := Hydrate(priorDecisions, records, &profile).Sheet
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
		newFeats := map[string]bool{}
		for _, id := range stringsOf(decisions["feats"]) {
			if !seenFeats[id] {
				newFeats[id] = true
			}
		}
		plan := BuilderPlan(decisions, records, profile)
		for _, id := range stringsOf(decisions["feats"]) {
			if seenFeats[id] {
				continue
			}
			seenFeats[id] = true
			record := recordByID(records, "feat", id)
			prior := cloneObjectDeep(decisions)
			remaining := []any{}
			for _, current := range stringsOf(prior["feats"]) {
				if !newFeats[current] {
					remaining = append(remaining, current)
				}
			}
			prior["feats"] = remaining
			grants := []any{}
			for _, grant := range objects(prior["abilityGrants"]) {
				remove := false
				for featID := range newFeats {
					remove = remove || characterGrantFromFeat(grant, featID, plan, decisions)
				}
				if !remove {
					grants = append(grants, grant)
				}
			}
			prior["abilityGrants"] = grants
			applyCharacterAbilityEffects(prefix, prior, &detached, profile, records)
			before := Hydrate(prior, records, &profile).Sheet
			validatePrerequisite(record["prerequisites"], "feat:"+id, "choices", prefix, before, result, &character.Reference{Kind: "feat", ID: id})
		}
	}
}

func characterGrantFromFeat(grant Object, featID string, plan, decisions Object) bool {
	source := object(grant["source"])
	if text(source["type"]) == "feat" && text(source["id"]) == featID {
		return true
	}
	for _, choice := range objects(plan["classChoices"]) {
		feat := object(choice["feat"])
		if text(object(feat["ability"])["id"]) == text(grant["id"]) && text(object(decisions["featureChoices"])[text(feat["id"])]) == featID {
			return true
		}
	}
	return false
}

// Predicates are a small serializable vocabulary. Narrative prerequisites need
// an exact recorded waiver; the engine never parses prose into authority.
func validatePrerequisite(value any, id, target string, input character.Inputs, sheet Object, result *character.Result, reference *character.Reference) {
	if value == nil || object(value) != nil && len(object(value)) == 0 {
		return
	}
	for _, grant := range activeCharacterGrants(input) {
		if contains(grant.Waivers, id) {
			addCharacterIssue(result, "waived:"+id, target, "DM given: "+grant.Name+" waives this requirement.", "info", reference)
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
	addCharacterIssue(result, id, target, message, "blocker", reference)
}
func prerequisiteMatches(value Object, sheet Object) (bool, bool) {
	return prerequisiteMatchesDepth(value, sheet, 0)
}
func prerequisiteMatchesDepth(value Object, sheet Object, depth int) (bool, bool) {
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
			valid = valid && integer(sheet["totalLevel"], 0) >= integer(raw, 0)
		case "abilities":
			if len(object(raw)) == 0 {
				return false, false
			}
			for ability, minScore := range object(raw) {
				if !contains(Abilities[:], ability) || !positivePredicateNumber(minScore) {
					return false, false
				}
				valid = valid && integer(object(object(sheet["abilities"])[ability])["score"], 0) >= integer(minScore, 0)
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
		case "feature":
			if text(raw) == "" {
				return false, false
			}
			found := false
			for _, feature := range objects(sheet["features"]) {
				found = found || text(feature["id"]) == text(raw)
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
