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
		saveIssues = append(saveIssues, character.Issue{ID: "unknown-ability:" + ability, Target: "abilities", Message: "Choose a supported base ability.", Severity: "blocker"})
	}
	for _, origin := range []struct{ kind, id string }{{"species", input.Build.Species}, {"background", input.Build.Background}} {
		if origin.id != "" && recordByID(records, origin.kind, origin.id) == nil {
			saveIssues = append(saveIssues, character.Issue{ID: "unavailable-" + origin.kind, Target: origin.kind, Message: "Choose an available " + origin.kind + ".", Severity: "blocker"})
		}
	}
	for _, issue := range result.Issues {
		if issue.Severity == "blocker" && !incompleteCharacterChoice(issue, input, profile, result) {
			saveIssues = append(saveIssues, issue)
		}
	}
	result.Guidance["canSave"] = len(saveIssues) == 0
	result.Guidance["saveIssues"] = saveIssues
	result.Guidance["equipment"] = characterEquipmentOptions(input, records, profile, result)
	slots := Object{}
	for id, raw := range object(result.Guidance["equipment"]) {
		slots[id] = Object{"slot": object(raw)["slot"]}
	}
	result.Sheet["equipment"] = slots
	options := []any{}
	if len(input.Build.Levels) < profile.Constants.Character.MaximumLevel {
		for _, class := range recordList(records, "class") {
			candidate := cloneCharacter(input)
			candidate.Build.Levels = append(candidate.Build.Levels, character.Level{ID: "editor-next-level", ClassID: text(class["id"])})
			check := character.Result{Issues: []character.Issue{}}
			validateSelectedProgression(candidate, records, profile, &check, progressionChecks{classes: true})
			allowed := true
			for _, issue := range check.Issues {
				if strings.HasPrefix(issue.ID, "multiclass:") && issue.Severity == "blocker" {
					allowed = false
				}
			}
			if allowed {
				options = append(options, guidanceOption(text(class["id"]), class))
			}
		}
	}
	sortGuidanceOptions(options)
	result.Guidance["classOptions"] = options
	for _, class := range objects(result.Guidance["classes"]) {
		class["hitDieMax"] = hitDieSize(text(recordByID(records, "class", text(class["classId"]))["hitDie"]))
	}
	// Check prerequisites at acquisition, including ordered multiclass levels.
	simpleLevels := len(input.Build.Levels) > 0 && len(input.Build.Levels) <= profile.Constants.Character.MaximumLevel
	for _, selected := range objects(decisions["classes"]) {
		simpleLevels = simpleLevels && recordByID(records, "class", text(selected["classId"])) != nil
	}
	acquisitions := selectedFeatAcquisitions(decisions, records, values(result.Plan["classChoices"]))
	for _, group := range []string{"creationChoices", "classChoices"} {
		for _, descriptor := range objects(result.Plan[group]) {
			entry := object(object(result.Guidance["choices"])[text(descriptor["id"])])
			choice := descriptor
			key := "options"
			if text(descriptor["kind"]) == "asiMode" {
				choice = object(descriptor["feat"])
				key = "featOptions"
			} else if text(descriptor["kind"]) != "feat" {
				continue
			}
			filtered := []any{}
			for _, option := range objects(entry[key]) {
				if simpleLevels {
					feat := recordByID(records, "feat", text(option["id"]))
					if allowed, handled := simpleFeatOptionAllowed(input, descriptor, choice, feat, acquisitions); handled {
						if allowed {
							filtered = append(filtered, option)
						}
						continue
					}
				}
				candidate := cloneCharacter(input)
				choices := []character.Choice{}
				for _, existing := range candidate.Build.Choices {
					if existing.ID != text(choice["id"]) && existing.ID != text(descriptor["id"]) {
						choices = append(choices, existing)
					}
				}
				if key == "featOptions" {
					choices = append(choices, character.Choice{ID: text(descriptor["id"]), Value: json.RawMessage(`"feat"`)})
				}
				value, _ := json.Marshal(text(option["id"]))
				candidate.Build.Choices = append(choices, character.Choice{ID: text(choice["id"]), Value: value})
				check := character.Result{Issues: []character.Issue{}}
				validateSelectedProgression(candidate, records, profile, &check, progressionChecks{feats: true, featID: text(option["id"])})
				allowed := true
				for _, issue := range check.Issues {
					if issue.ID == "feat:"+text(option["id"]) && issue.Severity == "blocker" {
						allowed = false
					}
				}
				if allowed {
					filtered = append(filtered, option)
				}
			}
			entry[key] = filtered
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
			descriptor := findAbilityDescriptor(Object(result.Plan), choice.ID)
			if descriptor == nil {
				return false
			}
			total := 0
			for ability, value := range assignment {
				if value < 0 || value > integer(descriptor["perAbilityMax"], 0) || !contains(Abilities[:], ability) || descriptor["eligible"] != nil && !contains(stringsOf(descriptor["eligible"]), ability) {
					return false
				}
				total += value
			}
			return total <= integer(descriptor["budget"], 0)
		}
	}
	if strings.HasPrefix(id, "cantrip-count:") {
		classID := strings.TrimPrefix(id, "cantrip-count:")
		for _, caster := range objects(object(result.Sheet["spellcasting"])["perClass"]) {
			if text(caster["classId"]) == classID {
				return len(input.Build.Spells.Cantrips[classID]) <= integer(caster["cantripsKnown"], 0)
			}
		}
	}
	return strings.HasPrefix(id, "spellbook-count:")
}
func findAbilityDescriptor(plan Object, id string) Object {
	for _, group := range []string{"creationAbilityChoices", "classChoices", "creationChoices"} {
		for _, choice := range objects(plan[group]) {
			for _, candidate := range []Object{choice, object(choice["ability"]), object(object(choice["feat"])["ability"])} {
				if text(candidate["id"]) == id {
					return candidate
				}
			}
		}
	}
	return nil
}
