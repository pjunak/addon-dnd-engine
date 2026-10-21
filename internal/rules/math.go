// Package rules contains deterministic, host-free D&D calculations. Edition
// policy is always supplied by a validated rules-data profile.
package rules

import (
	"math"
)

var Abilities = [...]string{"STR", "DEX", "CON", "INT", "WIS", "CHA"}

func AbilityModifier(score float64) int {
	return int(math.Floor((score - 10) / 2))
}
