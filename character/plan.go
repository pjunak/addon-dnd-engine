package character

// Plan is the Builder's decision structure: base scores, classes, point-buy
// policy and every choice the current build offers.
type Plan struct {
	SchemaVersion          int            `json:"schemaVersion"`
	Edition                string         `json:"edition"`
	BaseStats              map[string]int `json:"baseStats"`
	Classes                []PlanClass    `json:"classes"`
	PointBuy               PointBuy       `json:"pointBuy"`
	AbilityScoreRange      Range          `json:"abilityScoreRange"`
	ClassChoices           []PlanChoice   `json:"classChoices"`
	CreationChoices        []PlanChoice   `json:"creationChoices"`
	CreationAbilityChoices []PlanChoice   `json:"creationAbilityChoices"`
}

type PlanClass struct {
	ClassID  string `json:"classId"`
	Level    int    `json:"level"`
	Subclass string `json:"subclass"`
}

type PointBuy struct {
	Budget int            `json:"budget"`
	Min    int            `json:"min"`
	Max    int            `json:"max"`
	Cost   map[string]int `json:"cost"`
}

type Range struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

// PlanChoice is one choice the build offers. Kind decides which optional
// fields apply: asiMode carries Ability and Feat, abilityBudget carries the
// budget fields, and the others carry Count and their option pool.
type PlanChoice struct {
	ID                         string       `json:"id"`
	Kind                       ChoiceKind   `json:"kind"`
	Count                      int          `json:"count,omitempty"`
	ClassID                    string       `json:"classId,omitempty"`
	Level                      int          `json:"level,omitempty"`
	Source                     *Source      `json:"source,omitempty"`
	From                       []string     `json:"from,omitempty"`
	FromCategory               string       `json:"fromCategory,omitempty"`
	RepeatableOptions          []string     `json:"repeatableOptions,omitempty"`
	Category                   string       `json:"category,omitempty"`
	Categories                 []string     `json:"categories,omitempty"`
	Prompt                     string       `json:"prompt,omitempty"`
	Default                    string       `json:"default,omitempty"`
	ChangeOn                   string       `json:"changeOn,omitempty"`
	LevelReplacements          int          `json:"levelReplacements,omitempty"`
	OptionKind                 string       `json:"optionKind,omitempty"`
	Ability                    *PlanChoice  `json:"ability,omitempty"`
	Feat                       *FeatChoice  `json:"feat,omitempty"`
	Eligible                   []string     `json:"eligible,omitempty"`
	Budget                     int          `json:"budget,omitempty"`
	PerAbilityMax              int          `json:"perAbilityMax,omitempty"`
	Cap                        int          `json:"cap,omitempty"`
	Acquisition                *Acquisition `json:"acquisition,omitempty"`
	Name                       string       `json:"name,omitempty"`
	ChoiceName                 string       `json:"choiceName,omitempty"`
	DistinctAcrossAcquisitions bool         `json:"distinctAcrossAcquisitions,omitempty"`
}

// FeatChoice is the feat an advancement may take instead of ability
// increases, and the increase that feat may offer.
type FeatChoice struct {
	ID                  string         `json:"id"`
	Categories          []string       `json:"categories"`
	Ability             *PlanChoice    `json:"ability"`
	CategoryAbilityCaps map[string]int `json:"categoryAbilityCaps"`
}

// Choice is the feat choice as an ordinary feat descriptor.
func (feat FeatChoice) Choice() PlanChoice {
	return PlanChoice{ID: feat.ID, Kind: ChoiceFeat, Categories: feat.Categories}
}

// Guidance explains the build's completion and what each control may do.
type Guidance struct {
	Choices           map[string]ChoiceGuidance    `json:"choices"`
	Classes           []ClassGuidance              `json:"classes"`
	Sections          []GuidanceSection            `json:"sections"`
	Total             int                          `json:"total"`
	Complete          int                          `json:"complete"`
	Ready             bool                         `json:"ready"`
	Derived           Derived                      `json:"derived"`
	Warnings          []string                     `json:"warnings"`
	CanSave           bool                         `json:"canSave"`
	SaveIssues        []Issue                      `json:"saveIssues"`
	ClassOptions      []Option                     `json:"classOptions"`
	ClassReplacements []ReplacementGuidance        `json:"classReplacements"`
	Equipment         map[string]EquipmentGuidance `json:"equipment"`
	Hands             HandsGuidance                `json:"hands"`
	QuickUse          map[string]ItemUse           `json:"quickUse"`
	Conditions        ConditionGuidance            `json:"conditions"`
	Storage           StorageGuidance              `json:"storage"`
	AuthoredPlay      AuthoredPlay                 `json:"authoredPlay"`
}

// Text is Engine wording: the English Label, plus a LabelKey template and
// LabelArgs that clients may translate instead.
type Text struct {
	Label     string   `json:"label"`
	LabelKey  string   `json:"labelKey"`
	LabelArgs []string `json:"labelArgs"`
}

type ChoiceGuidance struct {
	ID string `json:"id"`
	Text
	Options     []Option `json:"options"`
	FeatOptions []Option `json:"featOptions,omitempty"`
	Picked      int      `json:"picked"`
	Required    int      `json:"required"`
	Done        bool     `json:"done"`
}

// Option is a choosable record or value.
type Option struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	LabelKey    string `json:"labelKey,omitempty"`
	Description string `json:"description,omitempty"`
}

type ClassGuidance struct {
	ClassID       string          `json:"classId"`
	Name          string          `json:"name"`
	Level         int             `json:"level"`
	SubclassLevel int             `json:"subclassLevel"`
	HitDieMax     int             `json:"hitDieMax"`
	Subclasses    []Option        `json:"subclasses"`
	Levels        []LevelGuidance `json:"levels"`
}

type LevelGuidance struct {
	Level    int      `json:"level"`
	Features []Option `json:"features"`
}

// GuidanceSection counts one Builder section's required decisions.
type GuidanceSection struct {
	ID       string          `json:"id"`
	Total    int             `json:"total"`
	Complete int             `json:"complete"`
	Issues   []GuidanceIssue `json:"issues"`
}

// GuidanceIssue is an unfinished decision, or one to repair (Repair).
type GuidanceIssue struct {
	ID string `json:"id"`
	Text
	Tab      string        `json:"tab"`
	Level    int           `json:"level,omitempty"`
	Repair   bool          `json:"repair,omitempty"`
	Severity IssueSeverity `json:"severity,omitempty"`
}

// ReplacementGuidance is a class-level replacement a grant allows.
type ReplacementGuidance struct {
	Kind       ReplacementKind `json:"kind"`
	Key        string          `json:"key"`
	ClassID    string          `json:"classId"`
	ClassLevel int             `json:"classLevel"`
	Source     Reference       `json:"source"`
	Name       string          `json:"name"`
	Remaining  int             `json:"remaining"`
	Picked     []PickedOption  `json:"picked"`
	Options    []Option        `json:"options"`
}

type PickedOption struct {
	ID    string `json:"id"`
	Slot  int    `json:"slot"`
	Label string `json:"label"`
}

// EquipmentGuidance says whether an item can be equipped or attuned, and why
// not when it cannot.
type EquipmentGuidance struct {
	Slot           string   `json:"slot"`
	BodyPlacements []string `json:"bodyPlacements"`
	CanEquip       bool     `json:"canEquip"`
	CanAttune      bool     `json:"canAttune"`
	EquipReason    string   `json:"equipReason,omitempty"`
	AttuneReason   string   `json:"attuneReason,omitempty"`
	// BaseItems and Bonuses are what a magic weapon or armor may be, from its
	// record's baseItem, by name; with several, the owner picks one.
	BaseItems []Option `json:"baseItems,omitempty"`
	Bonuses   []int    `json:"bonuses,omitempty"`
}

type HandsGuidance struct {
	Options       []HandOption `json:"options"`
	Grips         []Grip       `json:"grips"`
	CanRelease    bool         `json:"canRelease"`
	CanTwo        bool         `json:"canTwo"`
	RestoreReason HandReason   `json:"restoreReason"`
}

type HandOption struct {
	ItemID  string `json:"itemId"`
	Name    string `json:"name"`
	Grips   []Grip `json:"grips"`
	CanMain bool   `json:"canMain"`
	CanOff  bool   `json:"canOff"`
}

type ItemUse struct {
	CanUse bool       `json:"canUse"`
	Reason ItemReason `json:"reason"`
}

type ConditionGuidance struct {
	Available bool              `json:"available"`
	Options   []ConditionOption `json:"options"`
}

type ConditionOption struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	LabelKey     string    `json:"labelKey"`
	Summary      string    `json:"summary"`
	MaximumLevel int       `json:"maximumLevel"`
	CanAdd       bool      `json:"canAdd"`
	Reference    Reference `json:"reference"`
}

type StorageGuidance struct {
	MaximumContainers int             `json:"maximumContainers"`
	MaximumNameLength int             `json:"maximumNameLength"`
	ContainerItems    []ContainerItem `json:"containerItems"`
}

// ContainerItem is an inventory item whose record makes it a container.
type ContainerItem struct {
	ItemID     string  `json:"itemId"`
	CapacityLb float64 `json:"capacityLb,omitempty"`
	Weightless bool    `json:"weightless"`
}

// AuthoredPlay lists the play state this Engine lets consumers edit.
type AuthoredPlay struct {
	Inspiration   bool `json:"inspiration"`
	QuickUse      bool `json:"quickUse"`
	Storage       bool `json:"storage"`
	BodyPlacement bool `json:"bodyPlacement"`
	Hands         bool `json:"hands"`
	Conditions    bool `json:"conditions"`
}

// SpellOptions describes the spell controls without changing the sheet.
type SpellOptions struct {
	Classes               []CasterOptions        `json:"classes"`
	PendingChoices        []SpellChoiceOptions   `json:"pendingChoices"`
	CastingAbilityChoices []CastingAbilityChoice `json:"castingAbilityChoices"`
	Granted               []SpellGrantOptions    `json:"granted"`
	Slots                 []SlotOption           `json:"slots"`
	AcquisitionLevels     []AcquisitionLevel     `json:"acquisitionLevels"`
}

type CasterOptions struct {
	ClassID               string              `json:"classId"`
	SpellIDs              []string            `json:"spellIds"`
	RitualIDs             []string            `json:"ritualIds"`
	CopyCosts             map[string]float64  `json:"copyCosts"`
	CastSlots             map[string][]string `json:"castSlots"`
	CanSwap               bool                `json:"canSwap"`
	ReplacementsRemaining int                 `json:"replacementsRemaining"`
}

type SpellChoiceOptions struct {
	SpellChoice
	EligibleSpellIDs []string `json:"eligibleSpellIds"`
}

// SpellGrantOptions is a granted spell with its play key and the slots or
// resources that can cast it ("" casts it free).
type SpellGrantOptions struct {
	SpellGrant
	Key   string   `json:"key"`
	Slots []string `json:"slots"`
}

type SlotOption struct {
	Key     string `json:"key"`
	Name    string `json:"name"`
	Max     int    `json:"max"`
	Current int    `json:"current"`
}

// AcquisitionLevel records which level gained a level-granted spellbook spell.
type AcquisitionLevel struct {
	ID         string    `json:"id"`
	ClassID    string    `json:"classId"`
	Level      int       `json:"level"`
	ClassLevel int       `json:"classLevel"`
	Source     Reference `json:"source"`
}
