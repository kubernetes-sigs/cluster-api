package internal

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"strings"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/cert"
	bootstrapv1 "sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm/api/v1alpha2"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	rsaKeySize     = 2048
	rootOwnerValue = "root:root"

	TLSKeyDataName = "tls.key"
	TLSCrtDataName = "tls.crt"

	// ClusterCAName is the secret name suffix for apiserver CA
	ClusterCAName = "ca"

	// EtcdCAName is the secret name suffix for the Etcd CA
	EtcdCAName = "etcd"

	// ServiceAccountName is the secret name suffix for the Service Account keys
	ServiceAccountName = "sa"

	// FrontProxyCAName is the secret name suffix for Front Proxy CA
	FrontProxyCAName = "proxy"
)

type Certificates []*Certificate

func NewCertificates() Certificates {
	return Certificates{
		&Certificate{Name: ClusterCAName},
		&Certificate{Name: EtcdCAName},
		&Certificate{Name: ServiceAccountName},
		&Certificate{Name: FrontProxyCAName},
	}
}

func (c Certificates) GetCertificateByName(name string) *Certificate {
	for _, certificate := range c {
		if certificate.Name == name {
			return certificate
		}
	}
	return nil
}

func (c Certificates) GetCertificates(ctx context.Context, ctrlclient client.Client, cluster *clusterv1.Cluster) error {
	// Look up each certificate as a secret and populate the certificate/key
	for _, certificate := range c {
		secret := &corev1.Secret{}
		key := client.ObjectKey{
			Name:      certificate.SecretName(cluster.Name),
			Namespace: cluster.Namespace,
		}
		if err := ctrlclient.Get(ctx, key, secret); err != nil {
			if apierrors.IsNotFound(err) {
				kp, err := generateCACert()
				if err != nil {
					return err
				}
				certificate.KeyPair = kp
				certificate.Generated = true
				continue
			}
			return errors.WithStack(err)
		}
		// If a user has a badly formatted secret it will prevent the cluster from working.
		kp, err := secretToKeyPair(secret)
		if err != nil {
			return err
		}
		certificate.KeyPair = kp
	}
	return nil
}

func (c Certificates) GenerateCertificates() error {
	for _, certificate := range c {
		if certificate.KeyPair == nil {
			kp, err := generateCACert()
			if err != nil {
				return err
			}
			certificate.KeyPair = kp
			certificate.Generated = true
		}
	}
	return nil
}

func (c Certificates) SaveGeneratedCertificates(ctx context.Context, ctrlclient client.Client, cluster *clusterv1.Cluster, config *bootstrapv1.KubeadmConfig) error {
	for _, certificate := range c {
		if !certificate.Generated {
			continue
		}
		secret := certificate.AsSecret(cluster, config)
		if err := ctrlclient.Create(ctx, secret); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

func (c Certificates) GetOrCreateCertificates(ctx context.Context, ctrlclient client.Client, cluster *clusterv1.Cluster, config *bootstrapv1.KubeadmConfig) error {
	// Get the certificates that exist
	if err := c.GetCertificates(ctx, ctrlclient, cluster); err != nil {
		return err
	}

	// Create the certificates that don't exist
	if err := c.GenerateCertificates(); err != nil {
		return err
	}

	// Save any certificates that have been generated
	if err := c.SaveGeneratedCertificates(ctx, ctrlclient, cluster, config); err != nil {
		return err
	}

	return nil
}

type Certificate struct {
	Generated bool
	Name      string
	*KeyPair
}

func (c *Certificate) SecretName(clustername string) string {
	return fmt.Sprintf("%s-%s", clustername, c.Name)
}

// CertificateHashes hash all the certificates stored in a CA certificate.
func (c *Certificate) Hashes() ([]string, error) {
	certificates, err := cert.ParseCertsPEM(c.Cert)
	if err != nil {
		return nil, errors.Wrap(err, "unable to parse Cluster CA Certificate")
	}
	out := make([]string, 0)
	for _, c := range certificates {
		out = append(out, hashCert(c))
	}
	return out, nil
}

// HashCert will calculate the sha256 of the incoming certificate.
func hashCert(certificate *x509.Certificate) string {
	spkiHash := sha256.Sum256(certificate.RawSubjectPublicKeyInfo)
	return "sha256:" + strings.ToLower(hex.EncodeToString(spkiHash[:]))
}

// KeyPair holds the raw bytes for a certificate and key
type KeyPair struct {
	Cert, Key []byte
}

func (c *Certificate) AsSecret(cluster *clusterv1.Cluster, config *bootstrapv1.KubeadmConfig) *corev1.Secret {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cluster.Namespace,
			Name:      fmt.Sprintf("%s-%s", cluster.Name, c.Name),
			Labels: map[string]string{
				clusterv1.MachineClusterLabelName: cluster.Name,
			},
		},
		Data: map[string][]byte{
			TLSKeyDataName: c.KeyPair.Key,
			TLSCrtDataName: c.KeyPair.Cert,
		},
	}

	if c.Generated {
		secret.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion: bootstrapv1.GroupVersion.String(),
				Kind:       "KubeadmConfig",
				Name:       config.Name,
				UID:        config.UID,
			},
		}
	}
	return secret
}

func (c Certificates) AsFiles() []bootstrapv1.File {
	clusterCA := c.GetCertificateByName(ClusterCAName)
	etcdCA := c.GetCertificateByName(EtcdCAName)
	frontProxyCA := c.GetCertificateByName(FrontProxyCAName)
	serviceAccountCA := c.GetCertificateByName(ServiceAccountName)

	return []bootstrapv1.File{
		{
			Path:        "/etc/kubernetes/pki/ca.crt",
			Owner:       rootOwnerValue,
			Permissions: "0640",
			Content:     string(clusterCA.Cert),
		},
		{
			Path:        "/etc/kubernetes/pki/ca.key",
			Owner:       rootOwnerValue,
			Permissions: "0600",
			Content:     string(clusterCA.Key),
		},
		{
			Path:        "/etc/kubernetes/pki/etcd/ca.crt",
			Owner:       rootOwnerValue,
			Permissions: "0640",
			Content:     string(etcdCA.Cert),
		},
		{
			Path:        "/etc/kubernetes/pki/etcd/ca.key",
			Owner:       rootOwnerValue,
			Permissions: "0600",
			Content:     string(etcdCA.Key),
		},
		{
			Path:        "/etc/kubernetes/pki/front-proxy-ca.crt",
			Owner:       rootOwnerValue,
			Permissions: "0640",
			Content:     string(frontProxyCA.Cert),
		},
		{
			Path:        "/etc/kubernetes/pki/front-proxy-ca.key",
			Owner:       rootOwnerValue,
			Permissions: "0600",
			Content:     string(frontProxyCA.Key),
		},
		{
			Path:        "/etc/kubernetes/pki/sa.pub",
			Owner:       rootOwnerValue,
			Permissions: "0640",
			Content:     string(serviceAccountCA.Cert),
		},
		{
			Path:        "/etc/kubernetes/pki/sa.key",
			Owner:       rootOwnerValue,
			Permissions: "0600",
			Content:     string(serviceAccountCA.Key),
		},
	}
}

// Validate checks that all KeyPairs are valid
func (c Certificates) Validate() error {
	for _, certificate := range c {
		if !certificate.KeyPair.isValid() {
			return errors.Errorf("CA cert material is missing cert/key for %s", certificate.Name)
		}
	}
	return nil
}

func (kp *KeyPair) isValid() bool {
	return kp.Cert != nil && kp.Key != nil
}

func secretToKeyPair(s *corev1.Secret) (*KeyPair, error) {
	cert, exists := s.Data[TLSCrtDataName]
	if !exists {
		return nil, errors.Errorf("missing data for key %s", TLSCrtDataName)
	}

	key, exists := s.Data[TLSKeyDataName]
	if !exists {
		return nil, errors.Errorf("missing data for key %s", TLSKeyDataName)
	}

	return &KeyPair{
		Cert: cert,
		Key:  key,
	}, nil
}

func generateCACert() (*KeyPair, error) {
	x509Cert, privKey, err := newCertificateAuthority()
	if err != nil {
		return nil, err
	}
	return &KeyPair{
		Cert: encodeCertPEM(x509Cert),
		Key:  encodePrivateKeyPEM(privKey),
	}, nil
}

// newCertificateAuthority creates new certificate and private key for the certificate authority
func newCertificateAuthority() (*x509.Certificate, *rsa.PrivateKey, error) {
	key, err := newPrivateKey()
	if err != nil {
		return nil, nil, err
	}

	cert, err := newSelfSignedCACert(key)
	if err != nil {
		return nil, nil, err
	}

	return cert, key, nil
}

// newPrivateKey creates an RSA private key
func newPrivateKey() (*rsa.PrivateKey, error) {
	pk, err := rsa.GenerateKey(rand.Reader, rsaKeySize)
	return pk, errors.WithStack(err)
}

// newSelfSignedCACert creates a CA certificate.
func newSelfSignedCACert(key *rsa.PrivateKey) (*x509.Certificate, error) {
	cfg := config{
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
		NotAfter:              now.Add(time.Hour * 24 * 365 * 10), // 10 years
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		MaxPathLenZero:        true,
		BasicConstraintsValid: true,
		MaxPathLen:            0,
		IsCA:                  true,
	}

	b, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, key.Public(), key)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create self signed CA certificate: %+v", tmpl)
	}

	cert, err := x509.ParseCertificate(b)
	return cert, errors.WithStack(err)
}

// encodeCertPEM returns PEM-endcoded certificate data.
func encodeCertPEM(cert *x509.Certificate) []byte {
	block := pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	}
	return pem.EncodeToMemory(&block)
}

// encodePrivateKeyPEM returns PEM-encoded private key data.
func encodePrivateKeyPEM(key *rsa.PrivateKey) []byte {
	block := pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}

	return pem.EncodeToMemory(&block)
}

// config contains the basic fields required for creating a certificate
type config struct {
	CommonName   string
	Organization []string
	AltNames     altNames
	Usages       []x509.ExtKeyUsage
}

// AltNames contains the domain names and IP addresses that will be added
// to the API Server's x509 certificate SubAltNames field. The values will
// be passed directly to the x509.Certificate object.
type altNames struct {
	DNSNames []string
	IPs      []net.IP
}
