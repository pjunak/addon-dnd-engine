package character

// TemporaryEffect is a value that temporary play state changes now: an
// authored condition, a feature the player switched on or a DM grant that
// expires. Path names the value as the explanations do ("derived.armorClass",
// "saves.STR.total"); a value in a list row also names the row by a stable
// key (see the contract README), never by its position. Tone sums up its
// causes, which say what each does.
type TemporaryEffect struct {
	Path   string        `json:"path"`
	Row    string        `json:"row,omitempty"`
	Tone   EffectTone    `json:"tone"`
	Causes []EffectCause `json:"causes"`
}

// EffectCause is one temporary cause of a TemporaryEffect. ID is the
// condition's ID, the activation's key or the grant's ID. Name is the record
// or grant name as it is; a condition's LabelKey is its rules-data label key,
// and Level its level when the condition has levels.
type EffectCause struct {
	Kind     CauseKind      `json:"kind"`
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	LabelKey string         `json:"labelKey,omitempty"`
	Level    int            `json:"level,omitempty"`
	Tone     EffectTone     `json:"tone"`
	Changes  []EffectChange `json:"changes"`
}

// EffectChange is what a cause does to a value. An add or set change carries
// Value and Included: whether the displayed value already holds it. The D20
// adjustment of a condition such as Exhaustion is not included; a client
// applies it to the roll.
type EffectChange struct {
	Kind     ChangeKind `json:"kind"`
	Value    *int       `json:"value,omitempty"`
	Included *bool      `json:"included,omitempty"`
}

// EffectTone says whether a temporary change helps (bonus), hinders
// (penalty) or both or neither (mixed). Every marked value is better higher.
type EffectTone string

const (
	ToneBonus   EffectTone = "bonus"
	TonePenalty EffectTone = "penalty"
	ToneMixed   EffectTone = "mixed"
)

var effectTones = map[EffectTone]bool{ToneBonus: true, TonePenalty: true, ToneMixed: true}

func (EffectTone) EnumValues() []string { return sortedValues(effectTones) }
func (tone EffectTone) Valid() bool     { return effectTones[tone] }

// CauseKind is the play state a temporary cause comes from.
type CauseKind string

const (
	CauseCondition     CauseKind = "condition"
	CauseActiveFeature CauseKind = "active-feature"
	CauseTimedGrant    CauseKind = "timed-grant"
)

var causeKinds = map[CauseKind]bool{CauseCondition: true, CauseActiveFeature: true, CauseTimedGrant: true}

func (CauseKind) EnumValues() []string { return sortedValues(causeKinds) }
func (kind CauseKind) Valid() bool     { return causeKinds[kind] }

// ChangeKind is how a cause changes a value: adds a number (negative for a
// penalty), sets it (Speed 0), gives Advantage or Disadvantage on its roll, or
// changes it in a way a number does not say (another damage die).
type ChangeKind string

const (
	ChangeAdd          ChangeKind = "add"
	ChangeSet          ChangeKind = "set"
	ChangeAdvantage    ChangeKind = "advantage"
	ChangeDisadvantage ChangeKind = "disadvantage"
	ChangeOther        ChangeKind = "other"
)

var changeKinds = map[ChangeKind]bool{ChangeAdd: true, ChangeSet: true, ChangeAdvantage: true, ChangeDisadvantage: true, ChangeOther: true}

func (ChangeKind) EnumValues() []string { return sortedValues(changeKinds) }
func (kind ChangeKind) Valid() bool     { return changeKinds[kind] }

// AttackKey is the stable key of an attack row that TemporaryEffect.Row uses:
// "item:" and its inventory ID, "feature:" and its feature attack's ID, or
// "unarmed" for the unarmed strike.
func AttackKey(attack Attack) string {
	switch {
	case attack.ItemID != "":
		return "item:" + attack.ItemID
	case attack.FeatureAttack != "":
		return "feature:" + attack.FeatureAttack
	case attack.Unarmed:
		return "unarmed"
	default:
		return "ref:" + attack.Ref
	}
}
