package roles

import "github.com/gophercloud/gophercloud"

func listURL(client *gophercloud.ServiceClient) string {
	return client.ServiceURL("roles")
}

func getURL(client *gophercloud.ServiceClient, roleID string) string {
	return client.ServiceURL("roles", roleID)
}

func listAssignmentsURL(client *gophercloud.ServiceClient) string {
	return client.ServiceURL("role_assignments")
}
