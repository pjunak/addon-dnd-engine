package rules

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/pjunak/addon-dnd-engine/character"
)

type conditionDefinition struct {
	ID                   string          `json:"id"`
	Name                 string          `json:"name"`
	LabelKey             string          `json:"labelKey,omitempty"`
	Summary              string          `json:"summary"`
	MaximumLevel         int             `json:"maximumLevel"`
	SpeedZero            bool            `json:"speedZero,omitempty"`
	SpeedPenaltyPerLevel int             `json:"speedPenaltyPerLevel,omitempty"`
	D20PenaltyPerLevel   int             `json:"d20PenaltyPerLevel,omitempty"`
	TerminalLevel        int             `json:"terminalLevel,omitempty"`
	Rolls                []conditionRoll `json:"rolls,omitempty"`
	EndsConcentration    bool            `json:"endsConcentration,omitempty"`
}

// conditionRoll is one of the character's own rolls a condition gives
// Advantage or Disadvantage on, as in grants.advantages.
type conditionRoll struct {
	On        character.RollKind `json:"on"`
	Mode      character.RollMode `json:"mode"`
	Abilities []string           `json:"abilities,omitempty"`
	When      string             `json:"when,omitempty"`
}

var conditionID = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// A complete source declaration is the authority. A malformed or missing
// catalog never falls back to an Engine-owned edition or inferred prose.
func conditionDefinitions(records Records, profile Ruleset) ([]conditionDefinition, character.Reference) {
	ref := character.Reference{Kind: "rule", ID: profile.Constants.Character.ConditionRules}
	if ref.ID == "" {
		return nil, ref
	}
	record := recordByID(records, ref.Kind, ref.ID)
	body, err := json.Marshal(record["conditionDefinitions"])
	if err != nil {
		return nil, ref
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var definitions []conditionDefinition
	if decoder.Decode(&definitions) != nil || len(definitions) == 0 || len(definitions) > 100 {
		return nil, ref
	}
	seen := map[string]bool{}
	for _, definition := range definitions {
		if !conditionID.MatchString(definition.ID) || len(definition.ID) > 100 || seen[definition.ID] || strings.TrimSpace(definition.Name) == "" || utf8.RuneCountInString(definition.Name) > 120 || utf8.RuneCountInString(definition.Summary) > 2000 || definition.MaximumLevel < 1 || definition.MaximumLevel > 20 || definition.SpeedPenaltyPerLevel < 0 || definition.SpeedPenaltyPerLevel > 100 || definition.D20PenaltyPerLevel < 0 || definition.D20PenaltyPerLevel > 100 || definition.TerminalLevel < 0 || definition.TerminalLevel > definition.MaximumLevel {
			return nil, ref
		}
		if len(definition.Rolls) > 10 {
			return nil, ref
		}
		for _, roll := range definition.Rolls {
			if !roll.On.Valid() || !roll.Mode.Valid() || utf8.RuneCountInString(roll.When) > 200 {
				return nil, ref
			}
			for _, ability := range roll.Abilities {
				if !contains(Abilities[:], ability) {
					return nil, ref
				}
			}
		}
		seen[definition.ID] = true
	}
	return definitions, ref
}

func validateCharacterConditions(input character.Inputs, records Records, profile Ruleset, result *character.Result) {
	definitions, ref := conditionDefinitions(records, profile)
	byID := map[string]conditionDefinition{}
	for _, definition := range definitions {
		byID[definition.ID] = definition
	}
	seen := map[string]bool{}
	for _, condition := range input.Play.Conditions {
		definition, exists := byID[condition.ID]
		if !exists || seen[condition.ID] || condition.Level < 1 || condition.Level > definition.MaximumLevel {
			addCharacterIssue(result, "condition:"+condition.ID, "conditions", "Choose a distinct available condition and a supported level, or remove it.", character.SeverityBlocker, &ref)
		}
		seen[condition.ID] = true
	}
}

func characterConditions(input character.Inputs, records Records, profile Ruleset, result *character.Result) {
	definitions, ref := conditionDefinitions(records, profile)
	byID, options := map[string]conditionDefinition{}, []character.ConditionOption{}
	sheet := result.Sheet.Calculation
	var immunities []string
	if sheet != nil {
		immunities = sheet.ConditionImmunities
	}
	for _, definition := range definitions {
		byID[definition.ID] = definition
		options = append(options, character.ConditionOption{ID: definition.ID, Name: definition.Name, LabelKey: definition.LabelKey, Summary: definition.Summary, MaximumLevel: definition.MaximumLevel, CanAdd: !contains(immunities, definition.ID), Reference: ref})
	}
	result.Guidance.Conditions = character.ConditionGuidance{Available: len(definitions) > 0, Options: options}
	rows, terms, sources := []character.SheetCondition{}, []character.Term{}, []character.Reference{}
	penalty, speedPenalty, speedZero := 0, 0, false
	rollEntries := []character.RollEntry{}
	seen := map[string]bool{}
	for _, condition := range input.Play.Conditions {
		definition, exists := byID[condition.ID]
		status := character.ConditionActive
		if !exists || seen[condition.ID] || condition.Level < 1 || condition.Level > definition.MaximumLevel {
			status = character.ConditionUnavailable
		} else if contains(immunities, condition.ID) {
			status = character.ConditionImmune
		}
		seen[condition.ID] = true
		name := definition.Name
		if name == "" {
			name = condition.ID
		}
		reference := ref
		row := character.SheetCondition{ID: condition.ID, Level: condition.Level, Name: name, LabelKey: definition.LabelKey, Summary: definition.Summary, Status: status, Reference: &reference}
		if status == character.ConditionActive {
			penalty += definition.D20PenaltyPerLevel * condition.Level
			speedPenalty += definition.SpeedPenaltyPerLevel * condition.Level
			speedZero = speedZero || definition.SpeedZero
			terminal := definition.TerminalLevel > 0 && condition.Level >= definition.TerminalLevel
			row.Terminal = &terminal
			row.EndsConcentration = definition.EndsConcentration
			for _, roll := range definition.Rolls {
				declared := Object{"on": string(roll.On), "mode": string(roll.Mode), "when": roll.When}
				if len(roll.Abilities) > 0 {
					declared["abilities"] = anyStrings(roll.Abilities)
				}
				if entry := rollEntry(declared, name, character.Source{Type: character.SourceCondition, ID: condition.ID}); entry != nil {
					rollEntries = append(rollEntries, *entry)
				}
			}
		}
		rows = append(rows, row)
		terms = append(terms, character.Term{Label: name, Value: condition.Level, Status: string(status), Source: &reference})
	}
	if len(definitions) > 0 && len(rows) > 0 {
		sources = append(sources, ref)
	}
	grantedTerms, grantedSources := characterGrantedConditions(input, records, immunities, &rows)
	result.Sheet.Conditions = rows
	result.Sheet.ConditionEffects = character.ConditionEffects{D20Adjustment: -penalty}
	// Condition roll effects join the feature and armor entries; the net marks
	// are recomputed so Advantage and Disadvantage from different sources cancel.
	if len(rollEntries) > 0 && sheet != nil {
		sheet.Advantages = append(sheet.Advantages, rollEntries...)
		applyRollMarks(sheet)
	}
	result.Explanations["conditions"] = character.Explanation{Label: "Conditions", Formula: "Authored conditions are preserved until explicitly changed. Movement restrictions apply to displayed speeds. Apply the separate D20 adjustment to rolls; ability modifiers, printed roll bonuses and spell save DCs remain unchanged. Situational effects and ending conditions require adjudication. Active items grant their own conditions, one level per item, with no automated effect.", Value: rows, Terms: append(append([]character.Term{}, terms...), grantedTerms...), Sources: append(append([]character.Reference{}, sources...), grantedSources...)}
	result.Explanations["conditionEffects.d20Adjustment"] = character.Explanation{Label: "D20 roll adjustment", Formula: "Sum of source-declared penalties for active condition levels. Apply once to each D20 Test, including Initiative. This adjustment is not included in printed bonuses or spell save DCs.", Value: -penalty, Terms: terms, Sources: sources}
	if !speedZero && speedPenalty == 0 || sheet == nil {
		return
	}
	// Speed-zero restrictions apply after equipment and DM bonuses. They must
	// also constrain independently granted movement, not just walking speed.
	for _, speed := range []struct {
		path    string
		value   *int
		present bool
	}{
		{"derived.speed", &sheet.Derived.Speed, true}, {"flySpeed", &sheet.FlySpeed, true},
		{"swimSpeed", &sheet.SwimSpeed, sheet.SwimSpeed != 0}, {"climbSpeed", &sheet.ClimbSpeed, sheet.ClimbSpeed != 0},
		{"burrowSpeed", &sheet.BurrowSpeed, sheet.BurrowSpeed != 0},
	} {
		if !speed.present {
			continue
		}
		before := *speed.value
		after := max(0, before-speedPenalty)
		if speedZero {
			after = 0
		}
		*speed.value = after
		explanation := result.Explanations[speed.path]
		explanation.Value = after
		explanation.Formula += " Apply source-declared condition restrictions after other speed effects; zero Speed takes precedence."
		explanation.Terms = append(explanation.Terms, character.Term{Label: "Before conditions", Value: before})
		explanation.Terms = append(explanation.Terms, terms...)
		explanation.Sources = append(explanation.Sources, sources...)
		result.Explanations[speed.path] = explanation
	}
	// The top-level copy mirrors derived.speed, as in applyCharacterEffects.
	sheet.Speed = sheet.Derived.Speed
}

// characterGrantedConditions adds one row per condition ID that active items
// grant. Its level is the number of active items granting it, and the first
// such item in inventory order names it. The rows are not authored play state,
// cannot be removed and have no automated effect; condition immunity marks
// them immune.
func characterGrantedConditions(input character.Inputs, records Records, immunities []string, rows *[]character.SheetCondition) ([]character.Term, []character.Reference) {
	granted, byID := []string{}, map[string]*character.SheetCondition{}
	for _, raw := range characterItemGrantSources(input, records) {
		item := object(raw)
		if !truth(item["active"]) {
			continue
		}
		for _, condition := range objects(object(item["grants"])["conditions"]) {
			id := text(condition["id"])
			row := byID[id]
			if row == nil {
				status := character.ConditionActive
				if contains(immunities, id) {
					status = character.ConditionImmune
				}
				row = &character.SheetCondition{ID: id, Name: text(condition["name"]), Summary: text(condition["summary"]), Granted: true, Sources: []character.Source{}, Status: status}
				byID[id] = row
				granted = append(granted, id)
			}
			row.Level++
			row.Sources = append(row.Sources, sourceFromObject(object(item["source"])))
		}
	}
	terms, refs := []character.Term{}, []character.Reference{}
	for _, id := range granted {
		row := byID[id]
		*rows = append(*rows, *row)
		first := grantSourceReference(row.Sources[0])
		terms = append(terms, character.Term{Label: row.Name, Value: row.Level, Status: string(row.Status), Source: &first})
		for _, source := range row.Sources {
			refs = append(refs, grantSourceReference(source))
		}
	}
	return terms, uniqueCharacterReferences(refs)
}
