// Package rules contains deterministic, host-free D&D calculations. Edition
// policy is always supplied by a validated rules-data profile.
package rules

import (
	"math"
	"strconv"
	"strings"
)

var Abilities = [...]string{"STR", "DEX", "CON", "INT", "WIS", "CHA"}

func AbilityModifier(score float64) int {
	return int(math.Floor((score - 10) / 2))
}

func ProficiencyBonus(totalLevel float64) int {
	level := math.Max(1, totalLevel)
	return 2 + int(math.Floor((level-1)/4))
}

func HitDieAverage(hitDie string) int {
	size, err := strconv.Atoi(strings.TrimPrefix(strings.ToLower(hitDie), "d"))
	if err != nil || size <= 0 {
		size = 8
	}
	return size/2 + 1
}

func ClampHP(hitPoints, maximum float64) float64 {
	if maximum > 0 {
		return math.Max(0, math.Min(maximum, hitPoints))
	}
	return math.Max(0, hitPoints)
}

func SaveDC(abilityScore, totalLevel float64) int {
	return 8 + ProficiencyBonus(totalLevel) + AbilityModifier(abilityScore)
}
