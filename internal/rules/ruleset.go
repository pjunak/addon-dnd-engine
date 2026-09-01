package rules

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type Ruleset struct {
	ID             string              `json:"id"`
	RulesetID      string              `json:"rulesetId"`
	RulesetVersion int                 `json:"rulesetVersion"`
	Edition        string              `json:"edition"`
	Constants      RulesetConstants    `json:"constants"`
	Capabilities   RulesetCapabilities `json:"capabilities"`
	Builder        BuilderPolicy       `json:"builder"`
}

type RulesetConstants struct {
	AbilityCap           int              `json:"abilityCap"`
	AbilityCapHard       int              `json:"abilityCapHard"`
	AttunementLimit      int              `json:"attunementLimit"`
	ScrollCopyGPPerLevel float64          `json:"scrollCopyGpPerLevel"`
	PointBuy             PointBuyPolicy   `json:"pointBuy"`
	MulticlassSlots      map[string][]int `json:"multiclassSlots"`
	CasterFractions      CasterFractions  `json:"casterFractions"`
	PactMagic            PactMagicPolicy  `json:"pactMagic"`
	Spellbook            SpellbookPolicy  `json:"spellbook"`
	Rest                 RestPolicy       `json:"rest"`
}

type PointBuyPolicy struct {
	Budget int            `json:"budget"`
	Min    int            `json:"min"`
	Max    int            `json:"max"`
	Cost   map[string]int `json:"cost"`
}

type CasterFractions struct {
	Half  string `json:"half"`
	Third string `json:"third"`
}

type PactMagicPolicy struct {
	Tiers        []PactMagicTier `json:"tiers"`
	SlotLevelCap int             `json:"slotLevelCap"`
}

type PactMagicTier struct {
	Level int `json:"level"`
	Slots int `json:"slots"`
}

type SpellbookPolicy struct {
	BaseKnown     int `json:"baseKnown"`
	KnownPerLevel int `json:"knownPerLevel"`
}

type RestPolicy struct {
	LongRestHitDice string `json:"longRestHitDice"`
}

type RulesetCapabilities struct {
	WeaponMastery *bool `json:"weaponMastery"`
}

type BuilderPolicy struct {
	AbilityScoreAdvancement AbilityScoreAdvancement `json:"abilityScoreAdvancement"`
	BackgroundAbilityGrant  json.RawMessage         `json:"backgroundAbilityGrant"`
	SpeciesAbilityGrant     json.RawMessage         `json:"speciesAbilityGrant"`
}

type AbilityScoreAdvancement struct {
	BaseLevels          []int               `json:"baseLevels"`
	Budget              int                 `json:"budget"`
	PerAbilityMax       int                 `json:"perAbilityMax"`
	FeatCategories      []string            `json:"featCategories"`
	CategoriesByLevel   map[string][]string `json:"categoriesByLevel"`
	CategoryAbilityCaps map[string]int      `json:"categoryAbilityCaps"`
}

type OriginAbilityGrant struct {
	Budget        int `json:"budget"`
	PerAbilityMax int `json:"perAbilityMax"`
}

type PactMagicResult struct {
	Slots int `json:"slots"`
	Level int `json:"level"`
}

func DecodeRuleset(record json.RawMessage) (Ruleset, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(record, &raw); err != nil || raw == nil {
		return Ruleset{}, errors.New("ruleset must be an object")
	}
	if _, extends := raw["extends"]; extends {
		return Ruleset{}, errors.New("ruleset inheritance is not supported")
	}
	var ruleset Ruleset
	if err := json.Unmarshal(record, &ruleset); err != nil {
		return Ruleset{}, fmt.Errorf("decode ruleset: %w", err)
	}
	if err := ruleset.Validate(); err != nil {
		return Ruleset{}, err
	}
	return ruleset, nil
}

func (ruleset Ruleset) StableID() string {
	if ruleset.RulesetID != "" {
		return ruleset.RulesetID
	}
	return ruleset.ID
}

func (ruleset Ruleset) Validate() error {
	if !validStableID(ruleset.StableID()) || ruleset.RulesetVersion < 1 ||
		len(ruleset.Edition) > 80 || strings.TrimSpace(ruleset.Edition) != ruleset.Edition {
		return errors.New("ruleset identity is invalid")
	}
	constants := ruleset.Constants
	if constants.AbilityCap < 1 || constants.AbilityCapHard < constants.AbilityCap ||
		constants.AttunementLimit < 0 || constants.ScrollCopyGPPerLevel < 0 {
		return errors.New("ruleset numeric constants are invalid")
	}
	if err := validatePointBuy(constants.PointBuy); err != nil {
		return err
	}
	for level := 1; level <= 20; level++ {
		row := constants.MulticlassSlots[strconv.Itoa(level)]
		if len(row) == 0 {
			return fmt.Errorf("multiclass slot row %d is missing", level)
		}
		for _, slots := range row {
			if slots < 0 {
				return fmt.Errorf("multiclass slot row %d is invalid", level)
			}
		}
	}
	if !validRounding(constants.CasterFractions.Half) || !validRounding(constants.CasterFractions.Third) {
		return errors.New("caster fractions are invalid")
	}
	if err := validatePactMagic(constants.PactMagic); err != nil {
		return err
	}
	if constants.Spellbook.BaseKnown < 0 || constants.Spellbook.KnownPerLevel < 0 ||
		(constants.Rest.LongRestHitDice != "all" && constants.Rest.LongRestHitDice != "half") {
		return errors.New("spellbook or rest policy is invalid")
	}
	if ruleset.Capabilities.WeaponMastery == nil {
		return errors.New("weapon mastery capability is required")
	}
	return validateBuilder(ruleset.Builder, constants)
}

func ScrollCopyCost(level float64, ruleset Ruleset) float64 {
	if level < 1 {
		level = 1
	}
	return ruleset.Constants.ScrollCopyGPPerLevel * level
}

func PointBuyCost(score int, ruleset Ruleset) int {
	policy := ruleset.Constants.PointBuy
	if score < policy.Min {
		score = policy.Min
	}
	if score > policy.Max {
		score = policy.Max
	}
	return policy.Cost[strconv.Itoa(score)]
}

func PointsSpent(scores map[string]int, ruleset Ruleset) int {
	total := 0
	for _, ability := range Abilities {
		total += PointBuyCost(scores[ability], ruleset)
	}
	return total
}

func MulticlassSlots(casterLevel int, ruleset Ruleset) []int {
	if casterLevel < 0 {
		casterLevel = 0
	}
	if casterLevel > 20 {
		casterLevel = 20
	}
	return append([]int(nil), ruleset.Constants.MulticlassSlots[strconv.Itoa(casterLevel)]...)
}

func PactMagic(level int, ruleset Ruleset) *PactMagicResult {
	if level < 1 {
		return nil
	}
	slots := 0
	for _, tier := range ruleset.Constants.PactMagic.Tiers {
		if level >= tier.Level {
			slots = tier.Slots
		}
	}
	if slots < 1 {
		return nil
	}
	slotLevel := (level + 1) / 2
	if slotLevel > ruleset.Constants.PactMagic.SlotLevelCap {
		slotLevel = ruleset.Constants.PactMagic.SlotLevelCap
	}
	return &PactMagicResult{Slots: slots, Level: slotLevel}
}

func FeatASIFrom(grant map[string]any) []string {
	raw, ok := grant["from"].([]any)
	if !ok {
		return []string{}
	}
	result := make([]string, 0, len(raw))
	for _, candidate := range raw {
		value, ok := candidate.(string)
		if !ok {
			return []string{}
		}
		if value == "ANY" {
			return append([]string(nil), Abilities[:]...)
		}
		result = append(result, value)
	}
	return result
}

func FeatAbilityCap(feat map[string]any, ruleset Ruleset) *int {
	if explicit, ok := nestedNumber(feat, "grants", "abilityScoreIncrease", "cap"); ok {
		cap := min(ruleset.Constants.AbilityCapHard, int(explicit))
		return &cap
	}
	category, _ := feat["category"].(string)
	value, exists := ruleset.Builder.AbilityScoreAdvancement.CategoryAbilityCaps[category]
	if !exists {
		return nil
	}
	cap := min(ruleset.Constants.AbilityCapHard, value)
	return &cap
}

func validatePointBuy(policy PointBuyPolicy) error {
	if policy.Budget < 0 || policy.Min < 0 || policy.Max < policy.Min || policy.Cost == nil {
		return errors.New("point-buy policy is invalid")
	}
	for score := policy.Min; score <= policy.Max; score++ {
		cost, exists := policy.Cost[strconv.Itoa(score)]
		if !exists || cost < 0 {
			return fmt.Errorf("point-buy cost for %d is invalid", score)
		}
	}
	return nil
}

func validatePactMagic(policy PactMagicPolicy) error {
	if len(policy.Tiers) == 0 || policy.SlotLevelCap < 1 {
		return errors.New("pact magic policy is invalid")
	}
	previous := 0
	for _, tier := range policy.Tiers {
		if tier.Level <= previous || tier.Slots < 0 {
			return errors.New("pact magic tiers are invalid")
		}
		previous = tier.Level
	}
	return nil
}

func validateBuilder(builder BuilderPolicy, constants RulesetConstants) error {
	advancement := builder.AbilityScoreAdvancement
	seen := make(map[int]struct{}, len(advancement.BaseLevels))
	for _, level := range advancement.BaseLevels {
		if level < 1 || level > 20 {
			return errors.New("ability advancement level is invalid")
		}
		if _, duplicate := seen[level]; duplicate {
			return errors.New("ability advancement levels contain a duplicate")
		}
		seen[level] = struct{}{}
	}
	if advancement.Budget < 1 || advancement.PerAbilityMax < 1 ||
		advancement.PerAbilityMax > advancement.Budget {
		return errors.New("ability advancement budget is invalid")
	}
	for _, category := range advancement.FeatCategories {
		if !validCategory(category) {
			return errors.New("feat category is invalid")
		}
	}
	for rawLevel, categories := range advancement.CategoriesByLevel {
		level, err := strconv.Atoi(rawLevel)
		if err != nil || level < 1 || level > 20 {
			return errors.New("level-specific feat category is invalid")
		}
		for _, category := range categories {
			if !validCategory(category) {
				return errors.New("level-specific feat category is invalid")
			}
		}
	}
	for category, cap := range advancement.CategoryAbilityCaps {
		if !validCategory(category) || cap < constants.AbilityCap || cap > constants.AbilityCapHard {
			return errors.New("feat category ability cap is invalid")
		}
	}
	if err := validateOriginGrant(builder.BackgroundAbilityGrant); err != nil {
		return fmt.Errorf("background ability grant: %w", err)
	}
	if err := validateOriginGrant(builder.SpeciesAbilityGrant); err != nil {
		return fmt.Errorf("species ability grant: %w", err)
	}
	return nil
}

func validateOriginGrant(body json.RawMessage) error {
	if string(body) == "false" {
		return nil
	}
	var grant OriginAbilityGrant
	if len(body) == 0 || json.Unmarshal(body, &grant) != nil || grant.Budget < 1 ||
		grant.PerAbilityMax < 1 || grant.PerAbilityMax > grant.Budget {
		return errors.New("policy must be false or a valid budget")
	}
	return nil
}

func validRounding(value string) bool { return value == "up" || value == "down" }

func validStableID(value string) bool {
	if len(value) < 1 || len(value) > 80 {
		return false
	}
	for index, character := range value {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' ||
			index > 0 && (character == '.' || character == '_' || character == '-') {
			continue
		}
		return false
	}
	return true
}

func validCategory(value string) bool {
	if len(value) < 1 || len(value) > 80 {
		return false
	}
	for index, character := range value {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' || index > 0 && (character == '.' || character == '_' || character == '-') {
			continue
		}
		return false
	}
	return true
}

func nestedNumber(value map[string]any, path ...string) (float64, bool) {
	var current any = value
	for _, key := range path {
		object, ok := current.(map[string]any)
		if !ok {
			return 0, false
		}
		current, ok = object[key]
		if !ok {
			return 0, false
		}
	}
	number, ok := current.(float64)
	return number, ok
}
