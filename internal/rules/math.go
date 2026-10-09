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

func ClampHP(hitPoints, maximum float64) float64 {
	if maximum > 0 {
		return math.Max(0, math.Min(maximum, hitPoints))
	}
	return math.Max(0, hitPoints)
}
