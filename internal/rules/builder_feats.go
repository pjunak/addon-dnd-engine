package rules

import (
	"net/url"
	"sort"
	"strings"
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
		owner += "@" + url.QueryEscape(acquired.id)
	}
	return owner
}

func selectedFeatAcquisitions(source Object, records Records, classChoices []any) []featAcquisition {
	result := []featAcquisition{}
	add := func(id, featID, name, classID string, level int, ancestors []string) {
		if featID == "" || contains(ancestors, featID) || recordByID(records, "feat", featID) == nil {
			return
		}
		result = append(result, featAcquisition{id: id, featID: featID, name: name, classID: classID, level: level,
			ancestors: append(append([]string{}, ancestors...), featID)})
	}
	for _, origin := range selectedOrigins(source, records) {
		record := object(origin["record"])
		add(text(origin["type"])+":"+text(record["id"]), text(record["originFeat"]), firstText(record["name"], record["id"]), "", 1, nil)
		if len(result) > 0 && result[len(result)-1].id == text(origin["type"])+":"+text(record["id"]) {
			result[len(result)-1].presets = object(record["originFeatChoices"])
		}
	}
	for _, grant := range objects(source["extraFeats"]) {
		add("grant:"+text(grant["id"]), text(grant["featId"]), firstText(grant["name"], grant["id"]), "", integer(grant["level"], 1), nil)
	}
	appendSelections := func(choices []any, ancestors []string) {
		for _, descriptor := range objects(choices) {
			choice := descriptor
			if text(descriptor["kind"]) == "asiMode" {
				if text(object(source["featureChoices"])[text(descriptor["id"])]) != "feat" {
					continue
				}
				choice = object(descriptor["feat"])
			} else if text(descriptor["kind"]) != "feat" {
				continue
			}
			ref := object(descriptor["source"])
			name := firstText(recordByID(records, text(ref["type"]), text(ref["id"]))["name"], ref["id"])
			classID := text(descriptor["classId"])
			if classID != "" {
				name = firstText(recordByID(records, "class", classID)["name"], classID)
			}
			count := max(1, integer(choice["count"], 1))
			for slot := 0; slot < count; slot++ {
				id := choiceIDForSlot(text(choice["id"]), slot, count)
				value := text(object(source["featureChoices"])[id])
				if slot == 0 && value == "" {
					value = text(choice["default"])
				}
				add(id, value, name, classID, integer(ref["level"], 1), ancestors)
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
			record := recordByID(records, "feat", acquired.featID)
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

func featChoices(acquired featAcquisition, record Object) []any {
	result := []any{}
	choices := objects(object(record["grants"])["choices"])
	for _, choice := range choices {
		if text(choice["id"]) == "" {
			continue
		}
		descriptor := Object{
			"id":   featChoiceOwner(acquired, record) + ":" + text(choice["id"]),
			"kind": choiceKind(choice, choice["from"]), "count": max(1, integer(choice["count"], 1)),
			"source": Object{"type": "feat", "id": acquired.featID, "level": acquired.level},
		}
		copyChoiceFields(descriptor, choice)
		if preset := text(acquired.presets[text(choice["id"])]); preset != "" && contains(stringsOf(choice["from"]), preset) {
			descriptor["from"] = []any{preset}
			descriptor["default"] = preset
		}
		if conditional := conditionalFeatChoice(record); conditional != nil && text(conditional["id"]) == text(choice["id"]) {
			descriptor["distinctAcrossAcquisitions"] = true
		}
		if strings.Contains(featChoiceOwner(acquired, record), "@") {
			descriptor["legacyId"] = "feat:" + acquired.featID + ":" + text(choice["id"])
			descriptor["acquisition"] = Object{"id": acquired.id, "name": acquired.name, "classId": acquired.classID, "level": acquired.level}
			descriptor["name"] = firstText(record["name"], acquired.featID)
			if len(choices) > 1 {
				descriptor["choiceName"] = firstText(choice["prompt"], guidanceLabel(text(choice["id"])))
			}
		}
		result = append(result, descriptor)
	}
	return result
}
