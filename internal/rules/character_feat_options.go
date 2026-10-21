package rules

import (
	"strings"

	"github.com/pjunak/addon-dnd-engine/character"
)

// featOptionLevels finds the character levels (prefix lengths) at which a
// feat option, and every other acquisition of the same feat, is first
// acquired. known is false when a multi-slot choice, replacement history or a
// feat granted by another feat makes that mapping uncertain; repeatable is
// false when the feat cannot be acquired that many times.
func featOptionLevels(input character.Inputs, descriptor, choice character.PlanChoice, feat Object, acquisitions []featAcquisition) (ends []int, repeatable, known bool) {
	if choice.Count > 1 {
		return nil, true, false
	}
	// A replacement can move this acquisition, or another copy of the same
	// feat, to a later level. The progression validator reconstructs that history.
	for _, entry := range input.Build.Replacements {
		if entry.Kind == character.ReplaceFeat && (entry.Key == choice.ID || entry.In == text(feat["id"]) || entry.Out == text(feat["id"])) {
			return nil, true, false
		}
	}
	source := character.Source{}
	if descriptor.Source != nil {
		source = *descriptor.Source
	}
	classID := descriptor.ClassID
	if classID == "" && source.Type != character.SourceBackground && source.Type != character.SourceSpecies {
		return nil, true, false
	}
	level := source.Level
	if level == 0 {
		level = 1
	}
	at := featAcquisitionIndex(input, classID, level)
	if at < 0 {
		return nil, true, false
	}
	// Replacing a feat can also remove the feats it granted. Let the full
	// candidate plan resolve those descendants before checking duplicates.
	for _, selected := range acquisitions {
		if selected.id != choice.ID {
			continue
		}
		for _, acquired := range acquisitions {
			if len(acquired.ancestors) > 1 && contains(acquired.ancestors[:len(acquired.ancestors)-1], selected.featID) {
				return nil, true, false
			}
		}
	}
	ends = []int{at}
	count := 1
	for _, acquired := range acquisitions {
		if acquired.id == choice.ID || acquired.featID != text(feat["id"]) {
			continue
		}
		count++
		if !featRepetitionAllowed(feat, count) {
			return nil, false, true
		}
		if acquired.classID == "" && !strings.HasPrefix(acquired.id, "background:") && !strings.HasPrefix(acquired.id, "species:") && !strings.HasPrefix(acquired.id, "grant:") {
			return nil, true, false
		}
		prior := featAcquisitionIndex(input, acquired.classID, acquired.level)
		if prior < 0 {
			return nil, true, false
		}
		ends = append(ends, prior)
	}
	return ends, true, true
}

// levelPrerequisitesMet checks prerequisites that need no sheet (level or a
// DM waiver) at each acquisition level.
func levelPrerequisitesMet(input character.Inputs, feat Object, ends []int) bool {
	for _, end := range ends {
		prefix := input
		prefix.Build.Levels = prefix.Build.Levels[:end]
		check := character.Result{}
		validatePrerequisite(feat["prerequisites"], "feat:"+text(feat["id"]), "choices", prefix, &character.Calculation{TotalLevel: end}, &check, nil)
		if characterHasBlocker(check) {
			return false
		}
	}
	return true
}

// acquisitionLevels lists the progression level indexes that decide whether
// a feat acquired at these prefix lengths is allowed: each acquisition level
// and the level before it, which supplies what was already acquired.
func acquisitionLevels(ends []int) map[int]bool {
	levels := map[int]bool{}
	for _, end := range ends {
		levels[end-1] = true
		levels[end-2] = true
	}
	return levels
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
		if key != "level" && key != "all" && key != "any" && key != "text" {
			// Every predicate but the character level reads the sheet.
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
