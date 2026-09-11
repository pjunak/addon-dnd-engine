package main

import (
	"encoding/json"
	"github.com/pjunak/addon-dnd-engine/character"
	"os"
	"path/filepath"
)

func main() {
	output := "contracts"
	if len(os.Args) > 1 {
		output = os.Args[1]
	}
	write := func(name string, value any) {
		body, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			panic(err)
		}
		if err = os.WriteFile(filepath.Join(output, name), append(body, '\n'), 0644); err != nil {
			panic(err)
		}
	}
	write("character-inputs.schema.json", character.Schema(character.Inputs{}))
	write("character-result.schema.json", character.Schema(character.Result{}))
	request := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"contractVersion", "inputs"}, "properties": map[string]any{"contractVersion": map[string]any{"const": character.ContractVersion}, "inputs": map[string]any{"$ref": "character-inputs.schema.json"}, "change": map[string]any{"type": "object", "maxProperties": 20}}}
	write("character.request.schema.json", request)
	write("character.response.schema.json", map[string]any{"type": "object", "additionalProperties": false, "required": []string{"contractVersion", "identity", "evaluation", "policy"}, "properties": map[string]any{"contractVersion": map[string]any{"const": "rules-character-response.v1"}, "identity": map[string]any{"$ref": "provider-identity.schema.json#/$defs/identity"}, "evaluation": map[string]any{"$ref": "character-result.schema.json"}, "policy": map[string]any{"type": []string{"object", "null"}}}})
}
