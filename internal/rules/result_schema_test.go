package rules

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/pjunak/addon-dnd-engine/character"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

var (
	resultSchemaErrors   = map[string]bool{}
	resultSchemaErrorsMu sync.Mutex
)

// loadResultSchema compiles the packaged inputs and result schemas as the
// host does.
func loadResultSchema() (inputs, result *jsonschema.Schema) {
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	for _, name := range []string{"character-inputs.schema.json", "character-result.schema.json"} {
		body, err := os.ReadFile(filepath.Join("..", "..", "contracts", name))
		if err != nil {
			panic(err)
		}
		value, err := jsonschema.UnmarshalJSON(bytes.NewReader(body))
		if err != nil {
			panic(err)
		}
		if err := compiler.AddResource("https://engine.invalid/contracts/"+name, value); err != nil {
			panic(err)
		}
	}
	inputs, err := compiler.Compile("https://engine.invalid/contracts/character-inputs.schema.json")
	if err != nil {
		panic(err)
	}
	result, err = compiler.Compile("https://engine.invalid/contracts/character-result.schema.json")
	if err != nil {
		panic(err)
	}
	return inputs, result
}

// checkResultSchema validates a result the way the service sends it: a
// detached copy with every required list filled. Inputs the host would refuse
// never reach the rules, so their results are not checked.
func checkResultSchema(inputs, schema *jsonschema.Schema) func(character.Result) {
	return func(result character.Result) {
		body, _ := json.Marshal(result)
		var sent character.Result
		if err := json.Unmarshal(body, &sent); err != nil {
			record(err.Error())
			return
		}
		character.FillEmpty(&sent)
		request, _ := json.Marshal(sent.Inputs)
		if value, err := jsonschema.UnmarshalJSON(bytes.NewReader(request)); err != nil || inputs.Validate(value) != nil {
			return
		}
		body, _ = json.Marshal(sent)
		value, err := jsonschema.UnmarshalJSON(bytes.NewReader(body))
		if err == nil {
			err = schema.Validate(value)
		}
		if err != nil {
			record(err.Error())
		}
	}
}

func record(message string) {
	resultSchemaErrorsMu.Lock()
	defer resultSchemaErrorsMu.Unlock()
	resultSchemaErrors[message] = true
}

func invalidResults() []string {
	messages := make([]string, 0, len(resultSchemaErrors))
	for message := range resultSchemaErrors {
		messages = append(messages, message)
	}
	sort.Strings(messages)
	return messages
}
