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
	return entry.ClassID + ":" + entry.Source.Kind + ":" + entry.Source.ID + ":" + string(entry.Kind) + ":" + entry.Key
}

func validateClassReplacementLedger(input character.Inputs, profile Ruleset, result *character.Result) {
	seen := map[string]bool{}
	for _, entry := range input.Build.Replacements {
		key := fmt.Sprintf("%s:%d", replacementOwner(entry), entry.ClassLevel)
		if entry.ClassID == "" || entry.Key == "" || entry.Source.ID == "" || !contains([]string{"class", "subclass", "feature"}, entry.Source.Kind) ||
			!entry.Kind.Valid() || entry.ClassLevel < 2 || entry.ClassLevel > profile.Constants.Character.MaximumLevel ||
			entry.Slot < 0 || entry.Slot > 499 || entry.Out == "" || entry.In == "" || entry.Out == entry.In ||
			!contains([]string{"recorded", "import"}, entry.Origin) || seen[key] {
			addCharacterIssue(result, "class-replacement-ledger", "build", "The recorded class replacement allowance is invalid.", character.SeverityBlocker, nil)
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
		if entry.Kind != character.ReplaceFeat {
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

func classReplacementOptions(input character.Inputs, records Records, profile Ruleset, result *character.Result) []character.ReplacementGuidance {
	options := []character.ReplacementGuidance{}
	add := func(kind character.ReplacementKind, key, classID string, source character.Source, acquiredAt int, picked []character.PickedOption, candidates []character.Option) {
		ref := character.Reference{Kind: string(source.Type), ID: source.ID}
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
		if kind == character.ReplaceFeat && remaining > 0 {
			filtered := []character.Option{}
			for _, candidate := range candidates {
				for _, selected := range picked {
					if selected.ID == candidate.ID {
						continue
					}
					next := cloneCharacter(input)
					entry := owner
					entry.Slot, entry.Out, entry.In, entry.Origin = selected.Slot, selected.ID, candidate.ID, "recorded"
					applyClassReplacement(&next, entry)
					check := character.Result{}
					validateSelectedProgression(next, records, profile, &check, progressionChecks{feats: true})
					if !characterHasBlocker(check) {
						filtered = append(filtered, candidate)
						break
					}
				}
			}
			candidates = filtered
		}
		name := firstText(recordByID(records, ref.Kind, ref.ID)["name"], ref.ID)
		options = append(options, character.ReplacementGuidance{Kind: kind, Key: key, ClassID: classID, ClassLevel: level, Source: ref, Name: name, Remaining: remaining, Picked: picked, Options: candidates})
	}
	for _, choice := range result.Plan.ClassChoices {
		// The bounded contract currently supports one replacement per grant/level.
		if choice.Kind != character.ChoiceFeat || choice.ChangeOn != "classLevel" || choice.LevelReplacements != 1 {
			continue
		}
		picked := []character.PickedOption{}
		for _, selected := range input.Build.Choices {
			if selected.ID != choice.ID {
				continue
			}
			var id string
			if json.Unmarshal(selected.Value, &id) == nil && id != "" {
				picked = append(picked, character.PickedOption{ID: id, Slot: selected.Slot, Label: firstText(recordByID(records, "feat", id)["name"], id)})
			}
		}
		source := character.Source{}
		if choice.Source != nil {
			source = *choice.Source
		}
		add(character.ReplaceFeat, choice.ID, choice.ClassID, source, max(1, source.Level), picked, builderChoiceOptions(choice, nil, records))
	}
	for _, choice := range result.SpellOptions.PendingChoices {
		if choice.LevelReplacements != 1 {
			continue
		}
		source := choice.Source
		record := recordByID(records, string(source.Type), source.ID)
		classID := text(record["classId"])
		if source.Type == character.SourceClass {
			classID = source.ID
		}
		picked, candidates := []character.PickedOption{}, []character.Option{}
		for slot, id := range input.Build.Spells.GrantChoices[choice.Key] {
			picked = append(picked, character.PickedOption{ID: id, Slot: slot, Label: firstText(recordByID(records, "spell", id)["name"], id)})
		}
		for _, id := range choice.EligibleSpellIDs {
			if !slices.Contains(input.Build.Spells.GrantChoices[choice.Key], id) {
				candidates = append(candidates, guidanceOption(id, recordByID(records, "spell", id)))
			}
		}
		acquiredAt := choice.AcquiredAt
		if acquiredAt == 0 {
			acquiredAt = 1
		}
		add(character.ReplaceSpell, choice.Key, classID, source, acquiredAt, picked, candidates)
	}
	return options
}

func applyClassReplacement(input *character.Inputs, entry character.ClassReplacement) {
	if entry.Kind == character.ReplaceFeat {
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

func replaceClassChoice(input character.Inputs, command character.PlayCommand, records Records, profile Ruleset) (character.Result, error) {
	current := EvaluateCharacter(input, records, profile)
	if !current.Guidance.CanSave {
		return current, fmt.Errorf("Resolve invalid build choices before replacing a class choice.")
	}
	for _, option := range current.Guidance.ClassReplacements {
		if option.Kind != command.Kind || option.Key != command.Key || option.Remaining < 1 {
			continue
		}
		for _, selected := range option.Picked {
			if selected.ID != command.Out {
				continue
			}
			for _, candidate := range option.Options {
				if candidate.ID != command.Ref || command.Out == command.Ref {
					continue
				}
				next := cloneCharacter(current.Inputs)
				applyClassReplacement(&next, character.ClassReplacement{Origin: "recorded", Source: option.Source, ClassID: option.ClassID, ClassLevel: option.ClassLevel, Kind: option.Kind, Key: option.Key, Slot: selected.Slot, Out: command.Out, In: command.Ref})
				result := EvaluateCharacter(next, records, profile)
				if !result.Guidance.CanSave {
					return current, fmt.Errorf("The replacement conflicts with another character choice.")
				}
				return result, nil
			}
		}
	}
	return current, fmt.Errorf("No eligible class-level replacement remains for this choice.")
}
