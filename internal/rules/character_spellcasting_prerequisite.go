package rules

// Eligibility is a lasting class/trait capability, independent of today's spent
// slots. Item spells never supply the prerequisite for attuning another item.
func hasIntrinsicSpellcasting(sheet Object) bool {
	casting := object(sheet["spellcasting"])
	for _, class := range objects(casting["perClass"]) {
		if integer(class["cantripsKnown"], 0) > 0 || integer(class["preparedLimit"], 0) > 0 && integer(class["maxSpellLevel"], 0) > 0 {
			return true
		}
	}
	for _, grant := range objects(casting["granted"]) {
		if !contains([]string{"class", "subclass", "species", "lineage", "background", "feat", "feature"}, text(object(grant["source"])["type"])) {
			continue
		}
		if text(grant["ref"]) != "" && (integer(grant["level"], -1) == 0 || integer(grant["level"], -1) > 0 && text(grant["free"]) != "") {
			return true
		}
	}
	return false
}
