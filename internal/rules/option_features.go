package rules

import "github.com/pjunak/addon-dnd-engine/character"

// Option features are feature records with a category, such as Eldritch
// Invocations, maneuvers and Metamagic options. A class, subclass or feature
// offers them through a choice with fromCategory. Only the chosen options are
// acquired: they join the character's features, contribute their grants and
// offer their own choices. Unchosen options are reference text.

type chosenOption struct {
	feature Object
	// count is how many slots chose this option; only repeatable options can
	// fill more than one.
	count int
}

// optionChoiceSource is one record whose grants may offer option choices,
// with the source identity and fallback level its choices are read at.
type optionChoiceSource struct {
	grants Object
	source character.Source
	level  int
}

func optionChoiceSources(classID string, classLevel int, subclassID string, records Records) []optionChoiceSource {
	result := []optionChoiceSource{}
	class := recordView(records, "class", classID)
	if class == nil {
		return result
	}
	result = append(result, optionChoiceSource{object(class["grants"]), character.Source{Type: character.SourceClass, ID: classID}, 1})
	if subclass := recordView(records, "subclass", subclassID); subclass != nil && subclassID != "" {
		result = append(result, optionChoiceSource{object(subclass["grants"]), character.Source{Type: character.SourceSubclass, ID: subclassID},
			integer(subclass["subclassLevel"], classSubclassLevel(class))})
	}
	for _, feature := range recordCatalog(records, "feature") {
		if text(feature["classId"]) != classID || text(feature["category"]) != "" || integer(feature["level"], 0) > classLevel ||
			text(feature["subclassId"]) != "" && text(feature["subclassId"]) != subclassID {
			continue
		}
		result = append(result, optionChoiceSource{object(feature["grants"]), character.Source{Type: character.SourceFeature, ID: text(feature["id"])}, max(1, integer(feature["level"], 1))})
	}
	return result
}

// choiceCountAt is a choice's slot count at a class level: its count, raised
// by the highest countByLevel step the class has reached.
func choiceCountAt(choice Object, classLevel int) int {
	count := max(1, integer(choice["count"], 1))
	selectedLevel := -1
	for level, amount := range object(choice["countByLevel"]) {
		parsed := integer(level, 0)
		if parsed <= classLevel && parsed > selectedLevel {
			selectedLevel = parsed
			count = max(1, integer(amount, count))
		}
	}
	return count
}

// chosenOptions lists the option features a class has selected at its current
// level, in choice and slot order.
func chosenOptions(classID string, classLevel int, subclassID string, selections Object, records Records) []chosenOption {
	result := []chosenOption{}
	index := map[string]int{}
	for _, candidate := range optionChoiceSources(classID, classLevel, subclassID, records) {
		grants := selectedChoiceGrants(candidate.grants, candidate.source, selections)
		for _, choice := range objects(grants["choices"]) {
			category := text(choice["fromCategory"])
			if category == "" || text(choice["id"]) == "" || sourceLevelOf(choice, candidate.level) > classLevel {
				continue
			}
			count := choiceCountAt(choice, classLevel)
			for slot := 0; slot < count; slot++ {
				id := text(selections[choiceIDForSlot(text(choice["id"]), slot, count)])
				feature := recordView(records, "feature", id)
				if feature == nil || text(feature["category"]) != category {
					continue
				}
				if at, seen := index[id]; seen {
					result[at].count++
					continue
				}
				index[id] = len(result)
				result = append(result, chosenOption{feature: feature, count: 1})
			}
		}
	}
	return result
}

// optionPrerequisiteMet reads an option's prerequisite: level is a level in
// the option's own class and feature names an acquired feature. Descriptive
// fields such as text and cost do not restrict the choice.
func optionPrerequisiteMet(option Object, sheet *character.Calculation) bool {
	requirement := object(option["prerequisite"])
	predicate := Object{}
	if level := integer(requirement["level"], 0); level > 1 && text(option["classId"]) != "" {
		predicate["classes"] = Object{text(option["classId"]): level}
	}
	if feature := text(requirement["feature"]); feature != "" {
		predicate["feature"] = feature
	}
	if len(predicate) == 0 {
		return true
	}
	matched, known := prerequisiteMatches(predicate, sheet)
	return matched && known
}
