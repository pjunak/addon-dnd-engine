package character

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestPlayCommandsRoundTripWithExactlyTheirFields(t *testing.T) {
	for _, body := range []string{
		`{"operation":"rest","rest":"short"}`,
		`{"operation":"toggle-feature","key":"class:monk:focus","enabled":false}`,
		`{"operation":"cast-spell","classId":"wizard","ref":"spark","slot":""}`,
		`{"operation":"set-hand","hand":"off","itemId":""}`,
		`{"operation":"damage","amount":7}`,
		`{"operation":"end-concentration"}`,
	} {
		command, err := DecodePlayCommand([]byte(body))
		if err != nil {
			t.Fatal(body, err)
		}
		encoded, err := json.Marshal(command)
		if err != nil || string(encoded) != body {
			t.Fatalf("%s encoded as %s (%v)", body, encoded, err)
		}
	}
}

func TestPlayCommandsRejectWrongShapes(t *testing.T) {
	for body, message := range map[string]string{
		`{"operation":"rest"}`:                                        "invalid fields",
		`{"operation":"rest","rest":"short","key":"x"}`:               "invalid fields",
		`{"operation":"teleport"}`:                                    "invalid fields",
		`{"operation":"rest","rest":"week"}`:                          "short or long rest",
		`{"operation":"toggle-feature","key":"a","enabled":"1"}`:      "enabled must be a boolean",
		`{"operation":"heal","amount":2.5}`:                           "whole amount",
		`{"operation":"cast-ritual","classId":"","ref":"x"}`:          "classId must be a valid string",
		`{"operation":"set-grip","grip":"three"}`:                     "supported hand grip",
		`{"operation":"swap-spell","classId":"a","ref":"b","in":"c"}`: "missing out",
	} {
		if _, err := DecodePlayCommand([]byte(body)); err == nil || !strings.Contains(err.Error(), message) {
			t.Errorf("%s: %v, want %q", body, err, message)
		}
	}
}

func TestEveryPlayOperationIsAUnionVariant(t *testing.T) {
	_, variants := PlayCommand{}.UnionVariants()
	operations := PlayOperation("").EnumValues()
	if len(variants) != len(operations) || len(operations) != 21 {
		t.Fatalf("%d variants for %d operations", len(variants), len(operations))
	}
	for _, operation := range operations {
		for _, field := range variants[operation] {
			commandField(reflect.ValueOf(PlayCommand{}), field)
		}
	}
}

func TestFillEmptyReplacesRequiredNullsOnly(t *testing.T) {
	result := Result{Sheet: Sheet{Calculation: &Calculation{}}}
	FillEmpty(&result)
	body, _ := json.Marshal(result)
	text := string(body)
	if strings.Contains(text, "null") && !strings.Contains(text, `"armorRestrictions":null`) {
		t.Fatalf("required lists stayed null: %s", text)
	}
	if strings.Contains(text, `"selections"`) || strings.Contains(text, `"movement"`) {
		t.Fatal("optional lists were added", text)
	}
}

func TestSchemaAndTypeScriptAgreeOnOptionalAndNullable(t *testing.T) {
	schema := Schema(Attack{})
	if _, refers := schema["properties"].(map[string]any)["range"].(map[string]any)["$ref"]; !refers {
		t.Fatal("named structs are not shared definitions", schema["properties"])
	}
	required := map[string]bool{}
	for _, name := range schema["required"].([]string) {
		required[name] = true
	}
	if !required["versatileDamage"] || required["range"] || !required["properties"] {
		t.Fatalf("required = %v", schema["required"])
	}
	declarations := TypeScript(Attack{})
	for _, line := range []string{"versatileDamage: string | null;", "range?: WeaponRange;", "properties: Array<string>;", "grip?: Grip;"} {
		if !strings.Contains(declarations, line) {
			t.Fatalf("TypeScript lacks %q:\n%s", line, declarations)
		}
	}
	if !strings.Contains(TypeScript(PlayCommand{}), `| { operation: "rest"; rest: RestKind; }`) {
		t.Fatal("play commands are not a tagged union")
	}
}
