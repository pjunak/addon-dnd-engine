package rules

import (
	"encoding/json"
	"strings"

	"github.com/pjunak/addon-dnd-engine/character"
)

// Completion checks must agree with validation, including prerequisites evaluated
// at acquisition time. Repair points to a rendered decision without changing it.
func characterBuilderRepairs(input character.Inputs, result *character.Result) {
	sections := result.Guidance.Sections
	mark := func(group int, id, tab string, label character.Text) {
		if group >= len(sections) {
			return
		}
		section := &sections[group]
		for index := range section.Issues {
			if section.Issues[index].ID == id {
				section.Issues[index].Repair = true
				return
			}
		}
		section.Issues = append(section.Issues, character.GuidanceIssue{ID: id, Text: label, Tab: tab, Repair: true})
		section.Complete = max(0, section.Complete-1)
	}
	for _, issue := range result.Guidance.SaveIssues {
		if issue.Severity != character.SeverityBlocker || strings.HasPrefix(issue.ID, "choice:") {
			continue
		}
		if issue.Target == "abilities" {
			mark(0, "abilities", "character", guidanceText("Finish base ability scores"))
		}
		for groupIndex, group := range [][]character.PlanChoice{result.Plan.CreationChoices, result.Plan.CreationAbilityChoices, result.Plan.ClassChoices} {
			for _, descriptor := range group {
				id := descriptor.ID
				owns := func(key string) bool { return key == id || strings.HasPrefix(key, id+":") }
				matches := owns(issue.Target)
				if strings.HasPrefix(issue.ID, "feat:") {
					for _, choice := range input.Build.Choices {
						var selected string
						if owns(choice.ID) && json.Unmarshal(choice.Value, &selected) == nil && issue.ID == "feat:"+selected {
							matches = true
						}
					}
				}
				entry, exists := result.Guidance.Choices[id]
				if !matches || !exists {
					continue
				}
				entry.Done = false
				result.Guidance.Choices[id] = entry
				section, tab := 0, "character"
				if groupIndex == 2 {
					section, tab = 1, descriptor.ClassID
				}
				mark(section, id, tab, guidanceText(entry.LabelKey, labelArgs(entry.LabelArgs)...))
			}
		}
	}
	total, complete := 0, 0
	for _, section := range sections {
		total += section.Total
		complete += section.Complete
	}
	result.Guidance.Total, result.Guidance.Complete, result.Guidance.Ready = total, complete, total == complete
}
