package character

import (
	"slices"
	"sort"
)

// Every closed set is a named string type whose values are listed once, in a
// map literal the exhaustive linter checks. Valid reports membership and
// EnumValues feeds the schema and TypeScript generators.

func sortedValues[T ~string](set map[T]bool) []string {
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, string(value))
	}
	sort.Strings(result)
	return result
}

// IssueSeverity says whether an issue blocks play (blocker), needs attention
// (warning) or records a decision such as a DM waiver (info).
type IssueSeverity string

const (
	SeverityBlocker IssueSeverity = "blocker"
	SeverityWarning IssueSeverity = "warning"
	SeverityInfo    IssueSeverity = "info"
)

var issueSeverities = map[IssueSeverity]bool{SeverityBlocker: true, SeverityWarning: true, SeverityInfo: true}

func (IssueSeverity) EnumValues() []string { return sortedValues(issueSeverities) }
func (severity IssueSeverity) Valid() bool { return issueSeverities[severity] }

// CreationMethod is how the base ability scores were chosen.
type CreationMethod string

const (
	MethodPointBuy CreationMethod = "point-buy"
	MethodArray    CreationMethod = "array"
	MethodRolled   CreationMethod = "rolled"
)

var creationMethods = map[CreationMethod]bool{MethodPointBuy: true, MethodArray: true, MethodRolled: true}

func (CreationMethod) EnumValues() []string { return sortedValues(creationMethods) }
func (method CreationMethod) Valid() bool   { return creationMethods[method] }

// EffectTarget is the statistic a typed DM or item effect changes.
type EffectTarget string

const (
	TargetAbilityScore    EffectTarget = "abilityScore"
	TargetAbilityCap      EffectTarget = "abilityCap"
	TargetSavingThrow     EffectTarget = "savingThrow"
	TargetArmorClass      EffectTarget = "armorClass"
	TargetInitiative      EffectTarget = "initiative"
	TargetSpeed           EffectTarget = "speed"
	TargetMaxHP           EffectTarget = "maxHp"
	TargetAttunementLimit EffectTarget = "attunementLimit"
	TargetSense           EffectTarget = "sense"
	TargetResourceMax     EffectTarget = "resourceMax"
	TargetProficiency     EffectTarget = "proficiency"
)

var effectTargets = map[EffectTarget]bool{
	TargetAbilityScore: true, TargetAbilityCap: true, TargetSavingThrow: true, TargetArmorClass: true,
	TargetInitiative: true, TargetSpeed: true, TargetMaxHP: true, TargetAttunementLimit: true,
	TargetSense: true, TargetResourceMax: true, TargetProficiency: true,
}

func (EffectTarget) EnumValues() []string { return sortedValues(effectTargets) }
func (target EffectTarget) Valid() bool   { return effectTargets[target] }

// EffectMode is how an effect combines with the calculated value.
type EffectMode string

const (
	ModeSet     EffectMode = "set"
	ModeAdd     EffectMode = "add"
	ModeMinimum EffectMode = "minimum"
	ModeMaximum EffectMode = "maximum"
)

var effectModes = map[EffectMode]bool{ModeSet: true, ModeAdd: true, ModeMinimum: true, ModeMaximum: true}

func (EffectMode) EnumValues() []string { return sortedValues(effectModes) }
func (mode EffectMode) Valid() bool     { return effectModes[mode] }

// GrantCondition says when a DM grant applies.
type GrantCondition string

const (
	ConditionAlways   GrantCondition = "always"
	ConditionEquipped GrantCondition = "equipped"
	ConditionAttuned  GrantCondition = "attuned"
)

var grantConditions = map[GrantCondition]bool{ConditionAlways: true, ConditionEquipped: true, ConditionAttuned: true}

func (GrantCondition) EnumValues() []string  { return sortedValues(grantConditions) }
func (condition GrantCondition) Valid() bool { return grantConditions[condition] }

// Grip is how many hands hold the main-hand item.
type Grip string

const (
	GripOne Grip = "one"
	GripTwo Grip = "two"
)

var grips = map[Grip]bool{GripOne: true, GripTwo: true}

func (Grip) EnumValues() []string { return sortedValues(grips) }
func (grip Grip) Valid() bool     { return grips[grip] }

// Hand is the main or off hand.
type Hand string

const (
	HandMain Hand = "main"
	HandOff  Hand = "off"
)

var hands = map[Hand]bool{HandMain: true, HandOff: true}

func (Hand) EnumValues() []string { return sortedValues(hands) }
func (hand Hand) Valid() bool     { return hands[hand] }

// HandReason is why a hand item cannot be used or restored; empty when it can.
type HandReason string

const (
	HandUsable     HandReason = ""
	HandMissing    HandReason = "missing"
	HandEmpty      HandReason = "empty"
	HandChanged    HandReason = "changed"
	HandIneligible HandReason = "ineligible"
	HandOccupied   HandReason = "occupied"
)

var handReasons = map[HandReason]bool{HandUsable: true, HandMissing: true, HandEmpty: true, HandChanged: true, HandIneligible: true, HandOccupied: true}

func (HandReason) EnumValues() []string { return sortedValues(handReasons) }
func (reason HandReason) Valid() bool   { return handReasons[reason] }

// ReplacementKind is what a class-level replacement swaps.
type ReplacementKind string

const (
	ReplaceFeat  ReplacementKind = "feat"
	ReplaceSpell ReplacementKind = "spell"
)

var replacementKinds = map[ReplacementKind]bool{ReplaceFeat: true, ReplaceSpell: true}

func (ReplacementKind) EnumValues() []string { return sortedValues(replacementKinds) }
func (kind ReplacementKind) Valid() bool     { return replacementKinds[kind] }

// RestKind is a short or long rest.
type RestKind string

const (
	RestShort RestKind = "short"
	RestLong  RestKind = "long"
)

var restKinds = map[RestKind]bool{RestShort: true, RestLong: true}

func (RestKind) EnumValues() []string { return sortedValues(restKinds) }
func (rest RestKind) Valid() bool     { return restKinds[rest] }

// SpellList is a class's spell selection list.
type SpellList string

const (
	ListCantrips  SpellList = "cantrips"
	ListSpellbook SpellList = "spellbook"
	ListPrepared  SpellList = "preparedSpells"
)

var spellLists = map[SpellList]bool{ListCantrips: true, ListSpellbook: true, ListPrepared: true}

func (SpellList) EnumValues() []string { return sortedValues(spellLists) }
func (list SpellList) Valid() bool     { return spellLists[list] }

// SourceType is what a sheet entry comes from. The first seven are grant
// sources; the others name sheet-only origins.
type SourceType string

const (
	SourceClass        SourceType = "class"
	SourceSubclass     SourceType = "subclass"
	SourceFeature      SourceType = "feature"
	SourceSpecies      SourceType = "species"
	SourceBackground   SourceType = "background"
	SourceFeat         SourceType = "feat"
	SourceItem         SourceType = "item"
	SourceSpellcasting SourceType = "spellcasting"
	SourcePactMagic    SourceType = "pactMagic"
	SourceArmor        SourceType = "armor"
	SourceCondition    SourceType = "condition"
	SourceSpell        SourceType = "spell"
	SourceASI          SourceType = "asi"
	SourceAbility      SourceType = "ability"
)

var sourceTypes = map[SourceType]bool{
	SourceClass: true, SourceSubclass: true, SourceFeature: true, SourceSpecies: true, SourceBackground: true,
	SourceFeat: true, SourceItem: true, SourceSpellcasting: true, SourcePactMagic: true, SourceArmor: true,
	SourceCondition: true, SourceSpell: true, SourceASI: true, SourceAbility: true,
}

func (SourceType) EnumValues() []string { return sortedValues(sourceTypes) }
func (kind SourceType) Valid() bool     { return sourceTypes[kind] }

// ChoiceKind is what a plan choice selects.
type ChoiceKind string

const (
	ChoiceSkills         ChoiceKind = "skills"
	ChoiceExpertise      ChoiceKind = "expertise"
	ChoiceSkillExpertise ChoiceKind = "skillExpertise"
	ChoiceSkillUpgrade   ChoiceKind = "skillUpgrade"
	ChoiceTools          ChoiceKind = "tools"
	ChoiceProficiencies  ChoiceKind = "proficiencies"
	ChoiceWeaponMastery  ChoiceKind = "weaponMastery"
	ChoiceLanguages      ChoiceKind = "languages"
	ChoiceSavingThrows   ChoiceKind = "savingThrows"
	ChoiceWeapons        ChoiceKind = "weapons"
	ChoiceArmor          ChoiceKind = "armor"
	ChoiceResistances    ChoiceKind = "resistances"
	ChoiceImmunities     ChoiceKind = "immunities"
	ChoiceFeat           ChoiceKind = "feat"
	ChoiceEnumerated     ChoiceKind = "enumerated"
	ChoiceASIMode        ChoiceKind = "asiMode"
	ChoiceAbilityBudget  ChoiceKind = "abilityBudget"
	ChoiceSize           ChoiceKind = "size"
)

var choiceKinds = map[ChoiceKind]bool{
	ChoiceSkills: true, ChoiceExpertise: true, ChoiceSkillExpertise: true, ChoiceSkillUpgrade: true,
	ChoiceTools: true, ChoiceProficiencies: true, ChoiceWeaponMastery: true, ChoiceLanguages: true,
	ChoiceSavingThrows: true, ChoiceWeapons: true, ChoiceArmor: true, ChoiceResistances: true,
	ChoiceImmunities: true, ChoiceFeat: true, ChoiceEnumerated: true, ChoiceASIMode: true,
	ChoiceAbilityBudget: true, ChoiceSize: true,
}

func (ChoiceKind) EnumValues() []string { return sortedValues(choiceKinds) }
func (kind ChoiceKind) Valid() bool     { return choiceKinds[kind] }

// SheetStatus says whether the statistics are calculated.
type SheetStatus string

const (
	SheetCalculated   SheetStatus = "calculated"
	SheetNeedsChoices SheetStatus = "needs-choices"
)

var sheetStatuses = map[SheetStatus]bool{SheetCalculated: true, SheetNeedsChoices: true}

func (SheetStatus) EnumValues() []string { return sortedValues(sheetStatuses) }

// ResourceKind is a tracked pool, spell slot, free-cast charge or hit dice.
type ResourceKind string

const (
	ResourcePool    ResourceKind = "pool"
	ResourceSlot    ResourceKind = "slot"
	ResourceCharge  ResourceKind = "charge"
	ResourceHitDice ResourceKind = "hitdice"
)

var resourceKinds = map[ResourceKind]bool{ResourcePool: true, ResourceSlot: true, ResourceCharge: true, ResourceHitDice: true}

func (ResourceKind) EnumValues() []string { return sortedValues(resourceKinds) }

// RechargeOn is the rest that restores a resource.
type RechargeOn string

const (
	RechargeShort       RechargeOn = "short"
	RechargeLong        RechargeOn = "long"
	RechargeShortOrLong RechargeOn = "shortOrLong"
)

var rechargeOns = map[RechargeOn]bool{RechargeShort: true, RechargeLong: true, RechargeShortOrLong: true}

func (RechargeOn) EnumValues() []string { return sortedValues(rechargeOns) }
func (on RechargeOn) Valid() bool       { return rechargeOns[on] }

// RollKind is a roll that Advantage or Disadvantage can apply to.
type RollKind string

const (
	RollSave          RollKind = "save"
	RollCheck         RollKind = "check"
	RollAttack        RollKind = "attack"
	RollInitiative    RollKind = "initiative"
	RollConcentration RollKind = "concentration"
	RollDeathSave     RollKind = "deathSave"
)

var rollKinds = map[RollKind]bool{RollSave: true, RollCheck: true, RollAttack: true, RollInitiative: true, RollConcentration: true, RollDeathSave: true}

func (RollKind) EnumValues() []string { return sortedValues(rollKinds) }
func (kind RollKind) Valid() bool     { return rollKinds[kind] }

// RollMode is Advantage or Disadvantage.
type RollMode string

const (
	RollAdvantage    RollMode = "advantage"
	RollDisadvantage RollMode = "disadvantage"
)

var rollModes = map[RollMode]bool{RollAdvantage: true, RollDisadvantage: true}

func (RollMode) EnumValues() []string { return sortedValues(rollModes) }
func (mode RollMode) Valid() bool     { return rollModes[mode] }

// MovementMode is a special movement speed.
type MovementMode string

const (
	MoveFly    MovementMode = "fly"
	MoveSwim   MovementMode = "swim"
	MoveClimb  MovementMode = "climb"
	MoveBurrow MovementMode = "burrow"
)

var movementModes = map[MovementMode]bool{MoveFly: true, MoveSwim: true, MoveClimb: true, MoveBurrow: true}

func (MovementMode) EnumValues() []string { return sortedValues(movementModes) }

// MovementModes lists the special movement speeds in sheet order.
func MovementModes() []MovementMode { return []MovementMode{MoveFly, MoveSwim, MoveClimb, MoveBurrow} }

// ConditionStatus is whether a tracked condition applies.
type ConditionStatus string

const (
	ConditionActive      ConditionStatus = "active"
	ConditionImmune      ConditionStatus = "immune"
	ConditionUnavailable ConditionStatus = "unavailable"
)

var conditionStatuses = map[ConditionStatus]bool{ConditionActive: true, ConditionImmune: true, ConditionUnavailable: true}

func (ConditionStatus) EnumValues() []string { return sortedValues(conditionStatuses) }

// BonusStatus says whether a conditional bonus applies now.
type BonusStatus string

const (
	BonusApplied  BonusStatus = "applied"
	BonusInactive BonusStatus = "inactive"
)

var bonusStatuses = map[BonusStatus]bool{BonusApplied: true, BonusInactive: true}

func (BonusStatus) EnumValues() []string { return sortedValues(bonusStatuses) }

// ItemReason is why an inventory item cannot be used now; empty when it can.
type ItemReason string

const (
	ItemUsable  ItemReason = ""
	ItemEmpty   ItemReason = "empty"
	ItemStored  ItemReason = "stored"
	ItemBuild   ItemReason = "build"
	ItemMissing ItemReason = "missing"
)

var itemReasons = map[ItemReason]bool{ItemUsable: true, ItemEmpty: true, ItemStored: true, ItemBuild: true, ItemMissing: true}

func (ItemReason) EnumValues() []string { return sortedValues(itemReasons) }

// Abilities lists the six ability IDs in sheet order.
var Abilities = []string{"STR", "DEX", "CON", "INT", "WIS", "CHA"}

// IsAbility reports whether id is an ability ID.
func IsAbility(id string) bool { return slices.Contains(Abilities, id) }
