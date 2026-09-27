package rules

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/pjunak/addon-dnd-engine/character"
)

func classLevel(input character.Inputs, id string) int {
	count := 0
	for _, level := range input.Build.Levels {
		if level.ClassID == id {
			count++
		}
	}
	return count
}

func replacementOwner(entry character.ClassReplacement) string {
	return entry.ClassID + ":" + entry.Source.Kind + ":" + entry.Source.ID + ":" + entry.Kind + ":" + entry.Key
}

func validateClassReplacementLedger(input character.Inputs, profile Ruleset, result *character.Result) {
	seen := map[string]bool{}
	for _, entry := range input.Build.Replacements {
		key := fmt.Sprintf("%s:%d", replacementOwner(entry), entry.ClassLevel)
		if entry.ClassID == "" || entry.Key == "" || entry.Source.ID == "" || !contains([]string{"class", "subclass", "feature"}, entry.Source.Kind) ||
			!contains([]string{"feat", "spell"}, entry.Kind) || entry.ClassLevel < 2 || entry.ClassLevel > profile.Constants.Character.MaximumLevel ||
			entry.Slot < 0 || entry.Slot > 499 || entry.Out == "" || entry.In == "" || entry.Out == entry.In ||
			!contains([]string{"recorded", "import"}, entry.Origin) || seen[key] {
			addCharacterIssue(result, "class-replacement-ledger", "build", "The recorded class replacement allowance is invalid.", "blocker", nil)
		}
		seen[key] = true
	}
}

// Current-state corrections remain possible. Only an unchanged replacement chain
// is rewound while validating earlier multiclass/feat acquisition prefixes.
func replacementPrefix(original character.Inputs, end int) character.Inputs {
	input := cloneCharacter(original)
	input.Build.Levels = input.Build.Levels[:end]
	if len(input.Build.Replacements) == 0 {
		return input
	}
	active := map[string]bool{}
	for index := len(input.Build.Replacements) - 1; index >= 0; index-- {
		entry := input.Build.Replacements[index]
		if entry.Kind != "feat" {
			continue
		}
		owner := fmt.Sprintf("%s:%d", replacementOwner(entry), entry.Slot)
		for i := range input.Build.Choices {
			choice := &input.Build.Choices[i]
			if choice.ID != entry.Key || choice.Slot != entry.Slot {
				continue
			}
			var value string
			_ = json.Unmarshal(choice.Value, &value)
			if _, seen := active[owner]; !seen {
				active[owner] = value == entry.In
			}
			if active[owner] && entry.ClassLevel <= classLevel(original, entry.ClassID) && entry.ClassLevel > classLevel(input, entry.ClassID) && value == entry.In {
				choice.Value = replacementValue(entry.Out)
			}
		}
	}
	return input
}

func classReplacementOptions(input character.Inputs, records Records, profile Ruleset, result *character.Result) []any {
	options := []any{}
	add := func(kind, key, classID string, source Object, acquiredAt int, picked, candidates []any) {
		ref := character.Reference{Kind: text(source["type"]), ID: text(source["id"])}
		level := classLevel(input, classID)
		if classID == "" || level <= acquiredAt || !contains([]string{"class", "subclass", "feature"}, ref.Kind) {
			return
		}
		owner := character.ClassReplacement{Source: ref, ClassID: classID, ClassLevel: level, Kind: kind, Key: key}
		remaining := 1
		for _, spent := range input.Build.Replacements {
			if replacementOwner(spent) == replacementOwner(owner) && spent.ClassLevel == level {
				remaining = 0
			}
		}
		if kind == "feat" && remaining > 0 {
			filtered := []any{}
			for _, raw := range candidates {
				candidate := object(raw)
				for _, selected := range objects(picked) {
					if text(selected["id"]) == text(candidate["id"]) {
						continue
					}
					next := cloneCharacter(input)
					entry := owner
					entry.Slot, entry.Out, entry.In, entry.Origin = integer(selected["slot"], 0), text(selected["id"]), text(candidate["id"]), "recorded"
					applyClassReplacement(&next, entry)
					check := character.Result{}
					validateSelectedProgression(next, records, profile, &check, progressionChecks{feats: true})
					allowed := true
					for _, issue := range check.Issues {
						if issue.Severity == "blocker" {
							allowed = false
						}
					}
					if allowed {
						filtered = append(filtered, candidate)
						break
					}
				}
			}
			candidates = filtered
		}
		name := firstText(recordByID(records, ref.Kind, ref.ID)["name"], ref.ID)
		options = append(options, Object{"kind": kind, "key": key, "classId": classID, "classLevel": level, "source": Object{"kind": ref.Kind, "id": ref.ID}, "name": name, "remaining": remaining, "picked": picked, "options": candidates})
	}
	for _, choice := range objects(result.Plan["classChoices"]) {
		// The bounded contract currently supports one replacement per grant/level.
		if text(choice["kind"]) != "feat" || text(choice["changeOn"]) != "classLevel" || number(choice["levelReplacements"], 0) != 1 {
			continue
		}
		picked := []any{}
		for _, selected := range input.Build.Choices {
			if selected.ID != text(choice["id"]) {
				continue
			}
			var id string
			if json.Unmarshal(selected.Value, &id) == nil && id != "" {
				picked = append(picked, Object{"id": id, "slot": selected.Slot, "label": firstText(recordByID(records, "feat", id)["name"], id)})
			}
		}
		add("feat", text(choice["id"]), text(choice["classId"]), object(choice["source"]), integer(object(choice["source"])["level"], 1), picked, builderChoiceOptions(choice, nil, records))
	}
	for _, choice := range objects(result.SpellOptions["pendingChoices"]) {
		if number(choice["levelReplacements"], 0) != 1 {
			continue
		}
		source := object(choice["source"])
		record := recordByID(records, text(source["type"]), text(source["id"]))
		classID := text(record["classId"])
		if text(source["type"]) == "class" {
			classID = text(source["id"])
		}
		picked, candidates := []any{}, []any{}
		for slot, id := range input.Build.Spells.GrantChoices[text(choice["key"])] {
			picked = append(picked, Object{"id": id, "slot": slot, "label": firstText(recordByID(records, "spell", id)["name"], id)})
		}
		for _, id := range stringsOf(choice["eligibleSpellIds"]) {
			if !slices.Contains(input.Build.Spells.GrantChoices[text(choice["key"])], id) {
				candidates = append(candidates, guidanceOption(id, recordByID(records, "spell", id)))
			}
		}
		add("spell", text(choice["key"]), classID, source, integer(choice["acquiredAt"], 1), picked, candidates)
	}
	return options
}

func applyClassReplacement(input *character.Inputs, entry character.ClassReplacement) {
	if entry.Kind == "feat" {
		for i := range input.Build.Choices {
			choice := &input.Build.Choices[i]
			if choice.ID == entry.Key && choice.Slot == entry.Slot {
				choice.Value = replacementValue(entry.In)
			}
		}
	} else {
		input.Build.Spells.GrantChoices[entry.Key][entry.Slot] = entry.In
	}
	input.Build.Replacements = append(input.Build.Replacements, entry)
}

func replacementValue(value string) json.RawMessage {
	encoded, _ := json.Marshal(value)
	return encoded
}

func replaceClassChoice(input character.Inputs, change Object, records Records, profile Ruleset) (character.Result, error) {
	current := EvaluateCharacter(input, records, profile)
	if current.Guidance["canSave"] != true || len(change) != 5 {
		return current, fmt.Errorf("Resolve invalid build choices before replacing a class choice.")
	}
	for _, option := range objects(current.Guidance["classReplacements"]) {
		if text(option["kind"]) != text(change["kind"]) || text(option["key"]) != text(change["key"]) || integer(option["remaining"], 0) < 1 {
			continue
		}
		for _, selected := range objects(option["picked"]) {
			if text(selected["id"]) != text(change["out"]) {
				continue
			}
			for _, candidate := range objects(option["options"]) {
				if text(candidate["id"]) != text(change["ref"]) || text(change["out"]) == text(change["ref"]) {
					continue
				}
				next := cloneCharacter(current.Inputs)
				ref := object(option["source"])
				applyClassReplacement(&next, character.ClassReplacement{Origin: "recorded", Source: character.Reference{Kind: text(ref["kind"]), ID: text(ref["id"])}, ClassID: text(option["classId"]), ClassLevel: integer(option["classLevel"], 0), Kind: text(option["kind"]), Key: text(option["key"]), Slot: integer(selected["slot"], 0), Out: text(change["out"]), In: text(change["ref"])})
				result := EvaluateCharacter(next, records, profile)
				if result.Guidance["canSave"] != true {
					return current, fmt.Errorf("The replacement conflicts with another character choice.")
				}
				return result, nil
			}
		}
	}
	return current, fmt.Errorf("No eligible class-level replacement remains for this choice.")
}
