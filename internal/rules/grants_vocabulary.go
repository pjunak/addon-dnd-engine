package rules

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/pjunak/addon-dnd-engine/character"
)

// Grants is the grants vocabulary a class, subclass, feature, species,
// lineage, background, feat or complete magic item may declare. Rules read
// grants field by field; this type checks them and generates
// contracts/grants.schema.json, which providers enforce at install.
type Grants struct {
	Languages               []string              `json:"languages,omitempty"`
	SavingThrows            []string              `json:"savingThrows,omitempty"`
	Armor                   []string              `json:"armor,omitempty"`
	Weapons                 []string              `json:"weapons,omitempty"`
	Tools                   []string              `json:"tools,omitempty"`
	Skills                  []string              `json:"skills,omitempty"`
	Expertise               []string              `json:"expertise,omitempty"`
	Resistances             []string              `json:"resistances,omitempty"`
	DamageImmunities        []string              `json:"damageImmunities,omitempty"`
	ConditionImmunities     []string              `json:"conditionImmunities,omitempty"`
	Proficiencies           *ProficiencyGrants    `json:"proficiencies,omitempty"`
	AbilityScoreIncrease    *AbilityScoreIncrease `json:"abilityScoreIncrease,omitempty"`
	AbilityScoreBonus       *AbilityScoreBonus    `json:"abilityScoreBonus,omitempty"`
	Choices                 []GrantChoice         `json:"choices,omitempty"`
	ChoicePackages          []ChoicePackage       `json:"choicePackages,omitempty"`
	ChoiceOverrides         []ChoiceOverride      `json:"choiceOverrides,omitempty"`
	Resources               []ResourceGrant       `json:"resources,omitempty"`
	Spells                  []SpellGrantDecl      `json:"spells,omitempty"`
	SpellList               []string              `json:"spellList,omitempty"`
	SpellLists              []string              `json:"spellLists,omitempty"`
	PrepareSpellListOf      string                `json:"prepareSpellListOf,omitempty"`
	SpellSlot               *SpellSlotGrant       `json:"spellSlot,omitempty"`
	SpellbookBonus          []SpellbookStep       `json:"spellbookBonus,omitempty"`
	CastingAbility          *CastingAbilityGrant  `json:"castingAbility,omitempty"`
	Advantages              []AdvantageGrant      `json:"advantages,omitempty"`
	Activations             []ActivationGrant     `json:"activations,omitempty"`
	AttackModifiers         []AttackModifierGrant `json:"attackModifiers,omitempty"`
	Attacks                 []AttackGrant         `json:"attacks,omitempty"`
	SaveDCs                 []SaveDCGrant         `json:"saveDCs,omitempty"`
	SkillBonuses            []CheckBonusGrant     `json:"skillBonuses,omitempty"`
	SaveBonuses             []CheckBonusGrant     `json:"saveBonuses,omitempty"`
	InitiativeBonuses       []CheckBonusGrant     `json:"initiativeBonuses,omitempty"`
	SpeedBonus              int                   `json:"speedBonus,omitempty"`
	SpeedBonuses            []SpeedBonusGrant     `json:"speedBonuses,omitempty"`
	Speeds                  map[string]SpeedValue `json:"speeds,omitempty"`
	Senses                  map[string]int        `json:"senses,omitempty"`
	SenseBonuses            map[string]int        `json:"senseBonuses,omitempty"`
	HPBonus                 int                   `json:"hpBonus,omitempty"`
	HPPerLevel              int                   `json:"hpPerLevel,omitempty"`
	HPPerClassLevel         int                   `json:"hpPerClassLevel,omitempty"`
	ACBonus                 int                   `json:"acBonus,omitempty"`
	ACBonuses               []ACBonusGrant        `json:"acBonuses,omitempty"`
	ACFormulas              []ACFormulaGrant      `json:"acFormulas,omitempty"`
	CriticalRange           int                   `json:"criticalRange,omitempty"`
	AttunementLimit         int                   `json:"attunementLimit,omitempty"`
	WeaponMasterySlots      int                   `json:"weaponMasterySlots,omitempty"`
	Conditions              []GrantedCondition    `json:"conditions,omitempty"`
	Bonuses                 map[string]int        `json:"bonuses,omitempty"`
	UnquantifiableModifiers []string              `json:"unquantifiable_modifiers,omitempty"`
}

// ProficiencyGrants groups fixed proficiencies under one key.
type ProficiencyGrants struct {
	Armor     []string `json:"armor,omitempty"`
	Weapons   []string `json:"weapons,omitempty"`
	Tools     []string `json:"tools,omitempty"`
	Skills    []string `json:"skills,omitempty"`
	Languages []string `json:"languages,omitempty"`
}

type AbilityScoreIncrease struct {
	Amount               int      `json:"amount,omitempty"`
	Choose               int      `json:"choose,omitempty"`
	Cap                  int      `json:"cap,omitempty"`
	From                 []string `json:"from,omitempty"`
	FromCastingAbilityOf string   `json:"fromCastingAbilityOf,omitempty"`
}

type AbilityScoreBonus struct {
	Assign map[string]int `json:"assign"`
	Cap    int            `json:"cap,omitempty"`
}

// GrantChoice is a choice a source offers (grants.choices[]).
type GrantChoice struct {
	ID                string                  `json:"id"`
	Type              string                  `json:"type,omitempty"`
	Count             int                     `json:"count,omitempty"`
	CountByLevel      map[string]int          `json:"countByLevel,omitempty"`
	From              []string                `json:"from,omitempty"`
	FromByLevel       map[string][]string     `json:"fromByLevel,omitempty"`
	FromCategory      string                  `json:"fromCategory,omitempty"`
	Filter            *RecordFilter           `json:"filter,omitempty"`
	FilterByLevel     map[string]RecordFilter `json:"filterByLevel,omitempty"`
	OptionKind        string                  `json:"optionKind,omitempty"`
	Category          string                  `json:"category,omitempty"`
	Prompt            string                  `json:"prompt,omitempty"`
	Default           string                  `json:"default,omitempty"`
	ChangeOn          string                  `json:"changeOn,omitempty"`
	SwappableOn       string                  `json:"swappableOn,omitempty"`
	LevelReplacements int                     `json:"levelReplacements,omitempty"`
	Source            string                  `json:"source,omitempty"`
}

// RecordFilter selects records of a choice's option kind.
type RecordFilter struct {
	CreatureType string   `json:"creatureType,omitempty"`
	MaxCR        *float64 `json:"maxCr,omitempty"`
	Fly          *bool    `json:"fly,omitempty"`
	Rarity       []string `json:"rarity,omitempty"`
	ItemTypes    []string `json:"itemTypes,omitempty"`
	NotItemTypes []string `json:"notItemTypes,omitempty"`
}

// ChoicePackage bundles grant fields per option of an enumerated choice.
type ChoicePackage struct {
	ChoiceID string            `json:"choiceId"`
	Options  map[string]Grants `json:"options"`
}

// ChoiceOverride replaces option limits of a class choice.
type ChoiceOverride struct {
	ChoiceID      string                  `json:"choiceId"`
	From          []string                `json:"from,omitempty"`
	FromByLevel   map[string][]string     `json:"fromByLevel,omitempty"`
	Filter        *RecordFilter           `json:"filter,omitempty"`
	FilterByLevel map[string]RecordFilter `json:"filterByLevel,omitempty"`
	Count         int                     `json:"count,omitempty"`
	CountByLevel  map[string]int          `json:"countByLevel,omitempty"`
}

type ResourceGrant struct {
	Key              string         `json:"key"`
	Name             string         `json:"name,omitempty"`
	Fixed            *int           `json:"fixed,omitempty"`
	PerLevel         *float64       `json:"perLevel,omitempty"`
	ProficiencyBonus bool           `json:"proficiencyBonus,omitempty"`
	AbilityMod       string         `json:"abilityMod,omitempty"`
	Multiplier       int            `json:"multiplier,omitempty"`
	Add              int            `json:"add,omitempty"`
	Min              *int           `json:"min,omitempty"`
	MinLevel         int            `json:"minLevel,omitempty"`
	Progression      []LevelMaximum `json:"progression,omitempty"`
	Recharge         *RechargeDecl  `json:"recharge,omitempty"`
}

type LevelMaximum struct {
	Level int `json:"level"`
	Max   int `json:"max"`
}

// SpellGrantDecl grants fixed spells (ids) or one spell choice (id + choose).
type SpellGrantDecl struct {
	ID                 string            `json:"id,omitempty"`
	IDs                []string          `json:"ids,omitempty"`
	Choose             int               `json:"choose,omitempty"`
	Level              int               `json:"level,omitempty"`
	AtLevel            int               `json:"atLevel,omitempty"`
	SpellLevel         *int              `json:"spellLevel,omitempty"`
	MaxSpellLevel      *SpellLevelLimit  `json:"maxSpellLevel,omitempty"`
	From               *SpellFromDecl    `json:"from,omitempty"`
	Default            string            `json:"default,omitempty"`
	ClassID            string            `json:"classId,omitempty"`
	AlwaysPrepared     bool              `json:"alwaysPrepared,omitempty"`
	AtWill             bool              `json:"atWill,omitempty"`
	RitualOnly         bool              `json:"ritualOnly,omitempty"`
	Components         *bool             `json:"components,omitempty"`
	Free               string            `json:"free,omitempty"`
	Resource           *ResourceRef      `json:"resource,omitempty"`
	LevelReplacements  int               `json:"levelReplacements,omitempty"`
	CastAtLevelByLevel []CastAtLevelStep `json:"castAtLevelByLevel,omitempty"`
}

type SpellFromDecl struct {
	Class       []string    `json:"class,omitempty"`
	School      []string    `json:"school,omitempty"`
	IDs         []string    `json:"ids,omitempty"`
	CastingTime *TextOrList `json:"castingTime,omitempty"`
	Ritual      bool        `json:"ritual,omitempty"`
}

type CastAtLevelStep struct {
	Level       int `json:"level"`
	CastAtLevel int `json:"castAtLevel"`
}

type SpellSlotGrant struct {
	Count       int            `json:"count,omitempty"`
	Level       SpellSlotLevel `json:"level"`
	Recharge    *RechargeDecl  `json:"recharge,omitempty"`
	Restriction string         `json:"restriction,omitempty"`
}

type SpellSlotLevel struct {
	Divisor int    `json:"divisor,omitempty"`
	Min     int    `json:"min,omitempty"`
	Max     int    `json:"max,omitempty"`
	Round   string `json:"round,omitempty"`
}

type SpellbookStep struct {
	Level  int    `json:"level"`
	Count  int    `json:"count"`
	School string `json:"school,omitempty"`
}

type CastingAbilityGrant struct {
	Fixed                    string   `json:"fixed,omitempty"`
	ID                       string   `json:"id,omitempty"`
	Choose                   []string `json:"choose,omitempty"`
	FromAbilityScoreIncrease bool     `json:"fromAbilityScoreIncrease,omitempty"`
}

type AdvantageGrant struct {
	On        string          `json:"on"`
	Mode      string          `json:"mode,omitempty"`
	Abilities []string        `json:"abilities,omitempty"`
	Skills    []string        `json:"skills,omitempty"`
	When      string          `json:"when,omitempty"`
	Requires  *ArmorCondition `json:"requires,omitempty"`
}

// ArmorCondition is a worn-armor requirement. Unsupported conditions keep
// their bonus inactive.
type ArmorCondition struct {
	NoArmor       bool     `json:"noArmor,omitempty"`
	NoShield      bool     `json:"noShield,omitempty"`
	ArmorTypes    []string `json:"armorTypes,omitempty"`
	NotArmorTypes []string `json:"notArmorTypes,omitempty"`
}

type ActivationGrant struct {
	ID             string               `json:"id,omitempty"`
	Name           string               `json:"name,omitempty"`
	Condition      string               `json:"condition,omitempty"`
	MinLevel       int                  `json:"minLevel,omitempty"`
	Resource       *ResourceRef         `json:"resource,omitempty"`
	ExclusiveGroup string               `json:"exclusiveGroup,omitempty"`
	Extends        *TextOrList          `json:"extends,omitempty"`
	While          *TextOrList          `json:"while,omitempty"`
	Restrictions   *ArmorCondition      `json:"restrictions,omitempty"`
	Modifiers      []ActivationModifier `json:"modifiers,omitempty"`
}

// ActivationModifier is one effect of an enabled activation; Target selects
// which other fields apply.
type ActivationModifier struct {
	Target             string          `json:"target"`
	Key                string          `json:"key,omitempty"`
	Value              *TextOrNumber   `json:"value,omitempty"`
	Add                int             `json:"add,omitempty"`
	AddAbility         string          `json:"addAbility,omitempty"`
	Multiply           int             `json:"multiply,omitempty"`
	Ability            string          `json:"ability,omitempty"`
	Proficient         *bool           `json:"proficient,omitempty"`
	On                 string          `json:"on,omitempty"`
	Mode               string          `json:"mode,omitempty"`
	Abilities          []string        `json:"abilities,omitempty"`
	Skills             []string        `json:"skills,omitempty"`
	When               string          `json:"when,omitempty"`
	Requires           *ArmorCondition `json:"requires,omitempty"`
	Match              []AttackMatch   `json:"match,omitempty"`
	AttackBonus        int             `json:"attackBonus,omitempty"`
	AttackBonusAbility string          `json:"attackBonusAbility,omitempty"`
	Min                *int            `json:"min,omitempty"`
	DamageBonus        int             `json:"damageBonus,omitempty"`
	DamageBonusColumn  string          `json:"damageBonusColumn,omitempty"`
	DamageDie          string          `json:"damageDie,omitempty"`
	DamageDieColumn    string          `json:"damageDieColumn,omitempty"`
}

type AttackModifierGrant struct {
	Ability            string          `json:"ability,omitempty"`
	AttackBonus        int             `json:"attackBonus,omitempty"`
	AttackBonusAbility string          `json:"attackBonusAbility,omitempty"`
	Min                *int            `json:"min,omitempty"`
	DamageBonus        int             `json:"damageBonus,omitempty"`
	DamageBonusColumn  string          `json:"damageBonusColumn,omitempty"`
	DamageDie          string          `json:"damageDie,omitempty"`
	DamageDieColumn    string          `json:"damageDieColumn,omitempty"`
	Match              []AttackMatch   `json:"match,omitempty"`
	Requires           *ArmorCondition `json:"requires,omitempty"`
}

type AttackMatch struct {
	Unarmed       *bool  `json:"unarmed,omitempty"`
	Range         string `json:"range,omitempty"`
	Category      string `json:"category,omitempty"`
	Property      string `json:"property,omitempty"`
	AttackAbility string `json:"attackAbility,omitempty"`
	Attack        string `json:"attack,omitempty"`
}

// AttackGrant is a weapon a feature creates.
type AttackGrant struct {
	ID              string        `json:"id"`
	Name            string        `json:"name"`
	Category        string        `json:"category,omitempty"`
	Range           string        `json:"range,omitempty"`
	Damage          string        `json:"damage"`
	DamageType      string        `json:"damageType,omitempty"`
	VersatileDamage string        `json:"versatileDamage,omitempty"`
	Properties      []string      `json:"properties,omitempty"`
	Ability         string        `json:"ability,omitempty"`
	Proficient      *bool         `json:"proficient,omitempty"`
	Mastery         string        `json:"mastery,omitempty"`
	MasteryActive   bool          `json:"masteryActive,omitempty"`
	RangeNormalLong *DistancePair `json:"rangeNormalLong,omitempty"`
	ThrownRange     *DistancePair `json:"thrownRange,omitempty"`
}

type DistancePair struct {
	Normal int `json:"normal"`
	Long   int `json:"long"`
}

type SaveDCGrant struct {
	Name      string   `json:"name,omitempty"`
	Abilities []string `json:"abilities"`
}

type CheckBonusGrant struct {
	Add           *TextOrNumber `json:"add,omitempty"`
	AddAbility    string        `json:"addAbility,omitempty"`
	Min           *int          `json:"min,omitempty"`
	Skills        []string      `json:"skills,omitempty"`
	Abilities     []string      `json:"abilities,omitempty"`
	NotProficient bool          `json:"notProficient,omitempty"`
}

type SpeedBonusGrant struct {
	Add      *int            `json:"add,omitempty"`
	Column   string          `json:"column,omitempty"`
	Requires *ArmorCondition `json:"requires,omitempty"`
}

type ACBonusGrant struct {
	Add      int             `json:"add"`
	Requires *ArmorCondition `json:"requires,omitempty"`
}

type ACFormulaGrant struct {
	ID           string          `json:"id"`
	Name         string          `json:"name,omitempty"`
	Base         int             `json:"base"`
	AddAbilities []string        `json:"addAbilities,omitempty"`
	Requires     *ArmorCondition `json:"requires,omitempty"`
}

// GrantedCondition is a condition an active item grants.
type GrantedCondition struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Summary string `json:"summary,omitempty"`
}

// validateGrants checks a record's grants against the vocabulary. It returns
// nil for a record without grants.
func validateGrants(raw any) error {
	if raw == nil {
		return nil
	}
	body, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	var grants Grants
	return strictDecode(body, &grants)
}

// strictDecode decodes JSON that may hold only the target's fields.
func strictDecode(body []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

// reportRulesData warns about every evidence record whose grants, or whose
// lineages' grants, break the vocabulary. The rules still read the fields
// that are well formed.
func reportRulesData(result *character.Result) {
	for _, entry := range result.Evidence {
		facts := factsOf(entry)
		problems := []error{}
		if err := validateGrants(facts["grants"]); err != nil {
			problems = append(problems, err)
		}
		for _, lineage := range objects(facts["lineages"]) {
			if err := validateGrants(lineage["grants"]); err != nil {
				problems = append(problems, fmt.Errorf("lineage %s: %w", text(lineage["id"]), err))
			}
		}
		for _, problem := range problems {
			ref := entry.Reference
			addCharacterIssue(result, "rules-data:"+ref.Kind+":"+ref.ID, "rules", fmt.Sprintf("%s's grants do not follow the rules vocabulary (%v); the rules data needs correction.", entry.Name, problem), character.SeverityWarning, &ref)
		}
	}
}
