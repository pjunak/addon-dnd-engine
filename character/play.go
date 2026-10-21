package character

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"slices"
	"unicode/utf8"
)

// PlayOperation names a character-play command.
type PlayOperation string

const (
	PlayRest                 PlayOperation = "rest"
	PlayToggleFeature        PlayOperation = "toggle-feature"
	PlaySelectSpell          PlayOperation = "select-spell"
	PlayCastSpell            PlayOperation = "cast-spell"
	PlaySelectGrantSpell     PlayOperation = "select-grant-spell"
	PlaySelectCastingAbility PlayOperation = "select-casting-ability"
	PlayCastGrantedSpell     PlayOperation = "cast-granted-spell"
	PlayCastRitual           PlayOperation = "cast-ritual"
	PlayCopySpell            PlayOperation = "copy-spell"
	PlaySwapSpell            PlayOperation = "swap-spell"
	PlayConsumeItem          PlayOperation = "consume-item"
	PlaySpendHitDie          PlayOperation = "spend-hit-die"
	PlaySetHP                PlayOperation = "set-hp"
	PlayHeal                 PlayOperation = "heal"
	PlayDamage               PlayOperation = "damage"
	PlaySetTemporaryHP       PlayOperation = "set-temporary-hp"
	PlayReplaceClassChoice   PlayOperation = "replace-class-choice"
	PlaySetHand              PlayOperation = "set-hand"
	PlaySetGrip              PlayOperation = "set-grip"
	PlayKeepConcentration    PlayOperation = "keep-concentration"
	PlayEndConcentration     PlayOperation = "end-concentration"
)

// playCommandFields lists each operation's fields, in the order they are
// checked. It is the only list of play operations.
var playCommandFields = map[PlayOperation][]string{
	PlayRest:                 {"rest"},
	PlayToggleFeature:        {"key", "enabled"},
	PlaySelectSpell:          {"classId", "ref", "selection", "selected"},
	PlayCastSpell:            {"classId", "ref", "slot"},
	PlaySelectGrantSpell:     {"key", "ref", "selected"},
	PlaySelectCastingAbility: {"key", "ability"},
	PlayCastGrantedSpell:     {"key", "slot"},
	PlayCastRitual:           {"classId", "ref"},
	PlayCopySpell:            {"classId", "ref", "scrollId", "acquisitionId"},
	PlaySwapSpell:            {"classId", "out", "ref"},
	PlayConsumeItem:          {"itemId"},
	PlaySpendHitDie:          {"key", "result", "rollId"},
	PlaySetHP:                {"amount"},
	PlayHeal:                 {"amount"},
	PlayDamage:               {"amount"},
	PlaySetTemporaryHP:       {"amount"},
	PlayReplaceClassChoice:   {"kind", "key", "out", "ref"},
	PlaySetHand:              {"hand", "itemId"},
	PlaySetGrip:              {"grip"},
	PlayKeepConcentration:    {},
	PlayEndConcentration:     {},
}

func (PlayOperation) EnumValues() []string {
	result := make([]string, 0, len(playCommandFields))
	for operation := range playCommandFields {
		result = append(result, string(operation))
	}
	slices.Sort(result)
	return result
}

// PlayCommand is one character-play change. Operation selects the fields it
// carries; the others stay zero and are never encoded.
type PlayCommand struct {
	Operation     PlayOperation   `json:"operation"`
	Rest          RestKind        `json:"rest,omitempty"`
	Key           string          `json:"key,omitempty"`
	Enabled       bool            `json:"enabled,omitempty"`
	ClassID       string          `json:"classId,omitempty"`
	Ref           string          `json:"ref,omitempty"`
	Selection     SpellList       `json:"selection,omitempty"`
	Selected      bool            `json:"selected,omitempty"`
	Slot          string          `json:"slot,omitempty"`
	Ability       string          `json:"ability,omitempty"`
	ScrollID      string          `json:"scrollId,omitempty"`
	AcquisitionID string          `json:"acquisitionId,omitempty"`
	Out           string          `json:"out,omitempty"`
	ItemID        string          `json:"itemId,omitempty"`
	Hand          Hand            `json:"hand,omitempty"`
	Grip          Grip            `json:"grip,omitempty"`
	Amount        int             `json:"amount,omitempty"`
	Result        int             `json:"result,omitempty"`
	RollID        string          `json:"rollId,omitempty"`
	Kind          ReplacementKind `json:"kind,omitempty"`
}

func (PlayCommand) UnionVariants() (string, map[string][]string) {
	variants := make(map[string][]string, len(playCommandFields))
	for operation, fields := range playCommandFields {
		variants[string(operation)] = fields
	}
	return "operation", variants
}

// playStringsMayBeEmpty are the string fields where "" is a meaning: no slot,
// no scroll, or a free hand.
var playStringsMayBeEmpty = []string{"slot", "scrollId", "itemId"}

// enumMessages explain an invalid closed value of a command field.
var enumMessages = map[string]string{
	"rest": "Choose a short or long rest.", "selection": "Choose a spell selection list.",
	"hand": "Choose a hand and one owned item, or leave it free.", "grip": "Choose a supported hand grip.",
	"kind": "Choose a feat or spell replacement.",
}

// DecodePlayCommand reads a command with exactly its operation's fields. Its
// errors are user-facing sentences.
func DecodePlayCommand(body []byte) (PlayCommand, error) {
	var raw map[string]json.RawMessage
	if json.Unmarshal(body, &raw) != nil || raw == nil {
		return PlayCommand{}, fmt.Errorf("The play change has invalid fields.")
	}
	var operation PlayOperation
	_ = json.Unmarshal(raw["operation"], &operation)
	fields, known := playCommandFields[operation]
	if !known || len(raw) != len(fields)+1 {
		return PlayCommand{}, fmt.Errorf("The play change has invalid fields.")
	}
	command := PlayCommand{Operation: operation}
	target := reflect.ValueOf(&command).Elem()
	for _, name := range fields {
		value, exists := raw[name]
		if !exists {
			return PlayCommand{}, fmt.Errorf("The play change is missing %s.", name)
		}
		destination := commandField(target, name)
		switch destination.Kind() {
		case reflect.Bool:
			var flag bool
			if json.Unmarshal(value, &flag) != nil {
				return PlayCommand{}, fmt.Errorf("%s must be a boolean.", name)
			}
			destination.SetBool(flag)
		case reflect.Int:
			var number float64
			if json.Unmarshal(value, &number) != nil || number != math.Trunc(number) || math.Abs(number) > 1_000_000_000 {
				if name == "amount" {
					return PlayCommand{}, fmt.Errorf("Enter a non-negative whole amount.")
				}
				return PlayCommand{}, fmt.Errorf("Record a valid result for an available hit die.")
			}
			destination.SetInt(int64(number))
		case reflect.String:
			var text string
			if json.Unmarshal(value, &text) != nil || utf8.RuneCountInString(text) > 200 || text == "" && !slices.Contains(playStringsMayBeEmpty, name) {
				return PlayCommand{}, fmt.Errorf("%s must be a valid string.", name)
			}
			destination.SetString(text)
			if closed, ok := destination.Interface().(interface{ Valid() bool }); ok && !closed.Valid() {
				return PlayCommand{}, fmt.Errorf("%s", enumMessages[name])
			}
		}
	}
	return command, nil
}

func commandField(command reflect.Value, name string) reflect.Value {
	for index := range command.NumField() {
		if jsonName(command.Type().Field(index)) == name {
			return command.Field(index)
		}
	}
	panic("play command field " + name + " is not declared")
}

func (command *PlayCommand) UnmarshalJSON(body []byte) error {
	decoded, err := DecodePlayCommand(body)
	if err != nil {
		return err
	}
	*command = decoded
	return nil
}

// MarshalJSON writes the operation and exactly its fields.
func (command PlayCommand) MarshalJSON() ([]byte, error) {
	fields, known := playCommandFields[command.Operation]
	if !known {
		return nil, fmt.Errorf("unknown play operation %q", command.Operation)
	}
	value := reflect.ValueOf(command)
	var buffer bytes.Buffer
	buffer.WriteString(`{"operation":`)
	operation, _ := json.Marshal(command.Operation)
	buffer.Write(operation)
	for _, name := range fields {
		encoded, err := json.Marshal(commandField(value, name).Interface())
		if err != nil {
			return nil, err
		}
		key, _ := json.Marshal(name)
		buffer.WriteByte(',')
		buffer.Write(key)
		buffer.WriteByte(':')
		buffer.Write(encoded)
	}
	buffer.WriteByte('}')
	return buffer.Bytes(), nil
}
