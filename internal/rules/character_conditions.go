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
	ID                   string `json:"id"`
	Name                 string `json:"name"`
	LabelKey             string `json:"labelKey,omitempty"`
	Summary              string `json:"summary"`
	MaximumLevel         int    `json:"maximumLevel"`
	SpeedZero            bool   `json:"speedZero,omitempty"`
	SpeedPenaltyPerLevel int    `json:"speedPenaltyPerLevel,omitempty"`
	D20PenaltyPerLevel   int    `json:"d20PenaltyPerLevel,omitempty"`
	TerminalLevel        int    `json:"terminalLevel,omitempty"`
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
			addCharacterIssue(result, "condition:"+condition.ID, "conditions", "Choose a distinct available condition and a supported level, or remove it.", "blocker", &ref)
		}
		seen[condition.ID] = true
	}
}

func characterConditions(input character.Inputs, records Records, profile Ruleset, result *character.Result) {
	definitions, ref := conditionDefinitions(records, profile)
	byID, options := map[string]conditionDefinition{}, []any{}
	immunities := stringsOf(result.Sheet["conditionImmunities"])
	for _, definition := range definitions {
		byID[definition.ID] = definition
		options = append(options, Object{"id": definition.ID, "name": definition.Name, "labelKey": definition.LabelKey, "summary": definition.Summary, "maximumLevel": definition.MaximumLevel, "canAdd": !contains(immunities, definition.ID), "reference": ref})
	}
	result.Guidance["conditions"] = Object{"available": len(definitions) > 0, "options": options}
	rows, terms, sources := []any{}, []character.Term{}, []character.Reference{}
	penalty, speedPenalty, speedZero := 0, 0, false
	seen := map[string]bool{}
	for _, condition := range input.Play.Conditions {
		definition, exists := byID[condition.ID]
		status := "active"
		if !exists || seen[condition.ID] || condition.Level < 1 || condition.Level > definition.MaximumLevel {
			status = "unavailable"
		} else if contains(immunities, condition.ID) {
			status = "immune"
		}
		seen[condition.ID] = true
		name := definition.Name
		if name == "" {
			name = condition.ID
		}
		row := Object{"id": condition.ID, "level": condition.Level, "name": name, "labelKey": definition.LabelKey, "summary": definition.Summary, "status": status, "reference": ref}
		if status == "active" {
			penalty += definition.D20PenaltyPerLevel * condition.Level
			speedPenalty += definition.SpeedPenaltyPerLevel * condition.Level
			speedZero = speedZero || definition.SpeedZero
			row["terminal"] = definition.TerminalLevel > 0 && condition.Level >= definition.TerminalLevel
		}
		rows = append(rows, row)
		terms = append(terms, character.Term{Label: name, Value: condition.Level, Status: status, Source: &ref})
	}
	if len(definitions) > 0 && len(rows) > 0 {
		sources = append(sources, ref)
	}
	result.Sheet["conditions"] = rows
	result.Sheet["conditionEffects"] = Object{"d20Adjustment": -penalty}
	result.Explanations["conditions"] = character.Explanation{Label: "Conditions", Formula: "Authored conditions are preserved until explicitly changed. Movement restrictions apply to displayed speeds. Apply the separate D20 adjustment to rolls; ability modifiers, printed roll bonuses and spell save DCs remain unchanged. Situational effects and ending conditions require adjudication.", Value: rows, Terms: terms, Sources: sources}
	result.Explanations["conditionEffects.d20Adjustment"] = character.Explanation{Label: "D20 roll adjustment", Formula: "Sum of source-declared penalties for active condition levels. Apply once to each D20 Test, including Initiative. This adjustment is not included in printed bonuses or spell save DCs.", Value: -penalty, Terms: terms, Sources: sources}
	if !speedZero && speedPenalty == 0 {
		return
	}
	// Speed-zero restrictions apply after equipment and DM bonuses. They must
	// also constrain independently granted movement, not just walking speed.
	for _, key := range []string{"speed", "flySpeed", "swimSpeed", "climbSpeed", "burrowSpeed"} {
		parent, path := Object(result.Sheet), key
		if key == "speed" {
			parent, path = object(result.Sheet["derived"]), "derived.speed"
		}
		if value, exists := parent[key]; exists && value != nil {
			before := integer(value, 0)
			after := max(0, before-speedPenalty)
			if speedZero {
				after = 0
			}
			parent[key] = after
			if key == "speed" {
				// The top-level copy mirrors derived.speed, as in applyCharacterEffects.
				result.Sheet["speed"] = after
			}
			explanation := result.Explanations[path]
			explanation.Value = after
			explanation.Formula += " Apply source-declared condition restrictions after other speed effects; zero Speed takes precedence."
			explanation.Terms = append(explanation.Terms, character.Term{Label: "Before conditions", Value: before})
			explanation.Terms = append(explanation.Terms, terms...)
			explanation.Sources = append(explanation.Sources, sources...)
			result.Explanations[path] = explanation
		}
	}
}
