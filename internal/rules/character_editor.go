package rules

import (
	"encoding/json"
	"github.com/pjunak/addon-dnd-engine/character"
	"strings"
)

// Completion is distinct from legality: an unfinished, bounded build can be
// persisted while remaining unavailable for rules-dependent play commands.
func characterEditorGuidance(input character.Inputs, records Records, profile Ruleset, result *character.Result) {
	canSave := true
	for ability := range input.Build.BaseScores {
		if !contains(Abilities[:], ability) {
			canSave = false
		}
	}
	for kind, id := range map[string]string{"species": input.Build.Species, "background": input.Build.Background} {
		if id != "" && recordByID(records, kind, id) == nil {
			canSave = false
		}
	}
	for _, issue := range result.Issues {
		if issue.Severity == "blocker" && !incompleteCharacterChoice(issue, input, profile, result) {
			canSave = false
		}
	}
	result.Guidance["canSave"] = canSave
	result.Guidance["equipment"] = characterEquipmentOptions(input, records, profile, result)
	options := []any{}
	if len(input.Build.Levels) < profile.Constants.Character.MaximumLevel {
		for _, class := range recordList(records, "class") {
			candidate := cloneCharacter(input)
			candidate.Build.Levels = append(candidate.Build.Levels, character.Level{ID: "editor-next-level", ClassID: text(class["id"])})
			check := character.Result{Issues: []character.Issue{}}
			validateCharacterProgression(candidate, records, profile, &check)
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
	// Check prerequisites at the level granting the choice, including existing
	// classes' prerequisites when introducing a new class.
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
				validateCharacterProgression(candidate, records, profile, &check)
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

// Equipment controls consume these facts instead of interpreting source rules.
func characterEquipmentOptions(input character.Inputs, records Records, profile Ruleset, result *character.Result) Object {
	options := Object{}
	attunement := object(result.Sheet["attunement"])
	for _, item := range input.Play.Inventory {
		entry := Object{"canEquip": false, "canAttune": false, "slot": "worn"}
		options[item.ID] = entry
		if item.Quantity < 1 {
			continue
		}
		if item.Reference == nil {
			for _, grant := range activeCharacterGrants(input) {
				if grant.ID == item.GrantID && grant.ItemID == item.ID && len(grant.Effects) > 0 {
					entry["canEquip"] = true
				}
			}
			continue
		}
		record := recordByID(records, item.Reference.Kind, item.Reference.ID)
		if record == nil {
			continue
		}
		if item.Reference.Kind == "armor" {
			entry["slot"] = "armor"
			if text(record["armorType"]) == "shield" {
				entry["slot"] = "shield"
			}
		}
		candidate := item
		candidate.Location = "equipped"
		check := character.Result{Issues: []character.Issue{}}
		validateCharacterItemMechanics(candidate, record, input, &check)
		if len(check.Issues) > 0 {
			continue
		}
		entry["canEquip"] = true
		if !truth(record["attunement"]) {
			continue
		}
		if !item.Attuned && integer(attunement["count"], 0) >= integer(attunement["limit"], 0) {
			continue
		}
		duplicate := false
		if profile.Constants.Character.UniqueAttunement {
			for _, other := range input.Play.Inventory {
				if other.ID != item.ID && other.Attuned && other.Reference != nil && *other.Reference == *item.Reference {
					duplicate = true
				}
			}
		}
		if duplicate {
			continue
		}
		validatePrerequisite(record["attunementPrerequisites"], "attunement:"+item.ID, "inventory", input, Object(result.Sheet), &check, item.Reference)
		allowed := true
		for _, issue := range check.Issues {
			if issue.Severity == "blocker" {
				allowed = false
			}
		}
		entry["canAttune"] = allowed
	}
	return options
}
