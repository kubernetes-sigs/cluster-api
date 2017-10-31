package testing

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/gophercloud/gophercloud/openstack/identity/v3/roles"
	th "github.com/gophercloud/gophercloud/testhelper"
	"github.com/gophercloud/gophercloud/testhelper/client"
)

// ListOutput provides a single page of Role results.
const ListOutput = `
{
    "links": {
        "next": null,
        "previous": null,
        "self": "http://example.com/identity/v3/roles"
    },
    "roles": [
        {
            "domain_id": "default",
            "id": "2844b2a08be147a08ef58317d6471f1f",
            "links": {
                "self": "http://example.com/identity/v3/roles/2844b2a08be147a08ef58317d6471f1f"
            },
            "name": "admin-read-only"
        },
        {
            "domain_id": "1789d1",
            "id": "9fe1d3",
            "links": {
                "self": "https://example.com/identity/v3/roles/9fe1d3"
            },
            "name": "support"
        }
    ]
}
`

// GetOutput provides a Get result.
const GetOutput = `
{
    "role": {
        "domain_id": "1789d1",
        "id": "9fe1d3",
        "links": {
            "self": "https://example.com/identity/v3/roles/9fe1d3"
        },
        "name": "support"
    }
}
`

// FirstRole is the first role in the List request.
var FirstRole = roles.Role{
	DomainID: "default",
	ID:       "2844b2a08be147a08ef58317d6471f1f",
	Links: map[string]interface{}{
		"self": "http://example.com/identity/v3/roles/2844b2a08be147a08ef58317d6471f1f",
	},
	Name: "admin-read-only",
}

// SecondRole is the second role in the List request.
var SecondRole = roles.Role{
	DomainID: "1789d1",
	ID:       "9fe1d3",
	Links: map[string]interface{}{
		"self": "https://example.com/identity/v3/roles/9fe1d3",
	},
	Name: "support",
}

// ExpectedRolesSlice is the slice of roles expected to be returned from ListOutput.
var ExpectedRolesSlice = []roles.Role{FirstRole, SecondRole}

// HandleListRolesSuccessfully creates an HTTP handler at `/roles` on the
// test handler mux that responds with a list of two roles.
func HandleListRolesSuccessfully(t *testing.T) {
	th.Mux.HandleFunc("/roles", func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "GET")
		th.TestHeader(t, r, "Accept", "application/json")
		th.TestHeader(t, r, "X-Auth-Token", client.TokenID)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, ListOutput)
	})
}

// HandleGetRoleSuccessfully creates an HTTP handler at `/roles` on the
// test handler mux that responds with a single role.
func HandleGetRoleSuccessfully(t *testing.T) {
	th.Mux.HandleFunc("/roles/9fe1d3", func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "GET")
		th.TestHeader(t, r, "Accept", "application/json")
		th.TestHeader(t, r, "X-Auth-Token", client.TokenID)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, GetOutput)
	})
}
