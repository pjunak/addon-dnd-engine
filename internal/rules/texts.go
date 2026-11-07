package rules

import "github.com/pjunak/addon-dnd-engine/character"

// engineText is Engine wording with its catalog key; see character.NewText.
func engineText(template string, args ...any) character.Text {
	return character.NewText(template, args...)
}

// plainText is text the Engine repeats rather than words, such as a record
// name or a rules-data warning; it has no catalog key.
func plainText(text string) character.Text {
	return character.Text{Label: text, LabelArgs: []string{}}
}

// optionalArgs keeps optional placeholder values absent rather than empty.
func optionalArgs(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return values
}

func newIssue(id, target string, message character.Text, severity character.IssueSeverity, reference *character.Reference) character.Issue {
	return character.Issue{ID: id, Target: target, Message: message.Label, MessageKey: message.LabelKey, MessageArgs: optionalArgs(message.LabelArgs), Severity: severity, Reference: reference}
}

// term is an explanation term labelled with label.
func term(label character.Text, value any) character.Term {
	return character.Term{Label: label.Label, LabelKey: label.LabelKey, LabelArgs: optionalArgs(label.LabelArgs), Value: value}
}

func explanation(label, formula character.Text, value any, terms []character.Term, sources []character.Reference) character.Explanation {
	return character.Explanation{
		Label: label.Label, LabelKey: label.LabelKey, LabelArgs: optionalArgs(label.LabelArgs),
		Formula: formula.Label, FormulaKey: formula.LabelKey, FormulaArgs: optionalArgs(formula.LabelArgs),
		Value: value, Terms: terms, Sources: sources,
	}
}

// refuse returns a refusal in Engine wording; see character.Refuse.
func refuse(template string, args ...any) error {
	return character.Refuse(template, args...)
}
