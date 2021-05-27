/*
Copyright 2019 The Kubernetes Authors.

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

package kubeconfig

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/certs"
	"sigs.k8s.io/cluster-api/util/secret"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var (
	ctx = ctrl.SetupSignalHandler()

	validKubeConfig = `
clusters:
- cluster:
    certificate-authority-data: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUN5RENDQWJDZ0F3SUJBZ0lCQURBTkJna3Foa2lHOXcwQkFRc0ZBREFWTVJNd0VRWURWUVFERXdwcmRXSmwKY201bGRHVnpNQjRYRFRFNU1ERXhNREU0TURBME1Gb1hEVEk1TURFd056RTRNREEwTUZvd0ZURVRNQkVHQTFVRQpBeE1LYTNWaVpYSnVaWFJsY3pDQ0FTSXdEUVlKS29aSWh2Y05BUUVCQlFBRGdnRVBBRENDQVFvQ2dnRUJBT1EvCmVndmViNk1qMHkzM3hSbGFjczd6OXE4QTNDajcrdnRrZ0pUSjRUSVB4TldRTEd0Q0dmL0xadzlHMW9zNmRKUHgKZFhDNmkwaHA5czJuT0Y2VjQvREUrYUNTTU45VDYzckdWb2s0TkcwSlJPYmlRWEtNY1VQakpiYm9PTXF2R2lLaAoyMGlFY0h5K3B4WkZOb3FzdnlaRGM5L2dRSHJVR1FPNXp6TDNHZGhFL0k1Nkczek9JaWhhbFRHSTNaakRRS05CCmVFV3FONTVDcHZzT3I1b0ZnTmZZTXk2YzZ4WXlUTlhWSUkwNFN0Z2xBbUk4bzZWaTNUVEJhZ1BWaldIVnRha1EKU2w3VGZtVUlIdndKZUo3b2hxbXArVThvaGVTQUIraHZSbDIrVHE5NURTemtKcmRjNmphcyswd2FWaEJydEh1agpWMU15NlNvV2VVUlkrdW5VVFgwQ0F3RUFBYU1qTUNFd0RnWURWUjBQQVFIL0JBUURBZ0trTUE4R0ExVWRFd0VCCi93UUZNQU1CQWY4d0RRWUpLb1pJaHZjTkFRRUxCUUFEZ2dFQkFIT2thSXNsd0pCOE5PaENUZkF4UWlnaUc1bEMKQlo0LytGeHZ3Y1pnWGhlL0IyUWo1UURMNWlRUU1VL2NqQ0tyYUxkTFFqM1o1aHA1dzY0K2NWRUg3Vm9PSTFCaQowMm13YTc4eWo4aDNzQ2lLQXJiU21kKzNld1QrdlNpWFMzWk9EYWRHVVRRa1BnUHB0THlaMlRGakF0SW43WjcyCmpnYlVnT2FXaklKbnlwRVJ5UmlSKzBvWlk4SUlmWWFsTHUwVXlXcmkwaVhNRmZqQUQ1UVNURy8zRGN5djhEN1UKZHBxU2l5ekJkZVRjSExyenpEbktBeXhQWWgvcWpKZ0tRdEdIakhjY0FCSE1URlFtRy9Ea1pNTnZWL2FZSnMrKwp0aVJCbHExSFhlQ0d4aExFcGdQcGxVb3IrWmVYTGF2WUo0Z2dMVmIweGl2QTF2RUtyaUUwak1Wd2lQaz0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=
    server: https://test-cluster-api:6443
  name: test1
contexts:
- context:
    cluster: test1
    user: test1-admin
  name: test1-admin@test1
current-context: test1-admin@test1
kind: Config
preferences: {}
users:
- name: test1-admin
  user:
    client-certificate-data: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUM4akNDQWRxZ0F3SUJBZ0lJUTJFS3c0cU0wbFV3RFFZSktvWklodmNOQVFFTEJRQXdGVEVUTUJFR0ExVUUKQXhNS2EzVmlaWEp1WlhSbGN6QWVGdzB4T1RBeE1UQXhPREF3TkRCYUZ3MHlNREF4TVRBeE9EQXdOREphTURReApGekFWQmdOVkJBb1REbk41YzNSbGJUcHRZWE4wWlhKek1Sa3dGd1lEVlFRREV4QnJkV0psY201bGRHVnpMV0ZrCmJXbHVNSUlCSWpBTkJna3Foa2lHOXcwQkFRRUZBQU9DQVE4QU1JSUJDZ0tDQVFFQXUxeWNHcVdLMlZBUGpmWlYKeUdHWmhvQWdxZ1oreVFqU0pYQlVwNkR4VkZyYStENHJDVkNpRDNhWTVmTWNXaVpYVy9uanJsNFRudWJydndhWgpTN0hhMUxELzFZdmhFUFhLcnlBMzVNMStsN0JkUjA3T3NlRnFqNXNJQk9xWDNoNEJmckQ0SFQ5VGxSS082TXgxClMycSt5NzVaYjI5eXN0UTk3SGk4ZXVBS0sxN0JuSmJ5Zk80NlMvOFVxc2tlb0JXT3VnRkJHMlQrTFd6RXluK1oKVjdUUHZxdDE0MG1lQU40TStZUy91dFp2VmE0WFFmKy80czB4TjVwMGw4M0RrYnVWbnErcjR5dzBiSHM4cHdWdwo0Z3RuTVhrbjFhcGUwOGN4YUtBMGpodnZ4cFgyVnhHbEsxUTR0aDk1S2JNQWlpMlVINFcyNE1GSnlxOHloOUljCk54UldZd0lEQVFBQm95Y3dKVEFPQmdOVkhROEJBZjhFQkFNQ0JhQXdFd1lEVlIwbEJBd3dDZ1lJS3dZQkJRVUgKQXdJd0RRWUpLb1pJaHZjTkFRRUxCUUFEZ2dFQkFCbDUwSmNVblhGRHB4TWtkL2JOdlRHMkxVTnVWTVNnQmlCNQpmbFlpWTJibUQ3NVMreEE0eEwrZEYxanlJZHRDTGVtZURoS2MwRWtyVUpyYk5XSTZnVDB0OFAwWVNuYmxiY09ICmxBRHdmMjhoVW5TYTdIR1NOaWNBdDdKOGpnd296ZzFDVnNWbE9YM2cvcWdmSkdYeld0QzJMRFVvdjR3MFVNMVgKQ2pLNFdtMk8xTDFybEpzaHE1VysxalZzTEllYnZIcjlYb0cxRlcyY0ovcHJTK0dFS2dtNWc4YjZ1MWdJQXVFNAozOHNVQTltU3ZBYlgwR1RWdXI3Um9taWhSR2QwTktZK0k1S3A2bWtIRnpLVEVRbks2YXcrQWMvVFJObmFwUEh6Ci9IMXA0eGkyWlFHYi84MURhQjVTZDRwTURzK09FK1BNNitudkN4amN2eFdFbEdTdjE2Yz0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=
    client-key-data: LS0tLS1CRUdJTiBSU0EgUFJJVkFURSBLRVktLS0tLQpNSUlFcEFJQkFBS0NBUUVBdTF5Y0dxV0syVkFQamZaVnlHR1pob0FncWdaK3lRalNKWEJVcDZEeFZGcmErRDRyCkNWQ2lEM2FZNWZNY1dpWlhXL25qcmw0VG51YnJ2d2FaUzdIYTFMRC8xWXZoRVBYS3J5QTM1TTErbDdCZFIwN08Kc2VGcWo1c0lCT3FYM2g0QmZyRDRIVDlUbFJLTzZNeDFTMnEreTc1WmIyOXlzdFE5N0hpOGV1QUtLMTdCbkpieQpmTzQ2Uy84VXFza2VvQldPdWdGQkcyVCtMV3pFeW4rWlY3VFB2cXQxNDBtZUFONE0rWVMvdXRadlZhNFhRZisvCjRzMHhONXAwbDgzRGtidVZucStyNHl3MGJIczhwd1Z3NGd0bk1Ya24xYXBlMDhjeGFLQTBqaHZ2eHBYMlZ4R2wKSzFRNHRoOTVLYk1BaWkyVUg0VzI0TUZKeXE4eWg5SWNOeFJXWXdJREFRQUJBb0lCQVFDc0JLamw1aHNHemZTWgorQkptT1FXRmNWbU1BUTZpY0ZEUVFzUFdhM05tYVV3bElwN01uSlZOOFNzTDVCcWh3aFh1d2cwQjZDbkhlR2YxCktJL1I2V2JxWTk5ZkpsL3EvRitzVGI1RGVVL0M0UStqQ24zRzN4akE1Q3VHcUFQcTBFMjdEYXVlM3FkVWRJZDAKd1ZMbmZRZlRjOTRVNjVPNUVCZ1NaZjlXS1IvdEZDNHpGSlVselhHTlYxT2hOTWVyeXovbllmSVRZZGppUWNiRwplcDJucHk1cHZ5dEFPY1RiV0xXUEw4T2RKTDMvTER3b0h2aHlSa3huZXhWRTc0K3ZGd2lYbkRkNEp6ODVJVzBvCkFyeGEyRlJzOGZyWXFreHNSQ1VGYmRXNUpMTzhRVFBWYnl3S1c3Z0Z4S0c1U1c4Y004cmJLTHEzT01JOXBXVkoKTzNscVQxc1JBb0dCQU50QUxzR2VodzZsZWN3Zi9VU0RFN0FWNFQrdDJsWit6a3J1d3dodloyWXJYZHpXbGVqTAprNGpZWjhkQUNUUVdDRkJlaWtmYi9XdzZtMFF3ZUZudzExdVd1WndWRVJPS3BnRDFTa0krcVRtdGd0V2J2Y2lBClg4U0t4SU5qTGNzTzRLZUoxdEdkaVVDVEg3MW8zV0pBOXYzR3NaTlkrdW1WTVhnaGQ2d2YrTnB0QW9HQkFOckUKR3djOWNLVGVtZWZWSTcraFVtS2YvNm9SQ2NYdWxIK3gwSEZwNVBIQzl3NEhTMVp0Zk9NS3F6QzlJMWt6a200RwpjYW11WHovRy9iQXg4WGdaa3lURnRxTk5hdjE3Y0UzV25GRlMxejRHeGRQNDMvSkdLVWJrUzhkM1dZc0pkZnRYCkt5Vm45anl3Yjc0VG5hSnFIVlBSWFJRSkNFR3E2VlR4RVVGNlIzSVBBb0dBSmFTYlluckpUV1p6eHV3bkc4QTEKZlNJRWpsNVhBa3E3T0hwTjJnRG1pOUFlU1hBK1JMM1BFc3UwNWF6RTU4QndwUHZXV2dnWE5xSEpUcWZUd2Yxcgp2RG5nbkQreHN0MDNLeXJ5R1BXUk1HbnQ4S2JRcXIvL3NVcngrbXpveTlnK0VnWEVjRERRQTlvK3ROSndVQkkvClZjcnJhaFQ0MzJuU0dJSUdmZkx2VXZFQ2dZQmtNRGVvb3l5NWRQRExTY09yZVhnL2pzTUo0ZSsxNUVQQ0QyOUUKNFpobVdFSEkvUEkxek1MTFFCR1NxcXhMcCtEQjN0V2pQaWFGRU44U0dHMWI4V3FBQnNSVUdacU1LRUlRZzk3bgpKNmRIMHRZNjg5bXNIUkcrVThPWXdFSVQrT3M5aG5oT0UwU2tHckd5UFUyT0drY0FJZndjdHQ0L0pNVGpqOXUxClB3a0ZaUUtCZ1FDTWppdkpGL3crQXlUZUo0K1piWWpvZ091QkJFRE9oeXdiRnN5NC9ubVduTm4rQXRYQklDaGkKR2J6LzFuWkZTOGc2Nlh2cGFTWEQ5blVpS1BOUW5ORzUralJmclluTlI4WERCL3ZiNk9TMVFHbXBvWWxJZ2Q3UgpjTVpSRm1sbTUvbkJMWkdoVVpjOXZVN1pRVis4RXdLK2lHaGNrTFduVGhHNURZTkRWaksxcFE9PQotLS0tLUVORCBSU0EgUFJJVkFURSBLRVktLS0tLQo=
`

	validSecret = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test1-kubeconfig",
			Namespace: "test",
			Labels:    map[string]string{clusterv1.ClusterLabelName: "test1"},
		},
		Data: map[string][]byte{
			secret.KubeconfigDataName: []byte(validKubeConfig),
		},
		Type: clusterv1.ClusterSecretType,
	}
)

func TestGetKubeConfigSecret(t *testing.T) {
	g := NewWithT(t)

	clusterKey := client.ObjectKey{
		Name:      "test1",
		Namespace: "test",
	}
	// creating a local copy to ensure validSecret.ObjectMeta.ResourceVersion does not get set by fakeClient
	validSec := validSecret.DeepCopy()
	client := fake.NewClientBuilder().WithObjects(validSec).Build()

	found, err := FromSecret(ctx, client, clusterKey)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(found).To(Equal(validSecret.Data[secret.KubeconfigDataName]))
}

func getTestCACert(key *rsa.PrivateKey) (*x509.Certificate, error) {
	cfg := certs.Config{
		CommonName: "kubernetes",
	}

	now := time.Now().UTC()

	tmpl := x509.Certificate{
		SerialNumber: new(big.Int).SetInt64(0),
		Subject: pkix.Name{
			CommonName:   cfg.CommonName,
			Organization: cfg.Organization,
		},
		NotBefore:             now.Add(time.Minute * -5),
		NotAfter:              now.Add(time.Hour * 24), // 1 day
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		MaxPathLenZero:        true,
		BasicConstraintsValid: true,
		MaxPathLen:            0,
		IsCA:                  true,
	}

	b, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, key.Public(), key)
	if err != nil {
		return nil, err
	}

	c, err := x509.ParseCertificate(b)
	return c, err
}

func TestNew(t *testing.T) {
	g := NewWithT(t)

	testCases := []struct {
		cluster        string
		endpoint       string
		expectedConfig api.Config
		expectError    bool
	}{
		{
			cluster:  "foo",
			endpoint: "https://127:0.0.1:4003",
			expectedConfig: api.Config{
				Clusters: map[string]*api.Cluster{
					"foo": {
						Server: "https://127:0.0.1:4003",
					},
				},
				Contexts: map[string]*api.Context{
					"foo-admin@foo": {
						Cluster:  "foo",
						AuthInfo: "foo-admin",
					},
				},
				CurrentContext: "foo-admin@foo",
			},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		caKey, err := certs.NewPrivateKey()
		g.Expect(err).NotTo(HaveOccurred())

		caCert, err := getTestCACert(caKey)
		g.Expect(err).NotTo(HaveOccurred())

		actualConfig, actualError := New(tc.cluster, tc.endpoint, caCert, caKey)
		if tc.expectError {
			g.Expect(actualError).To(HaveOccurred())
			continue
		}

		g.Expect(actualConfig.Clusters).To(HaveLen(len(tc.expectedConfig.Clusters)))
		g.Expect(actualConfig.Contexts).To(HaveLen(len(tc.expectedConfig.Contexts)))

		g.Expect(actualConfig.Clusters[tc.cluster]).NotTo(BeNil())
		g.Expect(actualConfig.Contexts[tc.expectedConfig.CurrentContext]).NotTo(BeNil())
		g.Expect(actualConfig.CurrentContext).To(Equal(tc.expectedConfig.CurrentContext))
		g.Expect(actualConfig.Contexts).To(Equal(tc.expectedConfig.Contexts))
	}
}

func TestGenerateSecretWithOwner(t *testing.T) {
	g := NewWithT(t)

	owner := metav1.OwnerReference{
		Name:       "test1",
		Kind:       "Cluster",
		APIVersion: clusterv1.GroupVersion.String(),
	}

	expectedSecret := validSecret.DeepCopy()
	expectedSecret.SetOwnerReferences([]metav1.OwnerReference{owner})

	kubeconfigSecret := GenerateSecretWithOwner(
		client.ObjectKey{
			Name:      "test1",
			Namespace: "test",
		},
		[]byte(validKubeConfig),
		owner,
	)

	g.Expect(kubeconfigSecret).NotTo(BeNil())
	g.Expect(kubeconfigSecret).To(Equal(expectedSecret))
}

func TestGenerateSecret(t *testing.T) {
	g := NewWithT(t)

	expectedSecret := validSecret.DeepCopy()
	expectedSecret.SetOwnerReferences(
		[]metav1.OwnerReference{
			{
				Name:       "test1",
				Kind:       "Cluster",
				APIVersion: clusterv1.GroupVersion.String(),
			},
		},
	)

	cluster := &clusterv1.Cluster{}
	cluster.SetNamespace("test")
	cluster.SetName("test1")

	kubeconfigSecret := GenerateSecret(
		cluster,
		[]byte(validKubeConfig),
	)

	g.Expect(kubeconfigSecret).NotTo(BeNil())
	g.Expect(kubeconfigSecret).To(Equal(expectedSecret))
}

func TestCreateSecretWithOwner(t *testing.T) {
	g := NewWithT(t)

	caKey, err := certs.NewPrivateKey()
	g.Expect(err).NotTo(HaveOccurred())

	caCert, err := getTestCACert(caKey)
	g.Expect(err).NotTo(HaveOccurred())

	caSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test1-ca",
			Namespace: "test",
		},
		Data: map[string][]byte{
			secret.TLSKeyDataName: certs.EncodePrivateKeyPEM(caKey),
			secret.TLSCrtDataName: certs.EncodeCertPEM(caCert),
		},
	}

	c := fake.NewClientBuilder().WithObjects(caSecret).Build()

	owner := metav1.OwnerReference{
		Name:       "test1",
		Kind:       "Cluster",
		APIVersion: clusterv1.GroupVersion.String(),
	}

	err = CreateSecretWithOwner(
		ctx,
		c,
		client.ObjectKey{
			Name:      "test1",
			Namespace: "test",
		},
		"localhost:6443",
		owner,
	)

	g.Expect(err).NotTo(HaveOccurred())

	s := &corev1.Secret{}
	key := client.ObjectKey{Name: "test1-kubeconfig", Namespace: "test"}
	g.Expect(c.Get(ctx, key, s)).To(Succeed())
	g.Expect(s.OwnerReferences).To(ContainElement(owner))
	g.Expect(s.Type).To(Equal(clusterv1.ClusterSecretType))

	clientConfig, err := clientcmd.NewClientConfigFromBytes(s.Data[secret.KubeconfigDataName])
	g.Expect(err).NotTo(HaveOccurred())
	restClient, err := clientConfig.ClientConfig()
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(restClient.CAData).To(Equal(certs.EncodeCertPEM(caCert)))
	g.Expect(restClient.Host).To(Equal("https://localhost:6443"))
}

func TestCreateSecret(t *testing.T) {
	g := NewWithT(t)

	caKey, err := certs.NewPrivateKey()
	g.Expect(err).NotTo(HaveOccurred())

	caCert, err := getTestCACert(caKey)
	g.Expect(err).NotTo(HaveOccurred())

	caSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test1-ca",
			Namespace: "test",
		},
		Data: map[string][]byte{
			secret.TLSKeyDataName: certs.EncodePrivateKeyPEM(caKey),
			secret.TLSCrtDataName: certs.EncodeCertPEM(caCert),
		},
	}

	c := fake.NewClientBuilder().WithObjects(caSecret).Build()

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test1",
			Namespace: "test",
		},
		Spec: clusterv1.ClusterSpec{
			ControlPlaneEndpoint: clusterv1.APIEndpoint{
				Host: "localhost",
				Port: 8443,
			},
		},
	}

	err = CreateSecret(
		ctx,
		c,
		cluster,
	)

	g.Expect(err).NotTo(HaveOccurred())

	s := &corev1.Secret{}
	key := client.ObjectKey{Name: "test1-kubeconfig", Namespace: "test"}
	g.Expect(c.Get(ctx, key, s)).To(Succeed())
	g.Expect(s.OwnerReferences).To(ContainElement(
		metav1.OwnerReference{
			Name:       cluster.Name,
			Kind:       "Cluster",
			APIVersion: clusterv1.GroupVersion.String(),
		},
	))
	g.Expect(s.Type).To(Equal(clusterv1.ClusterSecretType))

	clientConfig, err := clientcmd.NewClientConfigFromBytes(s.Data[secret.KubeconfigDataName])
	g.Expect(err).NotTo(HaveOccurred())
	restClient, err := clientConfig.ClientConfig()
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(restClient.CAData).To(Equal(certs.EncodeCertPEM(caCert)))
	g.Expect(restClient.Host).To(Equal("https://localhost:8443"))
}

func TestNeedsClientCertRotation(t *testing.T) {
	g := NewWithT(t)
	caKey, err := certs.NewPrivateKey()
	g.Expect(err).NotTo(HaveOccurred())

	caCert, err := getTestCACert(caKey)
	g.Expect(err).NotTo(HaveOccurred())

	config, err := New("foo", "https://127:0.0.1:4003", caCert, caKey)
	g.Expect(err).NotTo(HaveOccurred())

	out, err := clientcmd.Write(*config)
	g.Expect(err).NotTo(HaveOccurred())

	owner := metav1.OwnerReference{
		Name:       "test1",
		Kind:       "Cluster",
		APIVersion: clusterv1.GroupVersion.String(),
	}

	kubeconfigSecret := GenerateSecretWithOwner(
		client.ObjectKey{
			Name:      "test1",
			Namespace: "test",
		},
		out,
		owner,
	)

	g.Expect(NeedsClientCertRotation(kubeconfigSecret, certs.DefaultCertDuration)).To(BeTrue())
	g.Expect(NeedsClientCertRotation(kubeconfigSecret, certs.DefaultCertDuration-time.Hour)).To(BeFalse())
}

func TestRegenerateClientCerts(t *testing.T) {
	g := NewWithT(t)
	caKey, err := certs.NewPrivateKey()
	g.Expect(err).NotTo(HaveOccurred())

	caCert, err := getTestCACert(caKey)
	g.Expect(err).NotTo(HaveOccurred())

	caSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test1-ca",
			Namespace: "test",
		},
		Data: map[string][]byte{
			secret.TLSKeyDataName: certs.EncodePrivateKeyPEM(caKey),
			secret.TLSCrtDataName: certs.EncodeCertPEM(caCert),
		},
	}

	c := fake.NewClientBuilder().WithObjects(validSecret, caSecret).Build()

	oldConfig, err := clientcmd.Load(validSecret.Data[secret.KubeconfigDataName])
	g.Expect(err).NotTo(HaveOccurred())
	oldCert, err := certs.DecodeCertPEM(oldConfig.AuthInfos["test1-admin"].ClientCertificateData)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(RegenerateSecret(ctx, c, validSecret)).To(Succeed())

	newSecret := &corev1.Secret{}
	g.Expect(c.Get(ctx, util.ObjectKey(validSecret), newSecret)).To(Succeed())
	newConfig, err := clientcmd.Load(newSecret.Data[secret.KubeconfigDataName])
	g.Expect(err).NotTo(HaveOccurred())
	newCert, err := certs.DecodeCertPEM(newConfig.AuthInfos["test1-admin"].ClientCertificateData)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(newCert.NotAfter).To(BeTemporally(">", oldCert.NotAfter))
}
