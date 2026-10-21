package rules

import (
	"strings"
	"unicode/utf8"

	"github.com/pjunak/addon-dnd-engine/character"
)

const maximumContainers = 500
const maximumContainerName = 120

// Containers organize inventory; they grant no items or mechanics. A linked
// item can give capacity (see characterCarrying). Membership is flat and
// independent of carried/stored location.
func validateCharacterStorage(input character.Inputs, result *character.Result) {
	containers := map[string]bool{}
	for _, container := range input.Play.Containers {
		if strings.TrimSpace(container.ID) == "" || containers[container.ID] {
			addCharacterIssue(result, "container-id:"+container.ID, "inventory", "Containers need distinct nonempty IDs.", character.SeverityBlocker, nil)
		}
		if strings.TrimSpace(container.Name) == "" || utf8.RuneCountInString(container.Name) > maximumContainerName {
			addCharacterIssue(result, "container-name:"+container.ID, "inventory", "Give each container a name of at most 120 characters.", character.SeverityBlocker, nil)
		}
		containers[container.ID] = true
	}
	if len(input.Play.Containers) > maximumContainers {
		addCharacterIssue(result, "container-limit", "inventory", "Containers exceed the character input limit.", character.SeverityBlocker, nil)
	}
	for _, item := range input.Play.Inventory {
		if item.ContainerID == "" {
			continue
		}
		if !containers[item.ContainerID] {
			addCharacterIssue(result, "container-missing:"+item.ID, "inventory", "Choose an existing container or remove this item's container assignment.", character.SeverityBlocker, nil)
		}
		if item.Location != "carried" && item.Location != "stored" {
			addCharacterIssue(result, "container-location:"+item.ID, "inventory", "Only carried or stored items can be assigned to a container. Remove the assignment before equipping.", character.SeverityBlocker, nil)
		}
	}
}

func characterStorage(input character.Inputs, result *character.Result) {
	containers := []character.StorageContainer{}
	for _, container := range input.Play.Containers {
		members := []string{}
		for _, item := range input.Play.Inventory {
			if item.ContainerID == container.ID {
				members = append(members, item.ID)
			}
		}
		containers = append(containers, character.StorageContainer{ID: container.ID, Name: container.Name, ItemIDs: members})
	}
	result.Sheet.Storage = character.SheetStorage{Containers: containers}
	result.Guidance.Storage = character.StorageGuidance{MaximumContainers: maximumContainers, MaximumNameLength: maximumContainerName}
	result.Explanations["storage"] = character.Explanation{
		Label: "Storage", Formula: "Named containers group existing inventory instances without creating items or enforcing capacity. Membership does not change carried/stored location, quantity, attunement or quick-use availability. Removing a container requires explicitly unassigning its contents.",
		Value: containers, Terms: []character.Term{}, Sources: []character.Reference{},
	}
}
