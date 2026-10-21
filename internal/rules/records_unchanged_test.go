package rules

import (
	"fmt"

	"github.com/pjunak/addon-dnd-engine/character"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

var changedRecords = map[string]bool{}

// Every evaluation in this package checks that the shared, read-only records
// still match their source, and that its result matches the published schema.
func TestMain(m *testing.M) {
	previous, check := checkResult, checkResultSchema(loadResultSchema())
	checkResult = func(result character.Result) {
		if previous != nil {
			previous(result)
		}
		check(result)
	}
	verifyRecords = func(snapshot *characterRecordSnapshot) {
		for _, entry := range snapshot.decoded {
			original, _ := DecodeObject(entry.body)
			if !reflect.DeepEqual(original, entry.value) {
				changedRecords[text(entry.value["kind"])+":"+text(entry.value["id"])] = true
			}
		}
	}
	code := m.Run()
	if invalid := invalidResults(); len(invalid) > 0 {
		fmt.Fprintln(os.Stderr, "results do not match contracts/character-result.schema.json:", strings.Join(invalid, "; "))
		code = 1
	}
	if len(changedRecords) > 0 {
		keys := make([]string, 0, len(changedRecords))
		for key := range changedRecords {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		fmt.Fprintln(os.Stderr, "rules changed shared records:", strings.Join(keys, ", "))
		code = 1
	}
	os.Exit(code)
}
