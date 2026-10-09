package rules

import (
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

var changedRecords = map[string]bool{}

// Every evaluation in this package checks that the shared, read-only records
// still match their source.
func TestMain(m *testing.M) {
	verifyRecords = func(snapshot *characterRecordSnapshot) {
		for _, entry := range snapshot.decoded {
			original, _ := DecodeObject(entry.body)
			if !reflect.DeepEqual(original, entry.value) {
				changedRecords[text(entry.value["kind"])+":"+text(entry.value["id"])] = true
			}
		}
	}
	code := m.Run()
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
