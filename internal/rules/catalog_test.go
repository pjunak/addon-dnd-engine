package rules

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// catalog reads a message catalog's messages by key across its sections.
func catalog(t *testing.T, locale string) map[string]string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "locales", locale+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Sections map[string]map[string]string `json:"sections"`
	}
	if err := json.Unmarshal(body, &file); err != nil {
		t.Fatal(err)
	}
	messages := map[string]string{}
	for name, section := range file.Sections {
		for key, message := range section {
			if _, duplicate := messages[key]; duplicate {
				t.Errorf("%s: key %q is in more than one section", locale, key)
			}
			if name != "manifest" {
				messages[key] = message
			}
		}
	}
	return messages
}

// sourceKeys lists the literal templates the Engine words: first arguments of
// engineText, refuse, keyed and the character package's NewText and Refuse,
// and the builder's guidance templates.
func sourceKeys(t *testing.T) []string {
	t.Helper()
	keys := map[string]bool{}
	literal := func(expression ast.Expr) (string, bool) {
		value, ok := expression.(*ast.BasicLit)
		if !ok || value.Kind != token.STRING {
			return "", false
		}
		text, err := strconv.Unquote(value.Value)
		return text, err == nil
	}
	for _, directory := range []string{".", filepath.Join("..", "..", "character"), filepath.Join("..", "engine")} {
		files, err := filepath.Glob(filepath.Join(directory, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range files {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			builder := filepath.Base(path) == "builder_guidance.go"
			ast.Inspect(file, func(node ast.Node) bool {
				switch value := node.(type) {
				case *ast.CallExpr:
					name := ""
					switch function := value.Fun.(type) {
					case *ast.Ident:
						name = function.Name
					case *ast.SelectorExpr:
						name = function.Sel.Name
					}
					switch {
					case slices.Contains([]string{"engineText", "refuse", "keyed", "NewText", "Refuse"}, name) && len(value.Args) > 0:
						if text, ok := literal(value.Args[0]); ok {
							keys[text] = true
						}
					case builder && name == "add" && len(value.Args) > 3:
						if text, ok := literal(value.Args[3]); ok {
							keys[text] = true
						}
					}
				case *ast.AssignStmt:
					if identifier, ok := value.Lhs[0].(*ast.Ident); builder && ok && identifier.Name == "label" {
						if text, ok := literal(value.Rhs[0]); ok {
							keys[text] = true
						}
					}
				}
				return true
			})
		}
	}
	result := make([]string, 0, len(keys))
	for key := range keys {
		result = append(result, key)
	}
	slices.Sort(result)
	return result
}

// TestEngineWordingIsCataloged keeps every text the Engine keys in its English
// catalog, worded as its key, and translated into Czech with the same
// placeholders. Keys built from closed sets are listed here.
func TestEngineWordingIsCataloged(t *testing.T) {
	english, czech := catalog(t, "en"), catalog(t, "cs")
	keys := sourceKeys(t)
	if len(keys) < 200 {
		t.Fatalf("found only %d keyed templates; the source scan is broken", len(keys))
	}
	for _, ability := range Abilities {
		for _, suffix := range []string{" score", " modifier", " saving throw", " must have a positive score and cap."} {
			keys = append(keys, ability+suffix)
		}
	}
	for skill := range SkillAbility {
		keys = append(keys, guidanceLabel(skill))
	}
	for _, spec := range choiceKindSpecs {
		if spec.heading != "" {
			keys = append(keys, spec.heading)
		}
	}
	for _, kind := range []string{"weapon", "armor"} {
		keys = append(keys, "Choose which "+kind+" {0} is.", "No "+kind+" in the rules data can be {0}.", "{0} cannot be that "+kind+"; choose one of its options.")
	}
	keys = append(keys, "Only one item may occupy the armor slot.", "Only one item may occupy the shield slot.",
		"Choose an available species.", "Choose an available background.", "Range", "Thrown range",
		"Tiny", "Small", "Medium", "Large", "Huge", "Gargantuan")
	placeholders := regexp.MustCompile(`\{\w+\}`)
	set := func(message string) string {
		found := placeholders.FindAllString(message, -1)
		slices.Sort(found)
		return strings.Join(slices.Compact(found), " ")
	}
	for _, key := range keys {
		if english[key] != key {
			t.Errorf("locales/en.json lacks %q", key)
		}
	}
	for key, message := range english {
		if translated, ok := czech[key]; !ok {
			t.Errorf("locales/cs.json lacks %q", key)
		} else if set(translated) != set(message) {
			t.Errorf("Czech changes the placeholders of %q", key)
		}
	}
}
