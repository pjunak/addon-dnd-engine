package rules

import (
	"encoding/json"
	"github.com/pjunak/addon-dnd-engine/character"
	"reflect"
	"testing"
)

func TestInspirationIsDetachedAuthoredState(t *testing.T) {
	for _, name := range []string{"absent", "available", "spent"} {
		t.Run(name, func(t *testing.T) {
			input, records, profile := characterFixture(t)
			if name != "absent" {
				value := name == "available"
				input.Play.Inspiration = &value
			}
			before, _ := json.Marshal(input)
			result := EvaluateCharacter(input, records, profile)
			if !result.Ready || result.Sheet["inspiration"] != (name == "available") || object(result.Guidance["authoredPlay"])["inspiration"] != true || result.Explanations["inspiration"].Value != result.Sheet["inspiration"] {
				t.Fatalf("missing authored state or guidance: %+v", result)
			}
			if !reflect.DeepEqual(result.Inputs, input) {
				t.Fatal("evaluation changed authored input")
			}
			for _, change := range []Object{{"operation": "damage", "amount": float64(1)}, {"operation": "rest", "rest": "short"}, {"operation": "rest", "rest": "long"}} {
				played, err := ApplyCharacterPlay(input, change, records, profile)
				if err != nil || !reflect.DeepEqual(played.Inputs.Play.Inspiration, input.Play.Inspiration) {
					t.Fatalf("play changed inspiration: %v %v", change, err)
				}
			}
			if result.Inputs.Play.Inspiration != nil {
				*result.Inputs.Play.Inspiration = !*result.Inputs.Play.Inspiration
			}
			after, _ := json.Marshal(input)
			if string(before) != string(after) {
				t.Fatal("returned inputs alias caller state")
			}
		})
	}
	input := character.Blank()
	available := true
	input.Play.Inspiration = &available
	_, records, profile := characterFixture(t)
	result := EvaluateCharacter(input, records, profile)
	if result.Ready || result.Sheet["status"] != "needs-choices" || result.Sheet["inspiration"] != true || object(result.Guidance["authoredPlay"])["inspiration"] != true {
		t.Fatal("incomplete characters lost authored Inspiration")
	}
}
