package character

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPackagedCharacterSchemasMatchPublicTypes(t *testing.T) {
	for name, value := range map[string]any{"character-inputs.schema.json": Inputs{}, "character-result.schema.json": Result{}} {
		t.Run(name, func(t *testing.T) {
			body, err := os.ReadFile(filepath.Join("..", "contracts", name))
			if err != nil {
				t.Fatal(err)
			}
			var packaged, expected any
			if err := json.Unmarshal(body, &packaged); err != nil {
				t.Fatal(err)
			}
			generated, err := json.Marshal(Schema(value))
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(generated, &expected); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(packaged, expected) {
				t.Fatal("packaged wire contract is stale; run go run ./cmd/character-contract before building")
			}
		})
	}
}
