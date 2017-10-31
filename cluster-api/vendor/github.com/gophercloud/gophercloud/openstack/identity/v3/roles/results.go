package roles

import (
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/pagination"
)

// Role grants permissions to a user.
type Role struct {
	// DomainID is the domain ID the role belongs to.
	DomainID string `json:"domain_id"`

	// ID is the unique ID of the role.
	ID string `json:"id"`

	// Links contains referencing links to the role.
	Links map[string]interface{} `json:"links"`

	// Name is the role name
	Name string `json:"name"`
}

type roleResult struct {
	gophercloud.Result
}

// GetResult is the response from a Get operation. Call its Extract method
// to interpret it as a Role.
type GetResult struct {
	roleResult
}

// RolePage is a single page of Role results.
type RolePage struct {
	pagination.LinkedPageBase
}

// IsEmpty determines whether or not a page of Roles contains any results.
func (r RolePage) IsEmpty() (bool, error) {
	roles, err := ExtractRoles(r)
	return len(roles) == 0, err
}

// NextPageURL extracts the "next" link from the links section of the result.
func (r RolePage) NextPageURL() (string, error) {
	var s struct {
		Links struct {
			Next     string `json:"next"`
			Previous string `json:"previous"`
		} `json:"links"`
	}
	err := r.ExtractInto(&s)
	if err != nil {
		return "", err
	}
	return s.Links.Next, err
}

// ExtractProjects returns a slice of Roles contained in a single page of
// results.
func ExtractRoles(r pagination.Page) ([]Role, error) {
	var s struct {
		Roles []Role `json:"roles"`
	}
	err := (r.(RolePage)).ExtractInto(&s)
	return s.Roles, err
}

// Extract interprets any roleResults as a Role.
func (r roleResult) Extract() (*Role, error) {
	var s struct {
		Role *Role `json:"role"`
	}
	err := r.ExtractInto(&s)
	return s.Role, err
}

// RoleAssignment is the result of a role assignments query.
type RoleAssignment struct {
	Role  AssignedRole `json:"role,omitempty"`
	Scope Scope        `json:"scope,omitempty"`
	User  User         `json:"user,omitempty"`
	Group Group        `json:"group,omitempty"`
}

// AssignedRole represents a Role in an assignment.
type AssignedRole struct {
	ID string `json:"id,omitempty"`
}

// Scope represents a scope in a Role assignment.
type Scope struct {
	Domain  Domain  `json:"domain,omitempty"`
	Project Project `json:"project,omitempty"`
}

// Domain represents a domain in a role assignment scope.
type Domain struct {
	ID string `json:"id,omitempty"`
}

// Project represents a project in a role assignment scope.
type Project struct {
	ID string `json:"id,omitempty"`
}

// User represents a user in a role assignment scope.
type User struct {
	ID string `json:"id,omitempty"`
}

// Group represents a group in a role assignment scope.
type Group struct {
	ID string `json:"id,omitempty"`
}

// RoleAssignmentPage is a single page of RoleAssignments results.
type RoleAssignmentPage struct {
	pagination.LinkedPageBase
}

// IsEmpty returns true if the RoleAssignmentPage contains no results.
func (r RoleAssignmentPage) IsEmpty() (bool, error) {
	roleAssignments, err := ExtractRoleAssignments(r)
	return len(roleAssignments) == 0, err
}

// NextPageURL uses the response's embedded link reference to navigate to
// the next page of results.
func (r RoleAssignmentPage) NextPageURL() (string, error) {
	var s struct {
		Links struct {
			Next string `json:"next"`
		} `json:"links"`
	}
	err := r.ExtractInto(&s)
	return s.Links.Next, err
}

// ExtractRoleAssignments extracts a slice of RoleAssignments from a Collection
// acquired from List.
func ExtractRoleAssignments(r pagination.Page) ([]RoleAssignment, error) {
	var s struct {
		RoleAssignments []RoleAssignment `json:"role_assignments"`
	}
	err := (r.(RoleAssignmentPage)).ExtractInto(&s)
	return s.RoleAssignments, err
}
