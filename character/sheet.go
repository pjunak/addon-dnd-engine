package character

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// Sheet is the evaluated character: calculated statistics, absent while the
// build needs choices, plus play state that is always present.
type Sheet struct {
	Status SheetStatus `json:"status"`
	*Calculation
	Inspiration      bool                      `json:"inspiration"`
	Hands            SheetHands                `json:"hands"`
	QuickUse         []QuickUsePin             `json:"quickUse"`
	ActiveItems      []ActiveItem              `json:"activeItems"`
	Storage          SheetStorage              `json:"storage"`
	BodyPlacement    []PlacementRow            `json:"bodyPlacement"`
	Conditions       []SheetCondition          `json:"conditions"`
	ConditionEffects ConditionEffects          `json:"conditionEffects"`
	Equipment        map[string]EquipmentFacts `json:"equipment"`
	Carrying         *Carrying                 `json:"carrying,omitempty"`
	Concentration    *SheetConcentration       `json:"concentration,omitempty"`
}

// Calculation holds the statistics the rules calculate from the build.
type Calculation struct {
	Abilities              map[string]AbilityScore `json:"abilities"`
	Saves                  map[string]SaveRow      `json:"saves"`
	Skills                 map[string]SkillRow     `json:"skills"`
	Derived                Derived                 `json:"derived"`
	Proficiencies          Proficiencies           `json:"proficiencies"`
	TotalLevel             int                     `json:"totalLevel"`
	Class                  *RecordIdentity         `json:"class,omitempty"`
	Species                *RecordIdentity         `json:"species,omitempty"`
	Classes                []SheetClass            `json:"classes"`
	ClassValues            []ClassValue            `json:"classValues"`
	Features               []SheetFeature          `json:"features"`
	Feats                  []SheetFeat             `json:"feats"`
	FeatIdentities         []FeatIdentity          `json:"featIdentities"`
	Selections             []Selection             `json:"selections,omitempty"`
	HP                     HitPoints               `json:"hp"`
	AC                     ArmorClass              `json:"ac"`
	ArmorRestrictions      *ArmorRestrictions      `json:"armorRestrictions"`
	Speed                  int                     `json:"speed"`
	FlySpeed               int                     `json:"flySpeed"`
	SwimSpeed              int                     `json:"swimSpeed,omitempty"`
	ClimbSpeed             int                     `json:"climbSpeed,omitempty"`
	BurrowSpeed            int                     `json:"burrowSpeed,omitempty"`
	Movement               []MovementRow           `json:"movement,omitempty"`
	SpeedBonuses           []ConditionalBonus      `json:"speedBonuses"`
	InitiativeBonuses      []NamedValue            `json:"initiativeBonuses,omitempty"`
	Senses                 map[string]int          `json:"senses"`
	Passives               Passives                `json:"passives"`
	Languages              []string                `json:"languages"`
	Resistances            []string                `json:"resistances"`
	DamageImmunities       []string                `json:"damageImmunities"`
	ConditionImmunities    []string                `json:"conditionImmunities"`
	ConcentrationSaveBonus int                     `json:"concentrationSaveBonus"`
	Advantages             []RollEntry             `json:"advantages,omitempty"`
	RollMarks
	Activations   []Activation   `json:"activations"`
	SaveDCs       []SaveDC       `json:"saveDCs,omitempty"`
	Spellcasting  Spellcasting   `json:"spellcasting"`
	WeaponMastery WeaponMastery  `json:"weaponMastery"`
	Weapons       []Attack       `json:"weapons"`
	Attunement    Attunement     `json:"attunement"`
	Resources     []Resource     `json:"resources"`
	CurrencyWorth *CurrencyWorth `json:"currencyWorth,omitempty"`
}

// RollMarks are the net Advantage or Disadvantage on rolls outside the
// ability, save and skill rows.
type RollMarks struct {
	AttackAdvantage           bool `json:"attackAdvantage,omitempty"`
	AttackDisadvantage        bool `json:"attackDisadvantage,omitempty"`
	InitiativeAdvantage       bool `json:"initiativeAdvantage,omitempty"`
	InitiativeDisadvantage    bool `json:"initiativeDisadvantage,omitempty"`
	ConcentrationAdvantage    bool `json:"concentrationAdvantage,omitempty"`
	ConcentrationDisadvantage bool `json:"concentrationDisadvantage,omitempty"`
	DeathSaveAdvantage        bool `json:"deathSaveAdvantage,omitempty"`
	DeathSaveDisadvantage     bool `json:"deathSaveDisadvantage,omitempty"`
}

// Source is what a sheet entry comes from. Item sources name their inventory
// instance in ID and their record in Ref; feat acquisitions name the choice
// that acquired them.
type Source struct {
	Type        SourceType   `json:"type"`
	ID          string       `json:"id,omitempty"`
	Level       int          `json:"level,omitempty"`
	Name        string       `json:"name,omitempty"`
	Ref         *Reference   `json:"ref,omitempty"`
	Acquisition *Acquisition `json:"acquisition,omitempty"`
	ClassID     string       `json:"classId,omitempty"`
}

// Acquisition is one acquisition of a repeatable feat.
type Acquisition struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	ClassID string `json:"classId"`
	Level   int    `json:"level"`
}

type RecordIdentity struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
	Kind string `json:"kind"`
	Book string `json:"book,omitempty"`
}

type AbilityScore struct {
	Base              int  `json:"base"`
	Score             int  `json:"score"`
	Mod               int  `json:"mod"`
	Bonus             int  `json:"bonus"`
	Cap               int  `json:"cap,omitempty"`
	CheckAdvantage    bool `json:"checkAdvantage,omitempty"`
	CheckDisadvantage bool `json:"checkDisadvantage,omitempty"`
}

type SaveRow struct {
	Mod          int          `json:"mod"`
	Proficient   bool         `json:"proficient"`
	Total        int          `json:"total"`
	Bonuses      []NamedValue `json:"bonuses,omitempty"`
	Advantage    bool         `json:"advantage,omitempty"`
	Disadvantage bool         `json:"disadvantage,omitempty"`
}

type SkillRow struct {
	Ability      string       `json:"ability"`
	Mod          int          `json:"mod"`
	Proficient   bool         `json:"proficient"`
	Expertise    bool         `json:"expertise"`
	Total        int          `json:"total"`
	Bonuses      []NamedValue `json:"bonuses,omitempty"`
	Advantage    bool         `json:"advantage,omitempty"`
	Disadvantage bool         `json:"disadvantage,omitempty"`
}

type Derived struct {
	ArmorClass        int     `json:"armorClass"`
	HitDie            *string `json:"hitDie"`
	Initiative        int     `json:"initiative"`
	MaxHP             int     `json:"maxHp"`
	PassivePerception int     `json:"passivePerception"`
	ProficiencyBonus  int     `json:"proficiencyBonus"`
	Speed             int     `json:"speed"`
	Size              string  `json:"size,omitempty"`
}

// ProficiencyLevel is a skill's training.
type ProficiencyLevel string

const (
	TrainingNone       ProficiencyLevel = "none"
	TrainingProficient ProficiencyLevel = "proficient"
	TrainingExpertise  ProficiencyLevel = "expertise"
)

var proficiencyLevels = map[ProficiencyLevel]bool{TrainingNone: true, TrainingProficient: true, TrainingExpertise: true}

func (ProficiencyLevel) EnumValues() []string { return sortedValues(proficiencyLevels) }

type Proficiencies struct {
	Saves     map[string]bool             `json:"saves"`
	Skills    map[string]ProficiencyLevel `json:"skills"`
	Armor     []string                    `json:"armor"`
	Weapons   []string                    `json:"weapons"`
	Tools     []string                    `json:"tools"`
	Languages []string                    `json:"languages"`
}

type SheetClass struct {
	ClassID  string `json:"classId"`
	Name     string `json:"name"`
	Level    int    `json:"level"`
	Subclass string `json:"subclass"`
	HitDie   string `json:"hitDie"`
}

// ClassValue is a class or subclass table column at the class's level.
type ClassValue struct {
	ClassID    string     `json:"classId"`
	SubclassID string     `json:"subclassId,omitempty"`
	Key        string     `json:"key"`
	Name       string     `json:"name"`
	Value      TableValue `json:"value"`
}

// SheetFeature is an acquired feature. Kind "subclass" marks the chosen
// subclass in its class table slot; a row without a name is a printed label
// with no record.
type SheetFeature struct {
	ID       string `json:"id"`
	Name     string `json:"name,omitempty"`
	Kind     string `json:"kind,omitempty"`
	Category string `json:"category,omitempty"`
	Count    int    `json:"count,omitempty"`
	Source   Source `json:"source"`
}

type SheetFeat struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type FeatIdentity struct {
	ID       string `json:"id"`
	Category string `json:"category"`
}

// Selection is a class choice whose picks are not features of their own.
type Selection struct {
	ID        string           `json:"id"`
	Label     string           `json:"label"`
	LabelKey  string           `json:"labelKey"`
	LabelArgs []string         `json:"labelArgs"`
	Source    Source           `json:"source"`
	Values    []SelectionValue `json:"values"`
}

type SelectionValue struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Kind  string `json:"kind,omitempty"`
}

type HitPoints struct {
	Max          int          `json:"max"`
	Breakdown    HPBreakdown  `json:"breakdown"`
	FixedBonuses []NamedValue `json:"fixedBonuses,omitempty"`
	Levels       []HPLevel    `json:"levels"`
}

type HPBreakdown struct {
	Dice         int `json:"dice"`
	ConMod       int `json:"conMod"`
	ConTotal     int `json:"conTotal"`
	MiscPerLevel int `json:"miscPerLevel"`
	MiscTotal    int `json:"miscTotal"`
	Level        int `json:"level"`
	FixedBonus   int `json:"fixedBonus,omitempty"`
}

// HPLevel is one character level's hit-point gain.
type HPLevel struct {
	LevelID      string `json:"levelId"`
	ClassID      string `json:"classId"`
	Die          int    `json:"die"`
	Result       int    `json:"result"`
	Method       string `json:"method"`
	Constitution int    `json:"constitution"`
	Minimum      int    `json:"minimum"`
	Bonus        int    `json:"bonus"`
	Gained       int    `json:"gained"`
}

// NamedValue is a bonus a named source contributes.
type NamedValue struct {
	Name   string `json:"name"`
	Source Source `json:"source"`
	Value  int    `json:"value"`
}

// ConditionalBonus is a bonus that applies only under its requirement.
type ConditionalBonus struct {
	Name     string            `json:"name"`
	Source   Source            `json:"source"`
	Value    int               `json:"value"`
	Status   BonusStatus       `json:"status"`
	Requires *ArmorRequirement `json:"requires"`
}

// ArmorRequirement is the worn-armor condition of a bonus or activation.
type ArmorRequirement struct {
	ArmorTypes    []string `json:"armorTypes,omitempty"`
	NoArmor       bool     `json:"noArmor,omitempty"`
	NoShield      bool     `json:"noShield,omitempty"`
	NotArmorTypes []string `json:"notArmorTypes,omitempty"`
}

type ArmorClass struct {
	Value        int                `json:"value"`
	Base         string             `json:"base"`
	Shield       int                `json:"shield"`
	Candidates   []ACCandidate      `json:"candidates"`
	Restrictions *ArmorRestrictions `json:"restrictions"`
	SpeciesBonus int                `json:"speciesBonus,omitempty"`
	ActiveBonus  int                `json:"activeBonus,omitempty"`
	Bonuses      []ConditionalBonus `json:"bonuses,omitempty"`
}

type ACCandidate struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Value int    `json:"value"`
}

// ArmorRestrictions are the worn body armor's Strength and Stealth effects.
type ArmorRestrictions struct {
	ArmorID             string `json:"armorId"`
	ArmorName           string `json:"armorName"`
	StrengthRequirement int    `json:"strengthRequirement"`
	MeetsStrength       bool   `json:"meetsStrength"`
	SpeedPenalty        int    `json:"speedPenalty"`
	StealthDisadvantage bool   `json:"stealthDisadvantage"`
}

// MovementRow is one special movement grant; the sheet keeps the best per mode.
type MovementRow struct {
	Mode     MovementMode  `json:"mode"`
	Value    MovementValue `json:"value"`
	Multiply int           `json:"multiply,omitempty"`
	Name     string        `json:"name"`
	Source   Source        `json:"source"`
}

type Passives struct {
	Perception int `json:"perception"`
}

// RollEntry is Advantage, or Disadvantage when Mode says so, on a roll. One
// with When applies only in that situation and marks no row.
type RollEntry struct {
	On        RollKind `json:"on"`
	Mode      RollMode `json:"mode,omitempty"`
	Name      string   `json:"name"`
	Source    Source   `json:"source"`
	Abilities []string `json:"abilities,omitempty"`
	Skills    []string `json:"skills,omitempty"`
	When      string   `json:"when,omitempty"`
}

// Activation is a player-controlled feature state.
type Activation struct {
	Key            string              `json:"key"`
	ID             string              `json:"id"`
	Name           string              `json:"name"`
	Source         Source              `json:"source"`
	Resource       *ActivationResource `json:"resource"`
	ExclusiveGroup *string             `json:"exclusiveGroup"`
	Active         bool                `json:"active"`
	Available      bool                `json:"available"`
	Condition      string              `json:"condition,omitempty"`
}

// ActivationResource is the resource one use spends.
type ActivationResource struct {
	Key  string `json:"key"`
	Cost int    `json:"cost,omitempty"`
}

// SaveDC is a feature's own save DC.
type SaveDC struct {
	Name    string `json:"name"`
	Ability string `json:"ability"`
	Value   int    `json:"value"`
	Source  Source `json:"source"`
}

type Spellcasting struct {
	PerClass              []Caster               `json:"perClass"`
	CasterLevel           int                    `json:"casterLevel"`
	Slots                 []int                  `json:"slots"`
	Granted               []SpellGrant           `json:"granted"`
	PendingChoices        []SpellChoice          `json:"pendingChoices"`
	CastingAbilityChoices []CastingAbilityChoice `json:"castingAbilityChoices"`
}

// Caster is one spellcasting class.
type Caster struct {
	ClassID               string                 `json:"classId"`
	Level                 int                    `json:"level"`
	Ability               string                 `json:"ability"`
	Type                  string                 `json:"type"`
	Prepares              string                 `json:"prepares"`
	Ritual                bool                   `json:"ritual"`
	SpellListClassID      string                 `json:"spellListClassId"`
	SaveDC                int                    `json:"saveDC"`
	SpellAttack           int                    `json:"spellAttack"`
	PreparedLimit         int                    `json:"preparedLimit"`
	PreparedCount         int                    `json:"preparedCount"`
	CantripsKnown         int                    `json:"cantripsKnown"`
	SpellbookKnown        int                    `json:"spellbookKnown"`
	SpellbookCapacity     int                    `json:"spellbookCapacity,omitempty"`
	SpellbookBonus        int                    `json:"spellbookBonus,omitempty"`
	SpellbookBonusSources []SpellbookBonusSource `json:"spellbookBonusSources,omitempty"`
	MaxSpellLevel         int                    `json:"maxSpellLevel"`
	Pact                  *PactSlots             `json:"pact"`
	ExpandedSpellIDs      []string               `json:"expandedSpellIds"`
	SpellLists            []string               `json:"spellLists,omitempty"`
	Preparation           *Preparation           `json:"preparation,omitempty"`
}

type PactSlots struct {
	Slots int `json:"slots"`
	Level int `json:"level"`
}

type SpellbookBonusSource struct {
	Source Source `json:"source"`
	Count  int    `json:"count"`
	School string `json:"school,omitempty"`
}

// Preparation says when the prepared list may change: after a long rest
// (Count is "all" or a number) or on gaining a class level.
type Preparation struct {
	On    string           `json:"on"`
	Count PreparationCount `json:"count"`
}

// SpellGrant is a spell a source grants.
type SpellGrant struct {
	Ref              string   `json:"ref"`
	Name             string   `json:"name"`
	Level            *int     `json:"level"`
	School           string   `json:"school"`
	Source           Source   `json:"source"`
	AlwaysPrepared   bool     `json:"alwaysPrepared"`
	Free             *string  `json:"free"`
	CastingAbility   *string  `json:"castingAbility"`
	CastAtLevel      *float64 `json:"castAtLevel"`
	ResourceKey      string   `json:"resourceKey,omitempty"`
	AtWill           bool     `json:"atWill,omitempty"`
	RitualOnly       bool     `json:"ritualOnly,omitempty"`
	CastResource     string   `json:"castResource,omitempty"`
	CastResourceCost int      `json:"castResourceCost,omitempty"`
}

// SpellChoice is a granted spell choice and what has been picked.
type SpellChoice struct {
	Key               string      `json:"key"`
	Source            Source      `json:"source"`
	Choose            int         `json:"choose"`
	SpellLevel        *int        `json:"spellLevel"`
	MaxSpellLevel     *int        `json:"maxSpellLevel"`
	From              SpellFilter `json:"from"`
	Default           *string     `json:"default"`
	AlwaysPrepared    bool        `json:"alwaysPrepared"`
	Picked            []string    `json:"picked"`
	Free              string      `json:"free,omitempty"`
	LevelReplacements int         `json:"levelReplacements,omitempty"`
	AcquiredAt        int         `json:"acquiredAt,omitempty"`
}

// SpellFilter limits a spell choice: by class list, school or ID, then by
// casting time and the ritual tag.
type SpellFilter struct {
	Class       []string `json:"class,omitempty"`
	School      []string `json:"school,omitempty"`
	IDs         []string `json:"ids,omitempty"`
	CastingTime []string `json:"castingTime,omitempty"`
	Ritual      bool     `json:"ritual,omitempty"`
}

type CastingAbilityChoice struct {
	Key      string   `json:"key"`
	Source   Source   `json:"source"`
	Options  []string `json:"options"`
	Selected *string  `json:"selected"`
}

type WeaponMastery struct {
	Slots  int      `json:"slots"`
	Chosen []string `json:"chosen"`
}

// Attack is one computed attack: an equipped weapon, the unarmed strike or a
// feature's attack.
type Attack struct {
	Ref             string          `json:"ref,omitempty"`
	Name            string          `json:"name"`
	AttackBonus     int             `json:"attackBonus"`
	Ability         string          `json:"ability"`
	Damage          string          `json:"damage"`
	VersatileDamage *string         `json:"versatileDamage"`
	DamageType      string          `json:"damageType"`
	Properties      []string        `json:"properties"`
	Mastery         string          `json:"mastery"`
	MasteryActive   bool            `json:"masteryActive"`
	Proficient      bool            `json:"proficient"`
	Unarmed         bool            `json:"unarmed,omitempty"`
	FeatureAttack   string          `json:"featureAttack,omitempty"`
	Source          *Source         `json:"source,omitempty"`
	ItemID          string          `json:"itemId,omitempty"`
	Grip            Grip            `json:"grip,omitempty"`
	CriticalRange   int             `json:"criticalRange,omitempty"`
	Reach           int             `json:"reach,omitempty"`
	Range           *WeaponRange    `json:"range,omitempty"`
	Modifiers       []AttackApplied `json:"modifiers,omitempty"`
}

type WeaponRange struct {
	Normal int  `json:"normal"`
	Long   int  `json:"long"`
	Thrown bool `json:"thrown,omitempty"`
}

// AttackApplied is an attack modifier that changed this attack.
type AttackApplied struct {
	Name        string `json:"name"`
	Source      Source `json:"source"`
	AttackBonus int    `json:"attackBonus,omitempty"`
	DamageBonus int    `json:"damageBonus,omitempty"`
	DamageDie   string `json:"damageDie,omitempty"`
	Ability     string `json:"ability,omitempty"`
}

type Attunement struct {
	Count int  `json:"count"`
	Limit int  `json:"limit"`
	Over  bool `json:"over"`
}

// Resource is a tracked use pool with its spent and remaining uses.
type Resource struct {
	Key         string       `json:"key"`
	Name        string       `json:"name"`
	Max         int          `json:"max"`
	Kind        ResourceKind `json:"kind"`
	Level       int          `json:"level,omitempty"`
	Die         string       `json:"die,omitempty"`
	Restriction string       `json:"restriction,omitempty"`
	Recharge    []Recharge   `json:"recharge"`
	Source      Source       `json:"source"`
	Active      *bool        `json:"active,omitempty"`
	Spent       int          `json:"spent"`
	Remaining   int          `json:"remaining"`
}

type Recharge struct {
	On     RechargeOn     `json:"on"`
	Amount RechargeAmount `json:"amount"`
}

type CurrencyWorth struct {
	Coin  string  `json:"coin"`
	Value float64 `json:"value"`
}

type SheetHands struct {
	Main           HandSlot   `json:"main"`
	Off            HandSlot   `json:"off"`
	Grip           Grip       `json:"grip"`
	SuspendedOff   *HandSlot  `json:"suspendedOff,omitempty"`
	RestoreReason  HandReason `json:"restoreReason,omitempty"`
	UnrestoredItem string     `json:"unrestoredItem,omitempty"`
}

type HandSlot struct {
	ItemID    string     `json:"itemId"`
	Name      string     `json:"name"`
	Reference *Reference `json:"reference,omitempty"`
	Active    bool       `json:"active"`
}

// QuickUsePin is a pinned inventory instance; a missing one keeps only its ID.
type QuickUsePin struct {
	ItemID    string     `json:"itemId"`
	Name      string     `json:"name,omitempty"`
	Quantity  *int       `json:"quantity,omitempty"`
	Location  string     `json:"location,omitempty"`
	Reference *Reference `json:"reference,omitempty"`
	Available bool       `json:"available"`
	Reason    ItemReason `json:"reason"`
}

// ActiveItem is a magic item whose properties apply now.
type ActiveItem struct {
	ItemID string    `json:"itemId"`
	Name   string    `json:"name"`
	Ref    Reference `json:"ref"`
}

type SheetStorage struct {
	Containers []StorageContainer `json:"containers"`
}

type StorageContainer struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	ItemIDs []string `json:"itemIds"`
}

type PlacementRow struct {
	ItemID    string `json:"itemId"`
	Name      string `json:"name"`
	Placement string `json:"placement"`
}

// SheetCondition is a tracked condition, or one active items grant (Granted).
type SheetCondition struct {
	ID                string          `json:"id"`
	Level             int             `json:"level"`
	Name              string          `json:"name"`
	LabelKey          string          `json:"labelKey,omitempty"`
	Summary           string          `json:"summary"`
	Status            ConditionStatus `json:"status"`
	Reference         *Reference      `json:"reference,omitempty"`
	Terminal          *bool           `json:"terminal,omitempty"`
	EndsConcentration bool            `json:"endsConcentration,omitempty"`
	Granted           bool            `json:"granted,omitempty"`
	Sources           []Source        `json:"sources,omitempty"`
}

type ConditionEffects struct {
	D20Adjustment int `json:"d20Adjustment"`
}

// EquipmentFacts are an item's source-declared slot and placements.
type EquipmentFacts struct {
	Slot           string   `json:"slot"`
	BodyPlacements []string `json:"bodyPlacements"`
}

// Carrying is carried weight against capacity, in pounds.
type Carrying struct {
	CapacityLb   float64                     `json:"capacityLb"`
	WeightLb     float64                     `json:"weightLb"`
	ItemsLb      float64                     `json:"itemsLb"`
	CoinsLb      float64                     `json:"coinsLb"`
	StoredLb     float64                     `json:"storedLb"`
	WeightlessLb float64                     `json:"weightlessLb"`
	CoinWeight   bool                        `json:"coinWeight"`
	Items        map[string]CarriedItem      `json:"items"`
	Containers   map[string]CarriedContainer `json:"containers"`
	Unknown      []string                    `json:"unknown"`
}

type CarriedItem struct {
	EachLb  float64 `json:"eachLb"`
	TotalLb float64 `json:"totalLb"`
	Custom  bool    `json:"custom"`
}

type CarriedContainer struct {
	LoadLb     float64 `json:"loadLb"`
	CapacityLb float64 `json:"capacityLb,omitempty"`
	Weightless bool    `json:"weightless"`
	Stored     bool    `json:"stored"`
}

type SheetConcentration struct {
	Ref         string `json:"ref"`
	Name        string `json:"name"`
	SaveAbility string `json:"saveAbility,omitempty"`
	SaveBonus   *int   `json:"saveBonus,omitempty"`
	SaveDC      int    `json:"saveDC,omitempty"`
	EndedBy     string `json:"endedBy,omitempty"`
}

// TableValue is a class table entry: text such as "1d6" or a number.
type TableValue struct{ raw json.RawMessage }

// TableValueOf keeps a record's string or number; anything else is empty text.
func TableValueOf(value any) TableValue {
	switch value.(type) {
	case string, float64, int:
		body, _ := json.Marshal(value)
		return TableValue{body}
	}
	return TableValue{json.RawMessage(`""`)}
}

// Text is the entry as text and Number as a number (zero for text).
func (value TableValue) Text() string {
	var text string
	if json.Unmarshal(value.raw, &text) == nil {
		return text
	}
	return string(value.raw)
}

func (value TableValue) Number() (float64, bool) {
	var number float64
	err := json.Unmarshal(value.raw, &number)
	return number, err == nil
}

func (value TableValue) MarshalJSON() ([]byte, error) {
	if len(value.raw) == 0 {
		return []byte(`""`), nil
	}
	return value.raw, nil
}

func (value *TableValue) UnmarshalJSON(body []byte) error {
	var decoded any
	if err := json.Unmarshal(body, &decoded); err != nil {
		return err
	}
	switch decoded.(type) {
	case string, float64:
		value.raw = append(json.RawMessage(nil), body...)
		return nil
	}
	return fmt.Errorf("table value must be text or a number")
}

func (TableValue) JSONSchema() map[string]any {
	return map[string]any{"type": []string{"string", "number"}}
}
func (TableValue) TypeScriptType() string { return "string | number" }

// MovementValue is a speed in the ruleset's distance unit, or FollowsSpeed
// for a speed equal to the walking speed (times the row's multiplier).
type MovementValue struct {
	Distance     int
	FollowsSpeed bool
}

func (value MovementValue) MarshalJSON() ([]byte, error) {
	if value.FollowsSpeed {
		return []byte(`"speed"`), nil
	}
	return []byte(strconv.Itoa(value.Distance)), nil
}

func (value *MovementValue) UnmarshalJSON(body []byte) error {
	if string(body) == `"speed"` {
		*value = MovementValue{FollowsSpeed: true}
		return nil
	}
	*value = MovementValue{}
	return json.Unmarshal(body, &value.Distance)
}

func (MovementValue) JSONSchema() map[string]any {
	return map[string]any{"oneOf": []any{map[string]any{"type": "integer"}, map[string]any{"const": "speed"}}}
}
func (MovementValue) TypeScriptType() string { return `number | "speed"` }

// RechargeAmount is how much a rest restores: everything, half the total
// level, a fixed number, or an ability modifier (at least one).
type RechargeAmount struct {
	Full      bool
	HalfLevel bool
	Uses      int
	Ability   string
}

func (amount RechargeAmount) MarshalJSON() ([]byte, error) {
	switch {
	case amount.Full:
		return []byte(`"full"`), nil
	case amount.HalfLevel:
		return []byte(`"halfLevel"`), nil
	case amount.Ability != "":
		return json.Marshal(map[string]string{"abilityMod": amount.Ability})
	}
	return []byte(strconv.Itoa(amount.Uses)), nil
}

func (amount *RechargeAmount) UnmarshalJSON(body []byte) error {
	var decoded any
	if err := json.Unmarshal(body, &decoded); err != nil {
		return err
	}
	*amount = RechargeAmount{}
	switch current := decoded.(type) {
	case string:
		switch current {
		case "full":
			amount.Full = true
		case "halfLevel":
			amount.HalfLevel = true
		default:
			return fmt.Errorf("unknown recharge amount %q", current)
		}
	case float64:
		amount.Uses = int(current)
	case map[string]any:
		ability, _ := current["abilityMod"].(string)
		if ability == "" || len(current) != 1 {
			return fmt.Errorf("recharge amount object needs only abilityMod")
		}
		amount.Ability = ability
	default:
		return fmt.Errorf("invalid recharge amount")
	}
	return nil
}

func (RechargeAmount) JSONSchema() map[string]any {
	return map[string]any{"oneOf": []any{
		map[string]any{"enum": []string{"full", "halfLevel"}},
		map[string]any{"type": "integer"},
		map[string]any{"type": "object", "additionalProperties": false, "required": []string{"abilityMod"}, "properties": map[string]any{"abilityMod": map[string]any{"type": "string"}}},
	}}
}
func (RechargeAmount) TypeScriptType() string {
	return `"full" | "halfLevel" | number | { abilityMod: string }`
}

// PreparationCount is "all" or a number of spells.
type PreparationCount struct {
	All   bool
	Count int
}

func (count PreparationCount) MarshalJSON() ([]byte, error) {
	if count.All {
		return []byte(`"all"`), nil
	}
	return []byte(strconv.Itoa(count.Count)), nil
}

func (count *PreparationCount) UnmarshalJSON(body []byte) error {
	if string(body) == `"all"` {
		*count = PreparationCount{All: true}
		return nil
	}
	*count = PreparationCount{}
	return json.Unmarshal(body, &count.Count)
}

func (PreparationCount) JSONSchema() map[string]any {
	return map[string]any{"oneOf": []any{map[string]any{"const": "all"}, map[string]any{"type": "integer"}}}
}
func (PreparationCount) TypeScriptType() string { return `"all" | number` }
