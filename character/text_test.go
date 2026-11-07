package character

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"
)

func TestNewTextFillsItsTemplate(t *testing.T) {
	text := NewText("Copying this spell requires {0} GP.", 12.5)
	want := Text{Label: "Copying this spell requires 12.5 GP.", LabelKey: "Copying this spell requires {0} GP.", LabelArgs: []string{"12.5"}}
	if !reflect.DeepEqual(text, want) {
		t.Fatalf("NewText = %+v", text)
	}
	var refusal Refusal
	if err := Refuse("No uses remain for this resource."); !errors.As(err, &refusal) || err.Error() != "No uses remain for this resource." || refusal.Text.LabelKey != err.Error() {
		t.Fatalf("Refuse = %#v", err)
	}
}

// TestPlayCommandRefusalsAreCataloged covers the closed-set messages that
// DecodePlayCommand words through a map, which the rules' source scan cannot see.
func TestPlayCommandRefusalsAreCataloged(t *testing.T) {
	body, err := os.ReadFile("../locales/en.json")
	if err != nil {
		t.Fatal(err)
	}
	var catalog struct {
		Sections map[string]map[string]string `json:"sections"`
	}
	if err := json.Unmarshal(body, &catalog); err != nil {
		t.Fatal(err)
	}
	for _, message := range enumMessages {
		if catalog.Sections["messages"][message] != message {
			t.Errorf("locales/en.json messages lacks %q", message)
		}
	}
}
