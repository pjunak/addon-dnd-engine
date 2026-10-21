package rules

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/pjunak/addon-dnd-engine/character"
)

// Completion is distinct from legality: an unfinished, bounded build can be
// persisted while remaining unavailable for rules-dependent play commands.
func characterEditorGuidance(input character.Inputs, decisions Object, records Records, profile Ruleset, result *character.Result) {
	saveIssues := []character.Issue{}
	unknown := []string{}
	for ability := range input.Build.BaseScores {
		if !contains(Abilities[:], ability) {
			unknown = append(unknown, ability)
		}
	}
	sort.Strings(unknown)
	for _, ability := range unknown {
		saveIssues = append(saveIssues, character.Issue{ID: "unknown-ability:" + ability, Target: "abilities", Message: "Choose a supported base ability.", Severity: character.SeverityBlocker})
	}
	for _, origin := range []struct{ kind, id string }{{"species", input.Build.Species}, {"background", input.Build.Background}} {
		if origin.id != "" && recordByID(records, origin.kind, origin.id) == nil {
			saveIssues = append(saveIssues, character.Issue{ID: "unavailable-" + origin.kind, Target: origin.kind, Message: "Choose an available " + origin.kind + ".", Severity: character.SeverityBlocker})
		}
	}
	for _, issue := range result.Issues {
		if issue.Severity == character.SeverityBlocker && !incompleteCharacterChoice(issue, input, profile, result) {
			saveIssues = append(saveIssues, issue)
		}
	}
	result.Guidance.CanSave = len(saveIssues) == 0
	result.Guidance.SaveIssues = saveIssues
	result.Guidance.Equipment = characterEquipmentOptions(input, records, profile, result)
	slots := map[string]character.EquipmentFacts{}
	for id, entry := range result.Guidance.Equipment {
		slots[id] = character.EquipmentFacts{Slot: entry.Slot, BodyPlacements: entry.BodyPlacements}
	}
	result.Sheet.Equipment = slots
	options := []character.Option{}
	if len(input.Build.Levels) < profile.Constants.Character.MaximumLevel {
		for _, class := range recordCatalog(records, "class") {
			candidate := cloneCharacter(input)
			candidate.Build.Levels = append(candidate.Build.Levels, character.Level{ID: "editor-next-level", ClassID: text(class["id"])})
			check := character.Result{Issues: []character.Issue{}}
			validateSelectedProgression(candidate, records, profile, &check, progressionChecks{classes: true})
			allowed := true
			for _, issue := range check.Issues {
				if strings.HasPrefix(issue.ID, "multiclass:") && issue.Severity == character.SeverityBlocker {
					allowed = false
				}
			}
			if allowed {
				options = append(options, guidanceOption(text(class["id"]), class))
			}
		}
	}
	sortGuidanceOptions(options)
	result.Guidance.ClassOptions = options
	for index := range result.Guidance.Classes {
		class := &result.Guidance.Classes[index]
		class.HitDieMax = hitDieSize(text(recordByID(records, "class", class.ClassID)["hitDie"]))
	}
	// Check prerequisites at acquisition, including ordered multiclass levels.
	simpleLevels := len(input.Build.Levels) > 0 && len(input.Build.Levels) <= profile.Constants.Character.MaximumLevel
	for _, selected := range objects(decisions["classes"]) {
		simpleLevels = simpleLevels && recordByID(records, "class", text(selected["classId"])) != nil
	}
	acquisitions := selectedFeatAcquisitions(decisions, records, result.Plan.ClassChoices)
	for _, group := range [][]character.PlanChoice{result.Plan.CreationChoices, result.Plan.ClassChoices} {
		for _, descriptor := range group {
			entry, exists := result.Guidance.Choices[descriptor.ID]
			if !exists {
				continue
			}
			choice := descriptor
			listed := &entry.Options
			featOptions := false
			if descriptor.Kind == character.ChoiceASIMode {
				choice, listed, featOptions = descriptor.Feat.Choice(), &entry.FeatOptions, true
			} else if descriptor.Kind != character.ChoiceFeat {
				continue
			}
			filtered := []character.Option{}
			for _, option := range *listed {
				var levels map[int]bool
				if simpleLevels {
					feat := recordByID(records, "feat", option.ID)
					ends, repeatable, known := featOptionLevels(input, descriptor, choice, feat, acquisitions)
					if known && !repeatable {
						continue
					}
					// Most catalog prerequisites need only acquisition level or a
					// DM waiver; others hydrate just the levels that decide them.
					if known && !prerequisiteNeedsSheet(object(feat["prerequisites"]), 0) {
						if levelPrerequisitesMet(input, feat, ends) {
							filtered = append(filtered, option)
						}
						continue
					}
					if known {
						levels = acquisitionLevels(ends)
					}
				}
				candidate := cloneCharacter(input)
				choices := []character.Choice{}
				for _, existing := range candidate.Build.Choices {
					if existing.ID != choice.ID && existing.ID != descriptor.ID {
						choices = append(choices, existing)
					}
				}
				if featOptions {
					choices = append(choices, character.Choice{ID: descriptor.ID, Value: json.RawMessage(`"feat"`)})
				}
				value, _ := json.Marshal(option.ID)
				candidate.Build.Choices = append(choices, character.Choice{ID: choice.ID, Value: value})
				check := character.Result{Issues: []character.Issue{}}
				validateSelectedProgression(candidate, records, profile, &check, progressionChecks{feats: true, featID: option.ID, levels: levels})
				allowed := true
				for _, issue := range check.Issues {
					if issue.ID == "feat:"+option.ID && issue.Severity == character.SeverityBlocker {
						allowed = false
					}
				}
				if allowed {
					filtered = append(filtered, option)
				}
			}
			*listed = filtered
			result.Guidance.Choices[descriptor.ID] = entry
		}
	}
	characterBuilderRepairs(input, result)
}

func incompleteCharacterChoice(issue character.Issue, input character.Inputs, profile Ruleset, result *character.Result) bool {
	id := issue.ID
	if strings.HasPrefix(id, "choice:") || id == "base-scores" || id == "roll-count" || id == "levels" && len(input.Build.Levels) == 0 {
		return true
	}
	if strings.HasPrefix(id, "ability-bound:") {
		_, exists := input.Build.BaseScores[strings.TrimPrefix(id, "ability-bound:")]
		return !exists
	}
	if strings.HasPrefix(id, "base-score:") {
		_, exists := input.Build.BaseScores[strings.TrimPrefix(id, "base-score:")]
		return !exists
	}
	if id == "point-buy" {
		spent := 0
		for _, value := range input.Build.BaseScores {
			cost, ok := profile.Constants.PointBuy.Cost[jsonNumber(value)]
			if !ok {
				return false
			}
			spent += cost
		}
		return spent <= profile.Constants.PointBuy.Budget
	}
	if id == "standard-array" {
		available := map[int]int{}
		for _, value := range profile.Constants.Character.StandardArray {
			available[value]++
		}
		for _, value := range input.Build.BaseScores {
			available[value]--
			if available[value] < 0 {
				return false
			}
		}
		return true
	}
	if strings.HasPrefix(id, "ability-assignment:") {
		for _, choice := range input.Build.Choices {
			if id != "ability-assignment:"+characterChoiceKey(choice) {
				continue
			}
			var assignment map[string]int
			if json.Unmarshal(choice.Value, &assignment) != nil {
				return false
			}
			descriptor := findAbilityDescriptor(result.Plan, choice.ID)
			if descriptor == nil {
				return false
			}
			total := 0
			for ability, value := range assignment {
				if value < 0 || value > descriptor.PerAbilityMax || !contains(Abilities[:], ability) || descriptor.Eligible != nil && !contains(descriptor.Eligible, ability) {
					return false
				}
				total += value
			}
			return total <= descriptor.Budget
		}
	}
	if strings.HasPrefix(id, "cantrip-count:") {
		classID := strings.TrimPrefix(id, "cantrip-count:")
		if sheet := result.Sheet.Calculation; sheet != nil {
			if caster := playCaster(sheet, classID); caster != nil {
				return len(input.Build.Spells.Cantrips[classID]) <= caster.CantripsKnown
			}
		}
	}
	return strings.HasPrefix(id, "spellbook-count:")
}
func findAbilityDescriptor(plan character.Plan, id string) *character.PlanChoice {
	for _, group := range [][]character.PlanChoice{plan.CreationAbilityChoices, plan.ClassChoices, plan.CreationChoices} {
		for index := range group {
			choice := &group[index]
			candidates := []*character.PlanChoice{choice, choice.Ability}
			if choice.Feat != nil {
				candidates = append(candidates, choice.Feat.Ability)
			}
			for _, candidate := range candidates {
				if candidate != nil && candidate.ID == id {
					return candidate
				}
			}
		}
	}
	return nil
}
