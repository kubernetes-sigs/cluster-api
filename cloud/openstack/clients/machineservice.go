/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package clients

import (
	"encoding/base64"
	"fmt"
	"github.com/golang/glog"
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/floatingips"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/keypairs"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/openstack/identity/v3/tokens"
	openstackconfigv1 "sigs.k8s.io/cluster-api/cloud/openstack/openstackproviderconfig/v1alpha1"
)

type InstanceService struct {
	provider       *gophercloud.ProviderClient
	computeClient  *gophercloud.ServiceClient
	identityClient *gophercloud.ServiceClient
}

type CloudConfig struct {
	AuthURL    string `json:"auth-url"`
	Username   string `json:"user-name"`
	UserID     string `json:"user-id"`
	Password   string `json:"password"`
	TenantID   string `json:"tenant-id"`
	TenantName string `json:"tenant-name"`
	TrustID    string `json:"trust-id"`
	DomainID   string `json:"domain-id"`
	DomainName string `json:"domain-name"`
	Region     string `json:"region"`
	CAFile     string `json:"ca-file"`
}

type Instance struct {
	servers.Server
}

type SshKeyPair struct {
	Name string `json:"name"`

	// PublicKey is the public key from this pair, in OpenSSH format.
	// "ssh-rsa AAAAB3Nz..."
	PublicKey string `json:"public_key"`

	// PrivateKey is the private key from this pair, in PEM format.
	// "-----BEGIN RSA PRIVATE KEY-----\nMIICXA..."
	// It is only present if this KeyPair was just returned from a Create call.
	PrivateKey string `json:"private_key"`
}

type InstanceListOpts struct {
	// Name of the image in URL format.
	Image string `q:"image"`

	// Name of the flavor in URL format.
	Flavor string `q:"flavor"`

	// Name of the server as a string; can be queried with regular expressions.
	// Realize that ?name=bob returns both bob and bobb. If you need to match bob
	// only, you can use a regular expression matching the syntax of the
	// underlying database server implemented for Compute.
	Name string `q:"name"`
}

func NewInstanceService(cfg *CloudConfig) (*InstanceService, error) {
	authUrl := gophercloud.NormalizeURL(cfg.AuthURL)
	opts := &gophercloud.AuthOptions{
		IdentityEndpoint: authUrl,
		Username:         cfg.Username,
		Password:         cfg.Password,
		DomainName:       cfg.DomainName,
		TenantID:         cfg.TenantID,
		AllowReauth:      true,
	}
	provider, err := openstack.AuthenticatedClient(*opts)
	if err != nil {
		return nil, fmt.Errorf("Create providerClient err: %v", err)
	}

	identityClient, err := openstack.NewIdentityV3(provider, gophercloud.EndpointOpts{
		Region: "",
	})
	if err != nil {
		return nil, fmt.Errorf("Create identityClient err: %v", err)
	}
	serverClient, err := openstack.NewComputeV2(provider, gophercloud.EndpointOpts{
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("Create serviceClient err: %v", err)
	}

	return &InstanceService{
		provider:       provider,
		identityClient: identityClient,
		computeClient:  serverClient,
	}, nil
}

// UpdateToken to update token if need.
func (is *InstanceService) UpdateToken() error {
	token := is.provider.Token()
	result, err := tokens.Validate(is.identityClient, token)
	if err != nil {
		return fmt.Errorf("Validate token err: %v", err)
	}
	if result {
		return nil
	}
	glog.V(2).Infof("Toen is out of date, need get new token.")
	reAuthFunction := is.provider.ReauthFunc
	if reAuthFunction() != nil {
		return fmt.Errorf("reAuth err: %v", err)
	}
	return nil
}

func (is *InstanceService) AssociateFloatingIP(instanceID, floatingIP string) error {
	opts := floatingips.AssociateOpts{
		FloatingIP: floatingIP,
	}
	return floatingips.AssociateInstance(is.computeClient, instanceID, opts).ExtractErr()
}

func (is *InstanceService) GetAcceptableFloatingIP() (string, error) {
	page, err := floatingips.List(is.computeClient).AllPages()
	if err != nil {
		return "", fmt.Errorf("Get floating IP list failed: %v", err)
	}
	list, err := floatingips.ExtractFloatingIPs(page)
	if err != nil {
		return "", err
	}
	for _, floatingIP := range list {
		if floatingIP.FixedIP == "" {
			return floatingIP.IP, nil
		}
	}
	return "", fmt.Errorf("Don't have acceptable floating IP.")
}

func (is *InstanceService) InstanceCreate(config *openstackconfigv1.OpenstackProviderConfig, cmd string, keyName string) (instance *Instance, err error) {
	var createOpts servers.CreateOpts
	if config == nil {
		return nil, fmt.Errorf("create Options need be specified to create instace.")
	}
	userData := base64.StdEncoding.EncodeToString([]byte(cmd))
	createOpts = servers.CreateOpts{
		Name:             config.Name,
		ImageName:        config.Image,
		FlavorName:       config.Flavor,
		AvailabilityZone: config.AvailabilityZone,
		Networks: []servers.Network{{
			UUID: config.Networks[0].UUID,
		}},
		UserData: []byte(userData),
		SecurityGroups: []string{
			"default",
		},
		ServiceClient: is.computeClient,
	}
	server, err := servers.Create(is.computeClient, keypairs.CreateOptsExt{
		CreateOptsBuilder: createOpts,
		KeyName:           keyName,
	}).Extract()
	if err != nil {
		return nil, fmt.Errorf("Create new server err: %v", err)
	}
	return serverToInstance(server), nil
}

func (is *InstanceService) InstanceDelete(id string) error {
	return servers.Delete(is.computeClient, id).ExtractErr()
}

func (is *InstanceService) GetInstanceList(opts *InstanceListOpts) ([]*Instance, error) {
	var listOpts servers.ListOpts
	if opts != nil {
		listOpts = servers.ListOpts{
			Name: opts.Name,
		}
	} else {
		listOpts = servers.ListOpts{}
	}

	allPages, err := servers.List(is.computeClient, listOpts).AllPages()
	if err != nil {
		return nil, fmt.Errorf("Get service list err: %v", err)
	}
	serverList, err := servers.ExtractServers(allPages)
	if err != nil {
		return nil, fmt.Errorf("Extract services list err: %v", err)
	}
	var instanceList []*Instance
	for _, server := range serverList {
		instanceList = append(instanceList, serverToInstance(&server))
	}
	return instanceList, nil
}

func (is *InstanceService) GetInstance(resourceId string) (instance *Instance, err error) {
	if resourceId == "" {
		return nil, fmt.Errorf("ResourceId should be specified to  get detail.")
	}
	server, err := servers.Get(is.computeClient, resourceId).Extract()
	if err != nil {
		return nil, fmt.Errorf("Get server %q detail failed: %v", resourceId, err)
	}
	return serverToInstance(server), err
}

func (is *InstanceService) CreateKeyPair(name, publicKey string) error {
	opts := keypairs.CreateOpts{
		Name:      name,
		PublicKey: publicKey,
	}
	_, err := keypairs.Create(is.computeClient, opts).Extract()
	return err
}

func (is *InstanceService) GetKeyPairList() ([]keypairs.KeyPair, error) {
	page, err := keypairs.List(is.computeClient).AllPages()
	if err != nil {
		return nil, err
	}
	return keypairs.ExtractKeyPairs(page)
}

func (is *InstanceService) DeleteKeyPair(name string) error {
	return keypairs.Delete(is.computeClient, name).ExtractErr()
}

func serverToInstance(server *servers.Server) *Instance {
	return &Instance{*server}
}
