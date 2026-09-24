package rules

func grantTotal(sources []grantSource, field string) int {
	total := 0
	for _, source := range sources {
		total += integer(source.Grants[field], 0)
	}
	return total
}

func armorGrantBonuses(sources []grantSource, armor Object) ([]any, int) {
	rows, total := []any{}, 0
	for _, source := range sources {
		for _, bonus := range objects(source.Grants["acBonuses"]) {
			applied := true
			if raw, exists := bonus["requires"]; exists {
				requirements := object(raw)
				types := stringsOf(requirements["armorTypes"])
				// Unsupported conditions must not turn a restricted bonus into an unconditional one.
				applied = len(requirements) == 1 && len(types) > 0 && contains(types, text(armor["armorType"]))
				for _, kind := range types {
					applied = applied && contains([]string{"light", "medium", "heavy"}, kind)
				}
			}
			value, status := integer(bonus["add"], 0), "inactive"
			if applied {
				total += value
				status = "applied"
			}
			rows = append(rows, Object{"name": firstText(source.Record["name"], source.Source["id"]),
				"source": source.Source, "value": value, "status": status, "requires": bonus["requires"]})
		}
	}
	return rows, total
}
