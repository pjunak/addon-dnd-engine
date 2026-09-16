package rules

import (
	"strings"

	"github.com/pjunak/addon-dnd-engine/character"
)

// Most catalog prerequisites need only acquisition level or a DM waiver.
// Keep those on the same predicate interpreter without hydrating every earlier
// level for every option. Ability/feature predicates retain full progression.
func simpleFeatOptionAllowed(input character.Inputs, descriptor, choice, feat Object, acquisitions []featAcquisition) (bool, bool) {
	if integer(choice["count"], 1) > 1 || prerequisiteNeedsSheet(object(feat["prerequisites"]), 0) {
		return false, false
	}
	source := object(descriptor["source"])
	classID := text(descriptor["classId"])
	if classID == "" && text(source["type"]) != "background" && text(source["type"]) != "species" {
		return false, false
	}
	at := featAcquisitionIndex(input, classID, integer(source["level"], 1))
	if at < 0 {
		return false, false
	}
	// Replacing a feat can also remove the feats it granted. Let the full
	// candidate plan resolve those descendants before checking duplicates.
	for _, selected := range acquisitions {
		if selected.id != text(choice["id"]) {
			continue
		}
		for _, acquired := range acquisitions {
			if len(acquired.ancestors) > 1 && contains(acquired.ancestors[:len(acquired.ancestors)-1], selected.featID) {
				return false, false
			}
		}
	}
	prefixes := []int{at}
	for _, acquired := range acquisitions {
		if acquired.id == text(choice["id"]) || acquired.featID != text(feat["id"]) {
			continue
		}
		if !truth(feat["repeatable"]) && object(feat["repeatable"]) == nil {
			return false, true
		}
		if acquired.classID == "" && !strings.HasPrefix(acquired.id, "background:") && !strings.HasPrefix(acquired.id, "species:") && !strings.HasPrefix(acquired.id, "grant:") {
			return false, false
		}
		prior := featAcquisitionIndex(input, acquired.classID, acquired.level)
		if prior < 0 {
			return false, false
		}
		prefixes = append(prefixes, prior)
	}
	for _, end := range prefixes {
		prefix := input
		prefix.Build.Levels = prefix.Build.Levels[:end]
		check := character.Result{}
		validatePrerequisite(feat["prerequisites"], "feat:"+text(feat["id"]), "choices", prefix, Object{"totalLevel": end}, &check, nil)
		for _, issue := range check.Issues {
			if issue.Severity == "blocker" {
				return false, true
			}
		}
	}
	return true, true
}

func featAcquisitionIndex(input character.Inputs, classID string, level int) int {
	if classID == "" {
		if level > len(input.Build.Levels) {
			return -1
		}
		return max(1, level)
	}
	count := 0
	for index, entry := range input.Build.Levels {
		if entry.ClassID == classID {
			count++
			if count == level {
				return index + 1
			}
		}
	}
	return -1
}

func prerequisiteNeedsSheet(value Object, depth int) bool {
	if depth > 12 {
		return false
	}
	for key, raw := range value {
		if key == "abilities" || key == "feature" {
			return true
		}
		if key == "all" || key == "any" {
			for _, child := range objects(raw) {
				if prerequisiteNeedsSheet(child, depth+1) {
					return true
				}
			}
		}
	}
	return false
}
