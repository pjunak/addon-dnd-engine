package rules

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pjunak/addon-dnd-engine/character"
)

// Human-readable fallback labels remain usable by older consumers. Templates
// translate engine-owned UI wording without translating authored record names.
func builderGuidanceText(template string, args ...any) Object {
	replacements := []string{}
	for index, arg := range args {
		replacements = append(replacements, fmt.Sprintf("{%d}", index), fmt.Sprint(arg))
	}
	return Object{"label": strings.NewReplacer(replacements...).Replace(template), "labelKey": template, "labelArgs": append([]any{}, args...)}
}

// Completion checks must agree with validation, including prerequisites evaluated
// at acquisition time. Repair points to a rendered decision without changing it.
func characterBuilderRepairs(input character.Inputs, result *character.Result) {
	sections := objects(result.Guidance["sections"])
	mark := func(group int, id, tab string, label Object) {
		if group >= len(sections) {
			return
		}
		section := sections[group]
		for _, issue := range objects(section["issues"]) {
			if text(issue["id"]) == id {
				issue["repair"] = true
				return
			}
		}
		issue := cloneObjectDeep(label)
		issue["id"], issue["tab"], issue["repair"] = id, tab, true
		section["issues"] = append(values(section["issues"]), issue)
		section["complete"] = max(0, integer(section["complete"], 0)-1)
	}
	saveIssues, _ := result.Guidance["saveIssues"].([]character.Issue)
	for _, issue := range saveIssues {
		if issue.Severity != "blocker" || strings.HasPrefix(issue.ID, "choice:") {
			continue
		}
		if issue.Target == "abilities" {
			mark(0, "abilities", "character", builderGuidanceText("Finish base ability scores"))
		}
		for groupIndex, group := range []string{"creationChoices", "creationAbilityChoices", "classChoices"} {
			for _, descriptor := range objects(result.Plan[group]) {
				id := text(descriptor["id"])
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
				if !matches {
					continue
				}
				entry := object(object(result.Guidance["choices"])[id])
				entry["done"] = false
				section, tab := 0, "character"
				if groupIndex == 2 {
					section, tab = 1, text(descriptor["classId"])
				}
				mark(section, id, tab, builderGuidanceText(text(entry["labelKey"]), values(entry["labelArgs"])...))
			}
		}
	}
	total, complete := 0, 0
	for _, section := range sections {
		total += integer(section["total"], 0)
		complete += integer(section["complete"], 0)
	}
	result.Guidance["total"], result.Guidance["complete"], result.Guidance["ready"] = total, complete, total == complete
}
