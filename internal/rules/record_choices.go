package rules

import (
	"sort"
	"strings"
)

// A choice can offer records of another kind (optionKind), such as Beast stat
// blocks for Wild Shape or magic items for an Artificer's plans. Its options
// are the explicit from list, the fromByLevel lists the class has reached and
// the records matching filter (as overridden by the reached filterByLevel
// steps). Filters are a small closed vocabulary; an unknown key matches
// nothing, so it can never widen a choice.
func recordChoiceOptions(choice Object, classLevel int, records Records) []string {
	kind := text(choice["optionKind"])
	ids := stringsOf(choice["from"])
	levels := []int{}
	for level := range object(choice["fromByLevel"]) {
		levels = append(levels, integer(level, 0))
	}
	for level := range object(choice["filterByLevel"]) {
		levels = append(levels, integer(level, 0))
	}
	sort.Ints(levels)
	filter := cloneObject(object(choice["filter"]))
	for _, level := range levels {
		if level < 1 || level > classLevel {
			continue
		}
		ids = append(ids, stringsOf(object(choice["fromByLevel"])[jsonNumber(level)])...)
		for key, value := range object(object(choice["filterByLevel"])[jsonNumber(level)]) {
			filter[key] = value
		}
	}
	if choice["filter"] != nil || len(object(choice["filterByLevel"])) > 0 {
		for _, record := range recordCatalog(records, kind) {
			if recordMatchesFilter(record, filter) {
				ids = append(ids, text(record["id"]))
			}
		}
	}
	result := []string{}
	for _, id := range unique(ids) {
		if recordView(records, kind, id) != nil {
			result = append(result, id)
		}
	}
	return result
}

func recordMatchesFilter(record, filter Object) bool {
	if len(filter) == 0 {
		return false
	}
	baseType := strings.ToLower(strings.TrimSpace(strings.Split(text(record["itemType"]), "(")[0]))
	for key, value := range filter {
		switch key {
		case "creatureType":
			if !strings.EqualFold(text(record["creatureType"]), text(value)) {
				return false
			}
		case "maxCr":
			if record["crValue"] == nil || number(record["crValue"], 0) > number(value, 0) {
				return false
			}
		case "fly":
			// fly: false excludes stat blocks with a Fly Speed.
			if hasFly := integer(object(record["speeds"])["fly"], 0) > 0; !truth(value) && hasFly {
				return false
			}
		case "rarity":
			if !containsFold(stringsOf(value), text(record["rarity"])) {
				return false
			}
		case "itemTypes":
			if !containsFold(stringsOf(value), baseType) {
				return false
			}
		case "notItemTypes":
			if containsFold(stringsOf(value), baseType) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func containsFold(values []string, wanted string) bool {
	for _, value := range values {
		if strings.EqualFold(value, wanted) {
			return true
		}
	}
	return false
}

// characterSelections lists the class choices whose picks are not features of
// their own (Divine Order, Hunter's Prey, Wild Shape forms), so the play sheet
// shows what was chosen. Chosen option features are already features. A pick
// of another record kind is retained as evidence for offline reading.
func characterSelections(decisions, plan, guidance Object, records Records) []any {
	rows := []any{}
	for _, choice := range objects(plan["classChoices"]) {
		if text(choice["kind"]) != "enumerated" || text(choice["fromCategory"]) != "" {
			continue
		}
		picked := choiceValues(decisions, choice)
		if len(picked) == 0 {
			continue
		}
		entry := object(object(guidance["choices"])[text(choice["id"])])
		labels := map[string]string{}
		for _, option := range objects(entry["options"]) {
			labels[text(option["id"])] = text(option["label"])
		}
		values := []any{}
		for _, id := range picked {
			value := Object{"id": id, "label": firstText(labels[id], guidanceLabel(id))}
			if kind := text(choice["optionKind"]); kind != "" {
				value["kind"] = kind
				recordView(records, kind, id)
			}
			values = append(values, value)
		}
		row := Object{"id": text(choice["id"]), "source": cloneValue(choice["source"]), "values": values}
		for _, key := range []string{"label", "labelKey", "labelArgs"} {
			if value, exists := entry[key]; exists {
				row[key] = cloneValue(value)
			}
		}
		rows = append(rows, row)
	}
	return rows
}
