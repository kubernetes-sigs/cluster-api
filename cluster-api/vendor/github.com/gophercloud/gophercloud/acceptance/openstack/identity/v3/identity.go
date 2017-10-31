package v3

import (
	"testing"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/acceptance/tools"
	"github.com/gophercloud/gophercloud/openstack/identity/v3/domains"
	"github.com/gophercloud/gophercloud/openstack/identity/v3/groups"
	"github.com/gophercloud/gophercloud/openstack/identity/v3/projects"
	"github.com/gophercloud/gophercloud/openstack/identity/v3/users"
)

// CreateProject will create a project with a random name.
// It takes an optional createOpts parameter since creating a project
// has so many options. An error will be returned if the project was
// unable to be created.
func CreateProject(t *testing.T, client *gophercloud.ServiceClient, c *projects.CreateOpts) (*projects.Project, error) {
	name := tools.RandomString("ACPTTEST", 8)
	t.Logf("Attempting to create project: %s", name)

	var createOpts projects.CreateOpts
	if c != nil {
		createOpts = *c
	} else {
		createOpts = projects.CreateOpts{}
	}

	createOpts.Name = name

	project, err := projects.Create(client, createOpts).Extract()
	if err != nil {
		return project, err
	}

	t.Logf("Successfully created project %s with ID %s", name, project.ID)

	return project, nil
}

// CreateUser will create a user with a random name.
// It takes an optional createOpts parameter since creating a user
// has so many options. An error will be returned if the user was
// unable to be created.
func CreateUser(t *testing.T, client *gophercloud.ServiceClient, c *users.CreateOpts) (*users.User, error) {
	name := tools.RandomString("ACPTTEST", 8)
	t.Logf("Attempting to create user: %s", name)

	var createOpts users.CreateOpts
	if c != nil {
		createOpts = *c
	} else {
		createOpts = users.CreateOpts{}
	}

	createOpts.Name = name

	user, err := users.Create(client, createOpts).Extract()
	if err != nil {
		return user, err
	}

	t.Logf("Successfully created user %s with ID %s", name, user.ID)

	return user, nil
}

// CreateGroup will create a group with a random name.
// It takes an optional createOpts parameter since creating a group
// has so many options. An error will be returned if the group was
// unable to be created.
func CreateGroup(t *testing.T, client *gophercloud.ServiceClient, c *groups.CreateOpts) (*groups.Group, error) {
	name := tools.RandomString("ACPTTEST", 8)
	t.Logf("Attempting to create group: %s", name)

	var createOpts groups.CreateOpts
	if c != nil {
		createOpts = *c
	} else {
		createOpts = groups.CreateOpts{}
	}

	createOpts.Name = name

	group, err := groups.Create(client, createOpts).Extract()
	if err != nil {
		return group, err
	}

	t.Logf("Successfully created group %s with ID %s", name, group.ID)

	return group, nil
}

// CreateDomain will create a domain with a random name.
// It takes an optional createOpts parameter since creating a domain
// has many options. An error will be returned if the domain was
// unable to be created.
func CreateDomain(t *testing.T, client *gophercloud.ServiceClient, c *domains.CreateOpts) (*domains.Domain, error) {
	name := tools.RandomString("ACPTTEST", 8)
	t.Logf("Attempting to create domain: %s", name)

	var createOpts domains.CreateOpts
	if c != nil {
		createOpts = *c
	} else {
		createOpts = domains.CreateOpts{}
	}

	createOpts.Name = name

	domain, err := domains.Create(client, createOpts).Extract()
	if err != nil {
		return domain, err
	}

	t.Logf("Successfully created domain %s with ID %s", name, domain.ID)

	return domain, nil
}

// DeleteProject will delete a project by ID. A fatal error will occur if
// the project ID failed to be deleted. This works best when using it as
// a deferred function.
func DeleteProject(t *testing.T, client *gophercloud.ServiceClient, projectID string) {
	err := projects.Delete(client, projectID).ExtractErr()
	if err != nil {
		t.Fatalf("Unable to delete project %s: %v", projectID, err)
	}

	t.Logf("Deleted project: %s", projectID)
}

// DeleteUser will delete a user by ID. A fatal error will occur if
// the user failed to be deleted. This works best when using it as
// a deferred function.
func DeleteUser(t *testing.T, client *gophercloud.ServiceClient, userID string) {
	err := users.Delete(client, userID).ExtractErr()
	if err != nil {
		t.Fatalf("Unable to delete user %s: %v", userID, err)
	}

	t.Logf("Deleted user: %s", userID)
}

// DeleteGroup will delete a group by ID. A fatal error will occur if
// the group failed to be deleted. This works best when using it as
// a deferred function.
func DeleteGroup(t *testing.T, client *gophercloud.ServiceClient, groupID string) {
	err := groups.Delete(client, groupID).ExtractErr()
	if err != nil {
		t.Fatalf("Unable to delete group %s: %v", groupID, err)
	}

	t.Logf("Deleted group: %s", groupID)
}

// DeleteDomain will delete a domain by ID. A fatal error will occur if
// the project ID failed to be deleted. This works best when using it as
// a deferred function.
func DeleteDomain(t *testing.T, client *gophercloud.ServiceClient, domainID string) {
	err := domains.Delete(client, domainID).ExtractErr()
	if err != nil {
		t.Fatalf("Unable to delete domain %s: %v", domainID, err)
	}

	t.Logf("Deleted domain: %s", domainID)
}
