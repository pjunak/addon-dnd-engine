// Package character contains the serializable rules-engine character contract.
// It has no storage, UI, service discovery, or engine implementation dependency.
package character

import "encoding/json"

const ContractVersion = "rules-character.v1"

type Reference struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}
type Choice struct {
	ID    string          `json:"id"`
	Slot  int             `json:"slot"`
	Value json.RawMessage `json:"value"`
}
type Roll struct {
	Origin  string `json:"origin,omitempty"`
	ID      string `json:"id"`
	Ability string `json:"ability"`
	Dice    []int  `json:"dice"`
	Kept    []int  `json:"kept"`
}
type Level struct {
	ID        string `json:"id"`
	ClassID   string `json:"classId"`
	HitPoints *int   `json:"hitPoints,omitempty"`
}
type SpellDecisions struct {
	Cantrips         map[string][]string `json:"cantrips"`
	Spellbook        map[string][]string `json:"spellbook"`
	GrantChoices     map[string][]string `json:"grantChoices"`
	CastingAbilities map[string]string   `json:"castingAbilities"`
	Swaps            []SpellSwap         `json:"swaps"`
	Acquisitions     []SpellAcquisition  `json:"acquisitions"`
}
type SpellSwap struct {
	Origin     string `json:"origin,omitempty"`
	Level      int    `json:"level"`
	ClassLevel int    `json:"classLevel"`
	ClassID    string `json:"classId"`
	Out        string `json:"out"`
	In         string `json:"in"`
}
type SpellAcquisition struct {
	Origin   string  `json:"origin,omitempty"`
	ID       string  `json:"id"`
	ClassID  string  `json:"classId"`
	SpellID  string  `json:"spellId"`
	Level    int     `json:"level"`
	At       string  `json:"at"`
	CostGP   float64 `json:"costGp"`
	ScrollID string  `json:"scrollId,omitempty"`
}
type Build struct {
	Method     string            `json:"method"`
	BaseScores map[string]int    `json:"baseScores"`
	Rolls      []Roll            `json:"rolls"`
	Species    string            `json:"species"`
	Lineage    string            `json:"lineage"`
	Background string            `json:"background"`
	Levels     []Level           `json:"levels"`
	Subclasses map[string]string `json:"subclasses"`
	Choices    []Choice          `json:"choices"`
	Spells     SpellDecisions    `json:"spells"`
}
type Item struct {
	ID          string     `json:"id"`
	Reference   *Reference `json:"reference,omitempty"`
	SpellID     string     `json:"spellId,omitempty"`
	Name        string     `json:"name"`
	Quantity    int        `json:"quantity"`
	Location    string     `json:"location"`
	Attuned     bool       `json:"attuned"`
	Acquisition string     `json:"acquisition"`
	GrantID     string     `json:"grantId,omitempty"`
	Notes       string     `json:"notes"`
}
type Play struct {
	Rolls          []PlayRoll          `json:"rolls"`
	HP             int                 `json:"hp"`
	TemporaryHP    int                 `json:"temporaryHp"`
	Inventory      []Item              `json:"inventory"`
	Currency       map[string]float64  `json:"currency"`
	ResourceUses   map[string]int      `json:"resourceUses"`
	ActiveFeatures map[string]bool     `json:"activeFeatures"`
	PreparedSpells map[string][]string `json:"preparedSpells"`
	AsOf           string              `json:"asOf"`
}
type PlayRoll struct {
	Origin   string `json:"origin,omitempty"`
	ID       string `json:"id"`
	Resource string `json:"resource"`
	Die      int    `json:"die"`
	Result   int    `json:"result"`
	At       string `json:"at"`
}
type Effect struct {
	Target string `json:"target"`
	Key    string `json:"key,omitempty"`
	Mode   string `json:"mode"`
	Value  int    `json:"value"`
}
type Grant struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Reason         string     `json:"reason"`
	ActorID        string     `json:"actorId"`
	GrantedAt      string     `json:"grantedAt"`
	Active         bool       `json:"active"`
	EffectiveLevel int        `json:"effectiveLevel"`
	ExpiresAt      string     `json:"expiresAt,omitempty"`
	ItemID         string     `json:"itemId,omitempty"`
	Condition      string     `json:"condition"`
	Feat           *Reference `json:"feat,omitempty"`
	Effects        []Effect   `json:"effects"`
	Waivers        []string   `json:"waivers"`
}
type Inputs struct {
	Build  Build   `json:"build"`
	Play   Play    `json:"play"`
	Grants []Grant `json:"grants"`
	Notes  string  `json:"notes"`
}
type Issue struct {
	ID        string     `json:"id"`
	Target    string     `json:"target"`
	Message   string     `json:"message"`
	Severity  string     `json:"severity"`
	Reference *Reference `json:"reference,omitempty"`
}
type Term struct {
	Label   string     `json:"label"`
	Value   any        `json:"value"`
	Source  *Reference `json:"source,omitempty"`
	GrantID string     `json:"grantId,omitempty"`
	Status  string     `json:"status,omitempty"`
}
type Explanation struct {
	Label   string      `json:"label"`
	Formula string      `json:"formula"`
	Value   any         `json:"value"`
	Unit    string      `json:"unit,omitempty"`
	Minimum *int        `json:"minimum,omitempty"`
	Maximum *int        `json:"maximum,omitempty"`
	Terms   []Term      `json:"terms"`
	Sources []Reference `json:"sources"`
}
type Evidence struct {
	Reference         Reference      `json:"reference"`
	Name              string         `json:"name"`
	Book              string         `json:"book,omitempty"`
	Hash              string         `json:"hash"`
	Summary           string         `json:"summary"`
	Facts             map[string]any `json:"facts"`
	PackageID         string         `json:"packageId,omitempty"`
	PackageGeneration string         `json:"packageGeneration,omitempty"`
	ContentRevision   string         `json:"contentRevision,omitempty"`
}
type Result struct {
	ContractVersion string                 `json:"contractVersion"`
	Inputs          Inputs                 `json:"inputs"`
	Sheet           map[string]any         `json:"sheet"`
	Guidance        map[string]any         `json:"guidance"`
	Plan            map[string]any         `json:"plan"`
	SpellOptions    map[string]any         `json:"spellOptions"`
	Explanations    map[string]Explanation `json:"explanations"`
	Evidence        []Evidence             `json:"evidence"`
	Issues          []Issue                `json:"issues"`
	Ready           bool                   `json:"ready"`
}

func Blank() Inputs {
	return Inputs{Build: Build{Method: "point-buy", BaseScores: map[string]int{}, Rolls: []Roll{}, Levels: []Level{}, Subclasses: map[string]string{}, Choices: []Choice{}, Spells: SpellDecisions{Cantrips: map[string][]string{}, Spellbook: map[string][]string{}, GrantChoices: map[string][]string{}, CastingAbilities: map[string]string{}, Swaps: []SpellSwap{}, Acquisitions: []SpellAcquisition{}}}, Play: Play{Rolls: []PlayRoll{}, Inventory: []Item{}, Currency: map[string]float64{}, ResourceUses: map[string]int{}, ActiveFeatures: map[string]bool{}, PreparedSpells: map[string][]string{}}, Grants: []Grant{}}
}
