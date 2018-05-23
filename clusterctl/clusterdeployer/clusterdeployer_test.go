package clusterdeployer_test

import (
	"fmt"
	"sigs.k8s.io/cluster-api/clusterctl/clusterdeployer"
	"testing"
)

type testClusterProvisioner struct {
	err            error
	clusterCreated bool
	clusterExists  bool
}

func (p *testClusterProvisioner) Create() error {
	if p.err != nil {
		return p.err
	}
	p.clusterCreated = true
	p.clusterExists = true
	return nil
}

func (p *testClusterProvisioner) Delete() error {
	if p.err != nil {
		return p.err
	}
	p.clusterExists = false
	return nil
}

func (p *testClusterProvisioner) GetKubeconfig() (string, error) {
	return "", p.err
}

func TestCreate(t *testing.T) {
	var testcases = []struct {
		name                  string
		provisionExternalErr  error
		cleanupExternal       bool
		expectErr             bool
		expectExternalExists  bool
		expectExternalCreated bool
	}{
		{
			name:                  "success",
			cleanupExternal:       true,
			expectExternalExists:  false,
			expectExternalCreated: true,
			expectErr:            true,
		},
		{
			name:                  "success no cleaning external",
			cleanupExternal:       false,
			expectExternalExists:  true,
			expectExternalCreated: true,
			expectErr:            true,
		},
		{
			name:                 "fail provision external cluster",
			provisionExternalErr: fmt.Errorf("Test failure"),
			expectErr:            true,
		},
	}
	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			p := &testClusterProvisioner{err: testcase.provisionExternalErr}
			d := clusterdeployer.New(p, testcase.cleanupExternal)
			err := d.Create(nil, nil)
			if (testcase.expectErr && err == nil) || (!testcase.expectErr && err != nil) {
				t.Fatalf("Unexpected returned error. Got: %v, Want Err: %v", err, testcase.expectErr)
			}
			if testcase.expectExternalExists != p.clusterExists {
				t.Errorf("Unexpected external cluster existance. Got: %v, Want: %v", p.clusterExists, testcase.expectExternalExists)
			}
			if testcase.expectExternalCreated != p.clusterCreated {
				t.Errorf("Unexpected external cluster provisioning. Got: %v, Want: %v", p.clusterCreated, testcase.expectExternalCreated)
			}
		})
	}
}
