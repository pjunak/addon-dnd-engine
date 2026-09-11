package rules

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"github.com/pjunak/addon-dnd-engine/character"
)

// EvaluateCharacter evaluates detached decisions. Completion, eligibility and
// numerical constraints all belong here, never in the persistence or UI layer.
func EvaluateCharacter(input character.Inputs, records Records, profile Ruleset) character.Result {
	input = cloneCharacter(input)
	result := character.Result{ContractVersion: character.ContractVersion, Inputs: input, Sheet: map[string]any{}, Guidance: map[string]any{}, Plan: map[string]any{}, SpellOptions: map[string]any{}, Explanations: map[string]character.Explanation{}, Evidence: []character.Evidence{}, Issues: []character.Issue{}}
	if profile.Constants.Character == nil {
		addCharacterIssue(&result, "character-policy", "rules", "The rules profile does not define character creation policy.", "blocker", nil)
		return result
	}
	tracked := newCharacterRecords(records)
	decisions := characterDecisions(input, tracked, profile, &result)
	normalized := NormalizeBuilderDecisions(decisions, tracked, profile)
	applyCharacterAbilityEffects(input, normalized, &result, profile, tracked)
	tracked.selected = map[string]character.Evidence{}
	hydrated := Hydrate(normalized, tracked, &profile)
	result.Sheet = hydrated.Sheet
	for _, ability := range Abilities {
		object(object(hydrated.Sheet["abilities"])[ability])["cap"] = object(normalized["scoreCaps"])[ability]
	}
	applyCharacterHitDice(input, hydrated.Sheet, tracked, &result, profile)
	applyCharacterEffects(input, hydrated.Sheet, &result, tracked)
	normalized["resourceUses"] = characterRemainingResources(input, hydrated.Sheet)
	// Catalog discovery and legality checks must not retain every unselected
	// option as character evidence. The calculation reads above are retained.
	untracked := newCharacterRecords(records)
	result.Plan = BuilderPlan(decisions, untracked, profile)
	result.Guidance = BuilderGuidance(decisions, Object(result.Plan), untracked, profile)
	result.SpellOptions = SpellOptions(normalized, hydrated.Sheet, untracked, profile)
	for _, caster := range objects(result.SpellOptions["classes"]) {
		remaining := characterSpellReplacements(input, untracked, text(caster["classId"]))
		caster["canSwap"] = remaining > 0
		caster["replacementsRemaining"] = remaining
	}
	validateCharacter(input, decisions, untracked, profile, &result)
	validateCharacterSpellProgression(input, untracked, profile, &result)
	for _, section := range objects(result.Guidance["sections"]) {
		for _, issue := range objects(section["issues"]) {
			addCharacterIssue(&result, "choice:"+text(issue["id"]), text(issue["id"]), text(issue["label"]), "blocker", nil)
		}
	}
	for _, warning := range hydrated.Warnings {
		addCharacterIssue(&result, "rules-warning:"+warning, "build", warning, "warning", nil)
	}
	captureCharacterSources(hydrated.Sheet, tracked)
	for _, selections := range []map[string][]string{input.Build.Spells.Cantrips, input.Build.Spells.Spellbook, input.Build.Spells.GrantChoices, input.Play.PreparedSpells} {
		for _, ids := range selections {
			for _, id := range ids {
				recordByID(tracked, "spell", id)
			}
		}
	}
	result.Evidence = tracked.evidence()
	compactCharacterProjection(hydrated.Sheet)
	result.Explanations = characterExplanations(input, hydrated.Sheet, result.Evidence)
	for key, explanation := range result.Explanations {
		if explanation.Unit != "" {
			explanation.Unit = profile.Constants.Character.DistanceUnit
			result.Explanations[key] = explanation
		}
	}
	appendEffectExplanations(input, &result, records)
	result.Ready = true
	for _, issue := range result.Issues {
		if issue.Severity == "blocker" {
			result.Ready = false
		}
	}
	if len(input.Build.BaseScores) != 6 || len(input.Build.Levels) == 0 || input.Build.Species == "" || input.Build.Background == "" {
		result.Sheet = map[string]any{"status": "needs-choices"}
		result.Explanations = map[string]character.Explanation{"status": {Label: "Calculated values need choices", Formula: "Choose base abilities, species, background and the first class level to calculate the character.", Value: nil, Terms: []character.Term{}, Sources: []character.Reference{}}}
	}
	return result
}

func characterDecisions(input character.Inputs, records Records, profile Ruleset, result *character.Result) Object {
	build, play := input.Build, input.Play
	decisions := Object{"baseStats": toObject(build.BaseScores), "species": build.Species, "lineage": build.Lineage, "background": build.Background, "classes": []any{}, "featureChoices": Object{}, "abilityGrants": []any{}, "extraFeats": []any{}, "manualScores": build.Method != "point-buy",
		"cantrips": toObject(build.Spells.Cantrips), "spellbook": toObject(build.Spells.Spellbook), "grantChoices": toObject(build.Spells.GrantChoices), "grantCastingAbilities": toObject(build.Spells.CastingAbilities), "spellSwaps": toArray(build.Spells.Swaps),
		"hp": play.HP, "tempHp": play.TemporaryHP, "inventory": []any{}, "currency": toObject(play.Currency), "resourceUses": toObject(play.ResourceUses), "activeFeatures": toObject(play.ActiveFeatures), "preparedSpells": toObject(play.PreparedSpells)}
	classes := []any{}
	indexes := map[string]int{}
	for _, level := range build.Levels {
		index, exists := indexes[level.ClassID]
		if !exists {
			index = len(classes)
			indexes[level.ClassID] = index
			classes = append(classes, Object{"classId": level.ClassID, "level": 0, "subclass": build.Subclasses[level.ClassID]})
		}
		class := object(classes[index])
		class["level"] = integer(class["level"], 0) + 1
	}
	decisions["classes"] = classes
	for _, grant := range input.Grants {
		if !characterGrantActive(grant, input) {
			continue
		}
		if grant.Feat != nil {
			decisions["extraFeats"] = append(values(decisions["extraFeats"]), Object{"id": grant.ID, "featId": grant.Feat.ID, "name": grant.Name})
		}
	}
	choices := append([]character.Choice(nil), build.Choices...)
	sort.SliceStable(choices, func(i, j int) bool {
		if choices[i].ID == choices[j].ID {
			return choices[i].Slot < choices[j].Slot
		}
		return choices[i].ID < choices[j].ID
	})
	for _, choice := range choices {
		var value any
		if json.Unmarshal(choice.Value, &value) != nil {
			addCharacterIssue(result, "invalid-choice:"+choice.ID, choice.ID, "Choice value is invalid.", "blocker", nil)
			continue
		}
		if assignment := object(value); assignment != nil {
			for _, ability := range Abilities {
				if amount, exists := assignment[ability]; exists {
					decisions = ApplyBuilderChoice(decisions, Object{"choiceId": choice.ID, "value": Object{"ability": ability, "amount": amount}}, records, profile)
				}
			}
		} else {
			decisions = ApplyBuilderChoice(decisions, Object{"choiceId": choice.ID, "slot": choice.Slot, "value": value}, records, profile)
		}
	}
	for _, item := range play.Inventory {
		if item.Quantity < 1 {
			continue
		}
		entry := Object{"id": item.ID, "name": item.Name, "qty": item.Quantity, "location": item.Location, "attuned": item.Attuned, "notes": item.Notes}
		if item.SpellID != "" {
			entry["spellRef"] = item.SpellID
		}
		if item.Reference != nil {
			entry["ref"] = item.Reference.ID
			entry["kind"] = item.Reference.Kind
			entry["itemRef"] = item.Reference.ID
			entry["catalogId"] = item.Reference.ID
		}
		decisions["inventory"] = append(values(decisions["inventory"]), entry)
	}
	return decisions
}

func ApplyCharacterPlay(input character.Inputs, change Object, records Records, profile Ruleset) (character.Result, error) {
	current := EvaluateCharacter(input, records, profile)
	if !current.Ready {
		return current, fmt.Errorf("Resolve the character's blocking choices before applying play changes.")
	}
	input = cloneCharacter(input)
	switch text(change["operation"]) {
	case "spend-hit-die":
		resource := findPlayResource(Object(current.Sheet), text(change["key"]))
		roll, ok := change["result"].(float64)
		size := hitDieSize(text(resource["die"]))
		id := text(change["rollId"])
		if len(change) != 4 || !ok || roll != float64(int(roll)) || roll < 1 || roll > float64(size) || id == "" || text(resource["kind"]) != "hitdice" {
			return current, fmt.Errorf("Record a valid result for an available hit die.")
		}
		for _, old := range input.Play.Rolls {
			if old.ID == id {
				return current, fmt.Errorf("This roll has already been recorded.")
			}
		}
		key := text(resource["key"])
		if input.Play.ResourceUses[key] >= integer(resource["max"], 0) {
			return current, fmt.Errorf("No hit dice remain in this pool.")
		}
		if input.Play.ResourceUses == nil {
			input.Play.ResourceUses = map[string]int{}
		}
		input.Play.ResourceUses[key]++
		healing := max(profile.Constants.Character.MinimumHPGain, int(roll)+integer(object(object(current.Sheet["abilities"])["CON"])["mod"], 0))
		input.Play.HP = min(integer(object(current.Sheet["derived"])["maxHp"], 0), input.Play.HP+healing)
		input.Play.Rolls = append(input.Play.Rolls, character.PlayRoll{ID: id, Resource: key, Die: size, Result: int(roll), At: input.Play.AsOf, Origin: "recorded"})
	case "set-hp", "heal", "damage", "set-temporary-hp":
		amount, ok := change["amount"].(float64)
		if !ok || amount != float64(int(amount)) || amount < 0 || amount > 1_000_000 || len(change) != 2 {
			return current, fmt.Errorf("Enter a non-negative whole amount.")
		}
		maximum := integer(object(current.Sheet["derived"])["maxHp"], 0)
		switch text(change["operation"]) {
		case "set-hp":
			if int(amount) > maximum {
				return current, fmt.Errorf("Current HP cannot exceed the effective maximum (%d).", maximum)
			}
			input.Play.HP = int(amount)
		case "heal":
			input.Play.HP = min(maximum, input.Play.HP+int(amount))
		case "damage":
			absorbed := min(input.Play.TemporaryHP, int(amount))
			input.Play.TemporaryHP -= absorbed
			input.Play.HP = max(0, input.Play.HP-(int(amount)-absorbed))
		case "set-temporary-hp":
			input.Play.TemporaryHP = int(amount)
		}
	default:
		if text(change["operation"]) == "swap-spell" && characterSpellReplacements(input, records, text(change["classId"])) < 1 {
			return current, fmt.Errorf("No recorded level-up spell replacements remain for this class.")
		}
		if text(change["operation"]) == "toggle-feature" && truth(change["enabled"]) && !input.Play.ActiveFeatures[text(change["key"])] {
			for _, activation := range objects(current.Sheet["activations"]) {
				if text(activation["key"]) == text(change["key"]) {
					if key := text(object(activation["resource"])["key"]); key != "" {
						resource := findPlayResource(Object(current.Sheet), key)
						if resource == nil || input.Play.ResourceUses[key] >= integer(resource["max"], 0) {
							return current, fmt.Errorf("No uses of this feature remain.")
						}
						if input.Play.ResourceUses == nil {
							input.Play.ResourceUses = map[string]int{}
						}
						input.Play.ResourceUses[key]++
					}
				}
			}
		}
		if text(change["operation"]) == "copy-spell" {
			if id := text(change["acquisitionId"]); id == "" {
				return current, fmt.Errorf("Spell copying requires a recorded acquisition ID.")
			}
			for _, acquisition := range input.Build.Spells.Acquisitions {
				if acquisition.ID == text(change["acquisitionId"]) {
					return current, fmt.Errorf("This spell acquisition is already recorded.")
				}
			}
			if scrollID := text(change["scrollId"]); scrollID != "" {
				found := false
				for _, item := range input.Play.Inventory {
					found = found || item.ID == scrollID && item.Quantity > 0 && item.SpellID == text(change["ref"])
				}
				if !found {
					return current, fmt.Errorf("Choose an owned scroll explicitly associated with this spell.")
				}
			}
		}
		decisions := characterDecisions(input, records, profile, &current)
		decisions["resourceUses"] = characterRemainingResources(input, Object(current.Sheet))
		command := cloneObjectDeep(change)
		if text(change["operation"]) == "copy-spell" {
			delete(command, "acquisitionId")
		}
		next, err := applyPlayWithSheet(decisions, command, records, profile, Object(current.Sheet))
		if err != nil {
			return current, err
		}
		if text(change["operation"]) == "copy-spell" {
			spell := recordByID(records, "spell", text(change["ref"]))
			input.Build.Spells.Acquisitions = append(input.Build.Spells.Acquisitions, character.SpellAcquisition{ID: text(change["acquisitionId"]), ClassID: text(change["classId"]), SpellID: text(change["ref"]), Level: len(input.Build.Levels), At: input.Play.AsOf, CostGP: ScrollCopyCost(number(spell["level"], 0), profile), ScrollID: text(change["scrollId"]), Origin: "recorded"})
		}
		input.Play.HP = integer(next["hp"], input.Play.HP)
		input.Play.TemporaryHP = integer(next["tempHp"], input.Play.TemporaryHP)
		decodeInto(next["currency"], &input.Play.Currency)
		for _, resource := range objects(current.Sheet["resources"]) {
			if input.Play.ResourceUses == nil {
				input.Play.ResourceUses = map[string]int{}
			}
			key := text(resource["key"])
			input.Play.ResourceUses[key] = integer(resource["max"], 0) - integer(object(next["resourceUses"])[key], integer(resource["max"], 0))
		}
		decodeInto(next["activeFeatures"], &input.Play.ActiveFeatures)
		decodeInto(next["preparedSpells"], &input.Play.PreparedSpells)
		decodeInto(next["cantrips"], &input.Build.Spells.Cantrips)
		decodeInto(next["spellbook"], &input.Build.Spells.Spellbook)
		decodeInto(next["grantChoices"], &input.Build.Spells.GrantChoices)
		decodeInto(next["grantCastingAbilities"], &input.Build.Spells.CastingAbilities)
		previousSwaps := len(input.Build.Spells.Swaps)
		decodeInto(next["spellSwaps"], &input.Build.Spells.Swaps)
		for index := previousSwaps; index < len(input.Build.Spells.Swaps); index++ {
			input.Build.Spells.Swaps[index].Origin = "recorded"
		}
		items := map[string]Object{}
		for _, item := range objects(next["inventory"]) {
			items[text(item["id"])] = item
		}
		for index := range input.Play.Inventory {
			item := &input.Play.Inventory[index]
			if next, ok := items[item.ID]; ok {
				item.Quantity = integer(next["qty"], item.Quantity)
			} else {
				item.Quantity = 0
			}
		}
	}
	return EvaluateCharacter(input, records, profile), nil
}

// The public character model records spent uses. The existing arithmetic
// helpers consume remaining uses, so the conversion is confined to this boundary.
func characterRemainingResources(input character.Inputs, sheet Object) Object {
	remaining := Object{}
	for _, resource := range objects(sheet["resources"]) {
		key := text(resource["key"])
		remaining[key] = integer(resource["max"], 0) - input.Play.ResourceUses[key]
	}
	return remaining
}

func cloneCharacter(input character.Inputs) character.Inputs {
	var result character.Inputs
	decodeInto(input, &result)
	return result
}
func toObject(value any) Object {
	var result Object
	decodeInto(value, &result)
	if result == nil {
		return Object{}
	}
	return result
}
func toArray(value any) []any {
	var result []any
	decodeInto(value, &result)
	if result == nil {
		return []any{}
	}
	return result
}
func decodeInto[T any](value any, target *T) {
	// Decode detached replacements. Unmarshalling into an existing map merges
	// keys and would resurrect ended activations or removed spell selections.
	var replacement T
	body, err := json.Marshal(value)
	if err == nil && json.Unmarshal(body, &replacement) == nil {
		*target = replacement
	}
}
func addCharacterIssue(result *character.Result, id, target, message, severity string, reference *character.Reference) {
	for _, issue := range result.Issues {
		if issue.ID == id {
			return
		}
	}
	result.Issues = append(result.Issues, character.Issue{ID: id, Target: target, Message: message, Severity: severity, Reference: reference})
}
func characterChoiceKey(choice character.Choice) string {
	return choice.ID + "#" + strconv.Itoa(choice.Slot)
}
