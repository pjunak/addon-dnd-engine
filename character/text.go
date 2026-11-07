package character

import (
	"fmt"
	"strings"
)

// NewText is Engine wording: the English Label and its key in the Engine's
// message catalog, which is template with {0}-style placeholders that args
// fill. Record names and other data go in args, never in the template, so
// clients translate the wording without translating the data.
func NewText(template string, args ...any) Text {
	replacements, values := []string{}, make([]string, 0, len(args))
	for index, arg := range args {
		value := fmt.Sprint(arg)
		replacements = append(replacements, fmt.Sprintf("{%d}", index), value)
		values = append(values, value)
	}
	return Text{Label: strings.NewReplacer(replacements...).Replace(template), LabelKey: template, LabelArgs: values}
}

// Refusal is a request or change the rules refuse, in Engine wording.
type Refusal struct{ Text Text }

func (failure Refusal) Error() string { return failure.Text.Label }

// Refuse returns a Refusal worded like NewText.
func Refuse(template string, args ...any) error {
	return Refusal{Text: NewText(template, args...)}
}
