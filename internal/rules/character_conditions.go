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
	On        string   `json:"on"`
	Mode      string   `json:"mode"`
	Abilities []string `json:"abilities,omitempty"`
	When      string   `json:"when,omitempty"`
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
			if !contains(advantageRolls, roll.On) || (roll.Mode != "advantage" && roll.Mode != "disadvantage") || utf8.RuneCountInString(roll.When) > 200 {
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
	rollEntries := []any{}
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
			if definition.EndsConcentration {
				row["endsConcentration"] = true
			}
			for _, roll := range definition.Rolls {
				declared := Object{"on": roll.On, "mode": roll.Mode, "when": roll.When}
				if len(roll.Abilities) > 0 {
					declared["abilities"] = anyStrings(roll.Abilities)
				}
				if entry := rollEntry(declared, name, Object{"type": "condition", "id": condition.ID}); entry != nil {
					rollEntries = append(rollEntries, entry)
				}
			}
		}
		rows = append(rows, row)
		terms = append(terms, character.Term{Label: name, Value: condition.Level, Status: status, Source: &ref})
	}
	if len(definitions) > 0 && len(rows) > 0 {
		sources = append(sources, ref)
	}
	grantedTerms, grantedSources := characterGrantedConditions(input, records, immunities, &rows)
	result.Sheet["conditions"] = rows
	result.Sheet["conditionEffects"] = Object{"d20Adjustment": -penalty}
	// Condition roll effects join the feature and armor entries; the net marks
	// are recomputed so Advantage and Disadvantage from different sources cancel.
	if len(rollEntries) > 0 {
		result.Sheet["advantages"] = append(values(result.Sheet["advantages"]), rollEntries...)
		applyRollMarks(result.Sheet)
	}
	result.Explanations["conditions"] = character.Explanation{Label: "Conditions", Formula: "Authored conditions are preserved until explicitly changed. Movement restrictions apply to displayed speeds. Apply the separate D20 adjustment to rolls; ability modifiers, printed roll bonuses and spell save DCs remain unchanged. Situational effects and ending conditions require adjudication. Active items grant their own conditions, one level per item, with no automated effect.", Value: rows, Terms: append(append([]character.Term{}, terms...), grantedTerms...), Sources: append(append([]character.Reference{}, sources...), grantedSources...)}
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

// characterGrantedConditions adds one row per condition ID that active items
// grant. Its level is the number of active items granting it, and the first
// such item in inventory order names it. The rows are not authored play state,
// cannot be removed and have no automated effect; condition immunity marks
// them immune.
func characterGrantedConditions(input character.Inputs, records Records, immunities []string, rows *[]any) ([]character.Term, []character.Reference) {
	granted, byID := []Object{}, map[string]Object{}
	for _, raw := range characterItemGrantSources(input, records) {
		item := object(raw)
		if !truth(item["active"]) {
			continue
		}
		for _, condition := range objects(object(item["grants"])["conditions"]) {
			id := text(condition["id"])
			row := byID[id]
			if row == nil {
				status := "active"
				if contains(immunities, id) {
					status = "immune"
				}
				row = Object{"id": id, "name": text(condition["name"]), "summary": text(condition["summary"]), "level": 0, "granted": true, "sources": []any{}, "status": status}
				byID[id] = row
				granted = append(granted, row)
			}
			row["level"] = integer(row["level"], 0) + 1
			row["sources"] = append(values(row["sources"]), cloneObjectDeep(object(item["source"])))
		}
	}
	terms, refs := []character.Term{}, []character.Reference{}
	for _, row := range granted {
		*rows = append(*rows, row)
		first := grantSourceReference(objects(row["sources"])[0])
		terms = append(terms, character.Term{Label: text(row["name"]), Value: row["level"], Status: text(row["status"]), Source: &first})
		for _, source := range objects(row["sources"]) {
			refs = append(refs, grantSourceReference(source))
		}
	}
	return terms, uniqueCharacterReferences(refs)
}
