package rules

import (
	"encoding/json"
	"sort"
	"strconv"

	"github.com/pjunak/addon-dnd-engine/character"
)

// EvaluateCharacter evaluates detached decisions. Completion, eligibility and
// numerical constraints all belong here, never in the persistence or UI layer.
func EvaluateCharacter(input character.Inputs, records Records, profile Ruleset) (result character.Result) {
	snapshot := newCharacterRecordSnapshot(records)
	if verifyRecords != nil {
		defer verifyRecords(snapshot)
	}
	records = snapshot
	input = cloneCharacter(input)
	result = character.Result{ContractVersion: character.ContractVersion, Inputs: input, Sheet: character.Sheet{Status: character.SheetNeedsChoices}, Explanations: map[string]character.Explanation{}, Evidence: []character.Evidence{}, Issues: []character.Issue{}}
	defer finishResult(&result)
	if profile.Constants.Character == nil {
		addCharacterIssue(&result, "character-policy", "rules", "The rules profile does not define character creation policy.", character.SeverityBlocker, nil)
		return result
	}
	needsChoices := len(input.Build.BaseScores) != 6 || len(input.Build.Levels) == 0 || input.Build.Species == "" || input.Build.Background == ""
	tracked := newCharacterRecords(records)
	decisions := characterDecisions(input, tracked, profile, &result)
	normalized := NormalizeBuilderDecisions(decisions, tracked, profile)
	applyCharacterAbilityEffects(input, normalized, &result, profile, tracked)
	tracked.selected = map[string]character.Evidence{}
	hydrated := Hydrate(normalized, tracked, profile)
	sheet := hydrated.Sheet
	result.Sheet = character.Sheet{Status: character.SheetCalculated, Calculation: sheet}
	sheet.Derived.Size = selectedSpeciesSize(normalized, recordByID(tracked, "species", text(normalized["species"])))
	annotateSpellbookCapacity(input, sheet)
	sheet.Feats = acquiredFeatRows(selectedFeats(normalized, tracked), objects(normalized["featAcquisitions"]))
	for _, ability := range Abilities {
		row := sheet.Abilities[ability]
		row.Cap = integer(object(normalized["scoreCaps"])[ability], 0)
		sheet.Abilities[ability] = row
	}
	baseAttunementWarning := attunementCapacityWarning(sheet.Attunement.Limit)
	applyCharacterHitDice(input, sheet, tracked, &result, profile)
	applyCharacterEffects(input, sheet, &result, tracked)
	// A lowered maximum lowers current HP with it; a raised one never heals.
	if !needsChoices && sheet.Derived.MaxHP >= 0 && input.Play.HP > sheet.Derived.MaxHP {
		input.Play.HP = sheet.Derived.MaxHP
		result.Inputs.Play.HP = input.Play.HP
	}
	normalized["resourceUses"] = characterRemainingResources(input, sheet)
	// Catalog discovery and legality checks must not retain every unselected
	// option as character evidence. The calculation reads above are retained.
	untracked := records
	result.Plan = BuilderPlan(decisions, untracked, profile)
	choiceOptions := characterExpertiseOptions(input, result.Plan, untracked, profile)
	for id, options := range characterProficiencyOptions(input, result.Plan, untracked, profile) {
		choiceOptions[id] = options
	}
	for id, options := range characterConditionalFeatOptions(input, result.Plan, untracked, profile) {
		choiceOptions[id] = options
	}
	result.Guidance = builderGuidance(decisions, result.Plan, untracked, profile, choiceOptions)
	result.SpellOptions = SpellOptions(normalized, sheet, untracked, profile)
	for index := range result.SpellOptions.Classes {
		caster := &result.SpellOptions.Classes[index]
		remaining := characterSpellReplacements(input, untracked, caster.ClassID)
		caster.CanSwap = remaining > 0
		caster.ReplacementsRemaining = remaining
	}
	validateCharacter(input, decisions, untracked, profile, &result)
	validateCharacterSpellProgression(input, untracked, profile, &result)
	validateClassReplacementLedger(input, profile, &result)
	for _, section := range result.Guidance.Sections {
		for _, issue := range section.Issues {
			severity := issue.Severity
			if severity == "" {
				severity = character.SeverityBlocker
			}
			addCharacterIssue(&result, "choice:"+issue.ID, issue.ID, issue.Label, severity, nil)
		}
	}
	for _, warning := range hydrated.Warnings {
		// Character validation checks final capacity after authorized item/DM effects.
		// The lower-level hydration warning only knows the unmodified class limit.
		if warning == baseAttunementWarning {
			continue
		}
		addCharacterIssue(&result, "rules-warning:"+warning, "build", warning, character.SeverityWarning, nil)
	}
	captureCharacterSources(sheet, tracked)
	// Retain learned spell facts for offline reading without attaching every
	// spell to unrelated statistics. Explanations use the calculation reads.
	calculationEvidence := tracked.evidence()
	for _, selections := range []map[string][]string{input.Build.Spells.Cantrips, input.Build.Spells.Spellbook, input.Build.Spells.GrantChoices, input.Play.PreparedSpells} {
		for _, ids := range selections {
			for _, id := range ids {
				recordByID(tracked, "spell", id)
			}
		}
	}
	if selections := characterSelections(decisions, result.Plan, result.Guidance, tracked); len(selections) > 0 {
		sheet.Selections = selections
	}
	result.Evidence = tracked.evidence()
	result.Explanations = characterExplanations(input, sheet, calculationEvidence, profile)
	for key, explanation := range result.Explanations {
		if explanation.Unit != "" {
			explanation.Unit = profile.Constants.Character.DistanceUnit
			result.Explanations[key] = explanation
		}
	}
	appendEffectExplanations(input, &result, records)
	scopeCharacterExplanationSources(input, normalized, sheet, calculationEvidence, result.Explanations)
	validateCharacterHands(input, records, &result)
	result.Ready = !characterHasBlocker(result)
	if needsChoices {
		result.Sheet = character.Sheet{Status: character.SheetNeedsChoices}
		result.Explanations = map[string]character.Explanation{"status": {Label: "Calculated values need choices", Formula: "Choose base abilities, species, background and the first class level to calculate the character.", Value: nil, Terms: []character.Term{}, Sources: []character.Reference{}}}
	}
	characterEditorGuidance(input, decisions, records, profile, &result)
	inspiration := input.Play.Inspiration != nil && *input.Play.Inspiration
	result.Sheet.Inspiration = inspiration
	result.Guidance.AuthoredPlay = character.AuthoredPlay{Inspiration: true, QuickUse: true, Storage: true, BodyPlacement: true, Hands: true, Conditions: true}
	characterConditions(input, records, profile, &result)
	characterConcentration(input, tracked, profile, &result)
	if len(input.Play.Conditions) > 0 && profile.Constants.Character.ConditionRules != "" {
		_, _ = tracked.Value("rule", profile.Constants.Character.ConditionRules)
	}
	result.Evidence = tracked.evidence()
	characterHands(input, records, &result)
	characterQuickUse(input, &result)
	result.Sheet.ActiveItems = characterActiveItems(input, records)
	characterStorage(input, &result)
	characterCarrying(input, tracked, profile, &result)
	characterPlacement(input, &result)
	reportRulesData(&result)
	result.Guidance.ClassReplacements = classReplacementOptions(input, records, profile, &result)
	result.Explanations["inspiration"] = character.Explanation{Label: "Inspiration", Formula: "Authored play state. Awarding and spending Inspiration are explicit edits; calculation and rest do not change it.", Value: inspiration, Terms: []character.Term{}, Sources: []character.Reference{}}
	return result
}

// finishResult makes every required list and map the evaluation computed
// non-null, as the published schema requires. Inputs stay as the caller sent
// them; the service fills them before responding.
func finishResult(result *character.Result) {
	character.FillEmpty(&result.Sheet)
	character.FillEmpty(&result.Plan)
	character.FillEmpty(&result.Guidance)
	character.FillEmpty(&result.SpellOptions)
	character.FillEmpty(&result.Explanations)
	character.FillEmpty(&result.Evidence)
	character.FillEmpty(&result.Issues)
	if checkResult != nil {
		checkResult(*result)
	}
}

// checkResult lets tests check every evaluation result.
var checkResult func(character.Result)

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
			decisions["extraFeats"] = append(values(decisions["extraFeats"]), Object{"id": grant.ID, "featId": grant.Feat.ID, "name": grant.Name, "level": grant.EffectiveLevel})
		}
	}
	choices := append([]character.Choice(nil), build.Choices...)
	sort.SliceStable(choices, func(i, j int) bool {
		if choices[i].ID == choices[j].ID {
			return choices[i].Slot < choices[j].Slot
		}
		return choices[i].ID < choices[j].ID
	})
	// Parents may sort after their children (for example a species feat and
	// its selections). Apply each available decision once, in dependency order.
	// Feat, mode and package selections can expose dependent descriptors.
	plan := BuilderPlan(decisions, records, profile)
	for len(choices) > 0 {
		pending := []character.Choice{}
		for _, choice := range choices {
			var value any
			if json.Unmarshal(choice.Value, &value) != nil {
				addCharacterIssue(result, "invalid-choice:"+choice.ID, choice.ID, "Choice value is invalid.", character.SeverityBlocker, nil)
				continue
			}
			descriptor := findCharacterChoice(plan, choice.ID, decisions, records)
			if descriptor == nil {
				pending = append(pending, choice)
				continue
			}
			if assignment := object(value); assignment != nil {
				for _, ability := range Abilities {
					if amount, exists := assignment[ability]; exists {
						decisions = applyOwnedBuilderChoice(decisions, Object{"choiceId": choice.ID, "value": Object{"ability": ability, "amount": amount}}, records, plan)
					}
				}
			} else {
				decisions = applyOwnedBuilderChoice(decisions, Object{"choiceId": choice.ID, "slot": choice.Slot, "value": value}, records, plan)
				if kind := descriptor.Kind; kind == character.ChoiceFeat || kind == character.ChoiceASIMode || kind == character.ChoiceEnumerated {
					plan = BuilderPlan(decisions, records, profile)
				}
			}
		}
		if len(pending) == len(choices) {
			break
		}
		choices = pending
	}
	for _, item := range play.Inventory {
		if item.Quantity < 1 {
			continue
		}
		entry := Object{"id": item.ID, "name": item.Name, "qty": item.Quantity, "location": item.Location, "attuned": item.Attuned, "notes": item.Notes}
		if characterItemSuspended(input, item.ID) {
			entry["location"] = "carried"
		}
		if hands := play.Hands; hands != nil && (item.ID == hands.Main || item.ID == hands.Off) {
			entry["handGrip"] = "one"
			if item.ID == hands.Main {
				entry["handGrip"] = hands.Grip
			}
		}
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
	decisions["itemGrants"] = characterItemGrantSources(input, records)
	return decisions
}

// The public character model records spent uses. The existing arithmetic
// helpers consume remaining uses, so the conversion is confined to this boundary.
func characterRemainingResources(input character.Inputs, sheet *character.Calculation) Object {
	remaining := Object{}
	for _, resource := range sheet.Resources {
		remaining[resource.Key] = resource.Max - input.Play.ResourceUses[resource.Key]
	}
	return remaining
}

func cloneCharacter(input character.Inputs) character.Inputs {
	return deepCopy(input)
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
func addCharacterIssue(result *character.Result, id, target, message string, severity character.IssueSeverity, reference *character.Reference) {
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
