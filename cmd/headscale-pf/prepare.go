package main

import (
	"fmt"

	"github.com/yousysadmin/headscale-pf/internal/models"
	"github.com/yousysadmin/headscale-pf/internal/policy"
	"github.com/yousysadmin/headscale-pf/internal/sources"
)

func preparePolicy(client sources.Source, logCh chan<- string) error {
	hsPolicy := policy.Policy{}

	// Reject an unknown output format before contacting the source.
	if !policy.IsValidFormat(outputFormat) {
		return fmt.Errorf("invalid output format %q: must be %q, %q, or %q", outputFormat, policy.FormatAuto, policy.FormatHJSON, policy.FormatJSON)
	}

	// Read policy template
	logCh <- fmt.Sprintf("Read policy template from: %s", inputPolicyFile)
	err := hsPolicy.ReadPolicyFromFile(inputPolicyFile)
	if err != nil {
		return err
	}

	// Get group names from policy file
	groups := hsPolicy.GetGroupNames()
	if len(groups) <= 0 {
		return fmt.Errorf("no groups found in the policy template")
	}

	// Get groups and group members
	var groupsInfo []*models.Group
	for _, g := range groups {
		// Get group info
		// If group doesn't find, returns nil
		group, err := client.GetGroupByName(g)
		if err != nil {
			return err
		}

		// If a group is found in source, try to get a members
		if group != nil {
			// Sources may populate Users during GetGroupByName (one round-trip).
			// A non-nil Users slice — even if empty — means "already loaded".
			if group.Users == nil {
				users, err := client.GetGroupMembers(group.ID)
				if err != nil {
					return err
				}
				group.Users = users
			}
			groupsInfo = append(groupsInfo, group)

			logCh <- fmt.Sprintf("Collect %d members for group: %s", len(group.Users), g)
		} else {
			logCh <- fmt.Sprintf("Group '%s' not found", g)
		}
	}

	// filling user groups
	hsGroups := map[string][]string{}
	for _, g := range groupsInfo {
		var upg []string
		for _, u := range g.Users {
			upg = append(upg, u.Username)
		}

		// Add the prefix 'group' to a group name
		groupName := fmt.Sprintf("group:%s", g.Name)

		hsGroups[groupName] = upg
	}
	hsPolicy.AppendGroups(hsGroups)

	// Write a prepared policy on a file. Resolve auto → concrete now so the log
	// reflects the format actually written.
	format := hsPolicy.ResolveFormat(outputFormat)
	logCh <- fmt.Sprintf("Write policy (%s) to: %s", format, outputPolicyFile)
	err = hsPolicy.WritePolicyToFile(outputPolicyFile, format)
	if err != nil {
		return err
	}

	logCh <- "Done"

	return nil
}
