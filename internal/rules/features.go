package rules

import (
	"sort"
	"strings"
)

func hydrateFeatures(sheet Object, classes []resolvedClass, records Records) {
	features := make([]any, 0)
	allRecords := recordList(records, "feature")
	for _, current := range classes {
		ownedNames := make(map[string]struct{})
		recordsAt := make(map[int][]Object)
		for _, feature := range allRecords {
			if text(feature["classId"]) != current.ID || text(feature["subclassId"]) != "" ||
				text(feature["category"]) != "" || feature["level"] == nil {
				continue
			}
			ownedNames[normalizedName(feature["name"])] = struct{}{}
			level := integer(feature["level"], 0)
			recordsAt[level] = append(recordsAt[level], feature)
		}
		rowsAt := make(map[int]Object)
		levels := make(map[int]struct{})
		for level := range recordsAt {
			levels[level] = struct{}{}
		}
		for _, row := range objects(current.Record["progression"]) {
			level := integer(row["level"], 0)
			rowsAt[level] = row
			levels[level] = struct{}{}
		}
		orderedLevels := make([]int, 0, len(levels))
		for level := range levels {
			orderedLevels = append(orderedLevels, level)
		}
		sort.Ints(orderedLevels)
		for _, level := range orderedLevels {
			if level > current.Level {
				continue
			}
			source := Object{"type": "class", "id": current.ID, "level": level}
			available := recordsAt[level]
			taken := make(map[string]struct{})
			for _, label := range stringsOf(rowsAt[level]["features"]) {
				var match Object
				for _, candidate := range available {
					if _, used := taken[text(candidate["id"])]; !used &&
						normalizedName(candidate["name"]) == normalizedName(label) {
						match = candidate
						break
					}
				}
				if match != nil {
					taken[text(match["id"])] = struct{}{}
					features = append(features, Object{
						"id": text(match["id"]), "name": text(match["name"]), "source": source,
					})
				} else if _, drifted := ownedNames[normalizedName(label)]; !drifted {
					features = append(features, Object{"id": label, "source": source})
				}
			}
			for _, feature := range available {
				if _, used := taken[text(feature["id"])]; used {
					continue
				}
				features = append(features, Object{
					"id": text(feature["id"]), "name": text(feature["name"]), "source": source,
				})
			}
		}
		subclass := recordByID(records, "subclass", current.Subclass)
		for _, feature := range objects(subclass["features"]) {
			if integer(feature["level"], 0) <= current.Level {
				features = append(features, Object{
					"id": text(feature["id"]), "name": text(feature["name"]),
					"source": Object{
						"type": "subclass", "id": current.Subclass, "level": integer(feature["level"], 0),
					},
				})
			}
		}
	}
	sheet["features"] = features
}

func normalizedName(value any) string {
	return strings.ToLower(strings.TrimSpace(text(value)))
}
