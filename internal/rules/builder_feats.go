package rules

import (
	"sort"
	"strings"

	"github.com/pjunak/addon-dnd-engine/character"
)

// An acquisition is a granting decision, not a feat record or its array index.
// Removing an earlier grant must never rename a later grant's saved choices.
type featAcquisition struct {
	id, featID, name, classID string
	level                     int
	ancestors                 []string
	presets                   Object
}

func featChoiceOwner(acquired featAcquisition, record Object) string {
	owner := "feat:" + acquired.featID
	if truth(record["repeatable"]) || object(record["repeatable"]) != nil {
		owner = character.AcquisitionOwner(owner, acquired.id)
	}
	return owner
}

func selectedFeatAcquisitions(source Object, records Records, classChoices []character.PlanChoice) []featAcquisition {
	result := []featAcquisition{}
	add := func(id, featID, name, classID string, level int, ancestors []string) {
		if featID == "" || contains(ancestors, featID) || recordView(records, "feat", featID) == nil {
			return
		}
		result = append(result, featAcquisition{id: id, featID: featID, name: name, classID: classID, level: level,
			ancestors: append(append([]string{}, ancestors...), featID)})
	}
	for _, origin := range selectedOrigins(source, records) {
		record := origin.record
		id := string(origin.kind) + ":" + text(record["id"])
		add(id, text(record["originFeat"]), firstText(record["name"], record["id"]), "", 1, nil)
		if len(result) > 0 && result[len(result)-1].id == id {
			result[len(result)-1].presets = object(record["originFeatChoices"])
		}
	}
	for _, grant := range objects(source["extraFeats"]) {
		add("grant:"+text(grant["id"]), text(grant["featId"]), firstText(grant["name"], grant["id"]), "", integer(grant["level"], 1), nil)
	}
	appendSelections := func(choices []character.PlanChoice, ancestors []string) {
		for _, descriptor := range choices {
			choice := descriptor
			if descriptor.Kind == character.ChoiceASIMode {
				if text(object(source["featureChoices"])[descriptor.ID]) != "feat" {
					continue
				}
				choice = descriptor.Feat.Choice()
			} else if descriptor.Kind != character.ChoiceFeat {
				continue
			}
			ref := character.Source{}
			if descriptor.Source != nil {
				ref = *descriptor.Source
			}
			name := firstText(recordView(records, string(ref.Type), ref.ID)["name"], ref.ID)
			classID := descriptor.ClassID
			if classID != "" {
				name = firstText(recordView(records, "class", classID)["name"], classID)
			}
			level := ref.Level
			if level == 0 {
				level = 1
			}
			count := max(1, choice.Count)
			for slot := 0; slot < count; slot++ {
				id := choiceIDForSlot(choice.ID, slot, count)
				value := text(object(source["featureChoices"])[id])
				if slot == 0 && value == "" {
					value = choice.Default
				}
				add(id, value, name, classID, level, ancestors)
			}
		}
	}
	appendSelections(append(collectOriginChoices(source, records), classChoices...), nil)
	// Resolve declared descendants before considering the normalized feat
	// summary, or a nested acquisition would be mistaken for a direct grant.
	processed := 0
	followDeclared := func() {
		for processed < len(result) {
			acquired := result[processed]
			processed++
			record := recordView(records, "feat", acquired.featID)
			appendSelections(featChoices(acquired, record), acquired.ancestors)
		}
	}
	followDeclared()
	// Direct feat inputs belong to the shared arithmetic adapter. Normalized
	// characters also carry this summary; do not mistake it for a new grant.
	for index, raw := range values(source["feats"]) {
		id := text(raw)
		if record := object(raw); record != nil {
			id = firstText(record["featId"], record["id"])
		}
		owned := false
		for _, acquired := range result {
			owned = owned || acquired.featID == id
		}
		if !owned {
			add("provided:"+jsonNumber(index), id, "", "", 1, nil)
		}
	}
	followDeclared()
	sort.Slice(result, func(i, j int) bool { return result[i].id < result[j].id })
	return result
}

func featChoices(acquired featAcquisition, record Object) []character.PlanChoice {
	result := []character.PlanChoice{}
	choices := objects(object(record["grants"])["choices"])
	for _, choice := range choices {
		if text(choice["id"]) == "" {
			continue
		}
		descriptor := character.PlanChoice{
			ID:   featChoiceOwner(acquired, record) + ":" + text(choice["id"]),
			Kind: choiceKind(choice, stringsOf(choice["from"])), Count: max(1, integer(choice["count"], 1)),
			Source: &character.Source{Type: character.SourceFeat, ID: acquired.featID, Level: acquired.level},
		}
		copyChoiceFields(&descriptor, choice)
		if preset := text(acquired.presets[text(choice["id"])]); preset != "" && contains(stringsOf(choice["from"]), preset) {
			descriptor.From = []string{preset}
			descriptor.Default = preset
		}
		if conditional := conditionalFeatChoice(record); conditional != nil && text(conditional["id"]) == text(choice["id"]) {
			descriptor.DistinctAcrossAcquisitions = true
		}
		if strings.Contains(featChoiceOwner(acquired, record), "@") {
			descriptor.Acquisition = &character.Acquisition{ID: acquired.id, Name: acquired.name, ClassID: acquired.classID, Level: acquired.level}
			descriptor.Name = firstText(record["name"], acquired.featID)
			if len(choices) > 1 {
				descriptor.ChoiceName = firstText(choice["prompt"], guidanceLabel(text(choice["id"])))
			}
		}
		result = append(result, descriptor)
	}
	return result
}
