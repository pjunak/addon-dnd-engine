package character

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// AcquisitionOwner scopes an engine-owned decision or resource to one acquisition.
func AcquisitionOwner(owner, acquisition string) string {
	if acquisition == "" {
		return owner
	}
	return owner + "@" + url.QueryEscape(acquisition)
}

// RemapGrantReferences returns detached inputs with references to renamed DM
// grants updated together. The caller owns authorization and grant provenance;
// this function does not rename grants or change choice values, spent amounts or notes.
func RemapGrantReferences(input Inputs, ids map[string]string) (Inputs, error) {
	targets := map[string]bool{}
	for old, next := range ids {
		if old == "" || next == "" || targets[next] {
			return Inputs{}, errors.New("Imported grant identities must be distinct and non-empty.")
		}
		targets[next] = true
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return Inputs{}, err
	}
	var next Inputs
	if err := json.Unmarshal(encoded, &next); err != nil {
		return Inputs{}, err
	}
	rename := func(key string) (string, error) { return remapAcquisitionKey(key, ids, 0) }
	choices := map[struct {
		id   string
		slot int
	}]bool{}
	for i := range next.Build.Choices {
		choice := &next.Build.Choices[i]
		choice.ID, err = rename(choice.ID)
		if err != nil {
			return Inputs{}, err
		}
		key := struct {
			id   string
			slot int
		}{choice.ID, choice.Slot}
		if choices[key] {
			return Inputs{}, errors.New("Imported grant references would overlap. Review the character file before importing.")
		}
		choices[key] = true
	}
	if next.Build.Spells.GrantChoices, err = remapGrantMap(next.Build.Spells.GrantChoices, rename); err != nil {
		return Inputs{}, err
	}
	if next.Build.Spells.CastingAbilities, err = remapGrantMap(next.Build.Spells.CastingAbilities, rename); err != nil {
		return Inputs{}, err
	}
	if next.Play.ResourceUses, err = remapGrantMap(next.Play.ResourceUses, rename); err != nil {
		return Inputs{}, err
	}
	if next.Play.ActiveFeatures, err = remapGrantMap(next.Play.ActiveFeatures, rename); err != nil {
		return Inputs{}, err
	}
	for i := range next.Play.Inventory {
		if id, ok := ids[next.Play.Inventory[i].GrantID]; ok {
			next.Play.Inventory[i].GrantID = id
		}
	}
	for i := range next.Play.Rolls {
		next.Play.Rolls[i].Resource, err = rename(next.Play.Rolls[i].Resource)
		if err != nil {
			return Inputs{}, err
		}
	}
	for i := range next.Grants {
		grant := &next.Grants[i]
		for j := range grant.Effects {
			if grant.Effects[j].Target != "resourceMax" {
				continue
			}
			grant.Effects[j].Key, err = rename(grant.Effects[j].Key)
			if err != nil {
				return Inputs{}, err
			}
		}
		for j := range grant.Waivers {
			grant.Waivers[j], err = rename(grant.Waivers[j])
			if err != nil {
				return Inputs{}, err
			}
		}
	}
	return next, nil
}

func remapGrantMap[T any](input map[string]T, rename func(string) (string, error)) (map[string]T, error) {
	if input == nil {
		return nil, nil
	}
	result := make(map[string]T, len(input))
	for key, value := range input {
		renamed, err := rename(key)
		if err != nil {
			return nil, err
		}
		if _, exists := result[renamed]; exists {
			return nil, errors.New("Imported grant references would overlap. Review the character file before importing.")
		}
		result[renamed] = value
	}
	return result, nil
}

func remapAcquisitionKey(key string, ids map[string]string, depth int) (string, error) {
	if depth > 32 {
		return "", errors.New("Imported grant references are nested too deeply.")
	}
	if strings.HasPrefix(key, "grant:") {
		if next, ok := ids[strings.TrimPrefix(key, "grant:")]; ok {
			return "grant:" + next, nil
		}
	}
	at := strings.IndexByte(key, '@')
	if at < 0 {
		return key, nil
	}
	tail := key[at+1:]
	end := strings.IndexByte(tail, ':')
	if end < 0 {
		end = len(tail)
	}
	acquisition, err := url.QueryUnescape(tail[:end])
	if err != nil {
		return "", fmt.Errorf("Invalid imported acquisition reference: %w", err)
	}
	// A feat can itself grant a feat. Its acquisition is the escaped parent
	// choice key, so rebinding follows the same bounded identity grammar.
	renamed, err := remapAcquisitionKey(acquisition, ids, depth+1)
	if err != nil {
		return "", err
	}
	if renamed == acquisition {
		return key, nil
	}
	return key[:at+1] + url.QueryEscape(renamed) + tail[end:], nil
}
