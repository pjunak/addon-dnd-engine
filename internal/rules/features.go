package rules

import (
	"sort"
	"strings"

	"github.com/pjunak/addon-dnd-engine/character"
)

func hydrateFeatures(sheet *character.Calculation, classes []resolvedClass, records Records, selections Object) {
	features := make([]character.SheetFeature, 0)
	allRecords := recordCatalog(records, "feature")
	// Generic slots such as Ability Score Improvement are one shared record that
	// many class tables name, so a label without a class record falls back to it.
	shared := make(map[string]Object)
	for _, feature := range allRecords {
		if truth(feature["shared"]) {
			shared[normalizedName(feature["name"])] = feature
		}
	}
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
		subclass := recordByID(records, "subclass", current.Subclass)
		for _, level := range orderedLevels {
			if level > current.Level {
				continue
			}
			source := character.Source{Type: character.SourceClass, ID: current.ID, Level: level}
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
				// A chosen subclass replaces its table slot ("Wizard Subclass")
				// with the subclass itself, and its own feature rows replace the
				// generic "Subclass Feature" placeholders.
				if subclass != nil && isSubclassSlotLabel(current, label, level) {
					if match != nil {
						taken[text(match["id"])] = struct{}{}
					}
					features = append(features, character.SheetFeature{
						ID: text(subclass["id"]), Name: text(subclass["name"]), Kind: "subclass", Source: source,
					})
					continue
				}
				if subclass != nil && normalizedName(label) == "subclass feature" {
					continue
				}
				if match != nil {
					taken[text(match["id"])] = struct{}{}
					features = append(features, character.SheetFeature{ID: text(match["id"]), Name: text(match["name"]), Source: source})
				} else if generic := shared[normalizedName(label)]; generic != nil {
					features = append(features, character.SheetFeature{ID: text(generic["id"]), Name: text(generic["name"]), Source: source})
				} else if _, drifted := ownedNames[normalizedName(label)]; !drifted {
					features = append(features, character.SheetFeature{ID: label, Source: source})
				}
			}
			for _, feature := range available {
				if _, used := taken[text(feature["id"])]; used {
					continue
				}
				features = append(features, character.SheetFeature{ID: text(feature["id"]), Name: text(feature["name"]), Source: source})
			}
		}
		features = append(features, subclassFeatures(current, records, allRecords)...)
		features = append(features, optionFeatureRows(current, records, selections)...)
	}
	sheet.Features = features
}

// Chosen options are acquired features of the class (or subclass) that
// offers them, listed at the level their record names.
func optionFeatureRows(current resolvedClass, records Records, selections Object) []character.SheetFeature {
	rows := []character.SheetFeature{}
	for _, option := range chosenOptions(current.ID, current.Level, current.Subclass, selections, records) {
		source := character.Source{Type: character.SourceClass, ID: current.ID, Level: min(current.Level, max(1, integer(option.feature["level"], 1)))}
		if subclassID := text(option.feature["subclassId"]); subclassID != "" {
			source.Type, source.ID = character.SourceSubclass, subclassID
		}
		row := character.SheetFeature{ID: text(option.feature["id"]), Name: text(option.feature["name"]), Source: source, Category: text(option.feature["category"])}
		if option.count > 1 {
			row.Count = option.count
		}
		rows = append(rows, row)
	}
	return rows
}

// The subclass-choice slot is the class table label "<Class> Subclass" at the
// class's subclass level; the compendium's data contract reserves that label.
func isSubclassSlotLabel(current resolvedClass, label string, level int) bool {
	return level == classSubclassLevel(current.Record) &&
		normalizedName(label) == normalizedName(current.Name)+" subclass"
}

// Inline subclass tables use local IDs. Resolve them within their owning
// subclass before exposing feature identities to prerequisites and evidence.
func subclassFeatures(current resolvedClass, records Records, allRecords []Object) []character.SheetFeature {
	subclass := recordByID(records, "subclass", current.Subclass)
	if subclass == nil {
		return nil
	}
	owned := []Object{}
	for _, feature := range allRecords {
		if text(feature["classId"]) == current.ID && text(feature["subclassId"]) == current.Subclass &&
			text(feature["category"]) == "" && feature["level"] != nil {
			owned = append(owned, feature)
		}
	}
	result := []character.SheetFeature{}
	seen := map[string]bool{}
	appendFeature := func(feature Object) {
		id, level := text(feature["id"]), integer(feature["level"], 0)
		if seen[id] || level > current.Level {
			return
		}
		seen[id] = true
		result = append(result, character.SheetFeature{
			ID: id, Name: text(feature["name"]),
			Source: character.Source{Type: character.SourceSubclass, ID: current.Subclass, Level: level},
		})
	}
	for _, inline := range objects(subclass["features"]) {
		resolved := inline
		for _, feature := range owned {
			if text(feature["id"]) == text(inline["id"]) ||
				text(feature["localId"]) != "" && text(feature["localId"]) == text(inline["id"]) {
				resolved = feature
				break
			}
		}
		appendFeature(resolved)
	}
	for _, feature := range owned {
		appendFeature(feature)
	}
	return result
}

func acquiredFeatRows(feats, acquisitions []Object) []character.SheetFeat {
	counts := map[string]int{}
	for _, acquisition := range acquisitions {
		counts[text(acquisition["id"])]++
	}
	result := []character.SheetFeat{}
	seen := map[string]bool{}
	for _, feat := range feats {
		id := text(feat["id"])
		if seen[id] {
			continue
		}
		seen[id] = true
		result = append(result, character.SheetFeat{ID: id, Name: firstText(feat["name"], id), Count: max(1, counts[id])})
	}
	return result
}

func normalizedName(value any) string {
	return strings.ToLower(strings.TrimSpace(text(value)))
}
