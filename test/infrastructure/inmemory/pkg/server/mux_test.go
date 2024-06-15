/*
Copyright 2023 The Kubernetes Authors.

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

package server

import (
	"context"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cloudv1 "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/internal/cloud/api/v1alpha1"
	inmemoryruntime "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/pkg/runtime"
	inmemoryproxy "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/pkg/server/proxy"
	"sigs.k8s.io/cluster-api/util/certs"
)

var (
	ctx    = context.Background()
	scheme = runtime.NewScheme()
)

func init() {
	_ = metav1.AddMetaToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = rbacv1.AddToScheme(scheme)

	ctrl.SetLogger(klog.Background())
}

func TestMux(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	manager := inmemoryruntime.NewManager(scheme)

	wcl := "workload-cluster"
	host := "127.0.0.1"
	wcmux, err := NewWorkloadClustersMux(manager, host, CustomPorts{
		// NOTE: make sure to use ports different than other tests, so we can run tests in parallel
		MinPort:   DefaultMinPort,
		MaxPort:   DefaultMinPort + 99,
		DebugPort: DefaultDebugPort,
	})
	g.Expect(err).ToNot(HaveOccurred())

	listener, err := wcmux.InitWorkloadClusterListener(wcl)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(listener.Host()).To(Equal(host))
	g.Expect(listener.Port()).ToNot(BeZero())

	caCert, caKey, err := newCertificateAuthority()
	g.Expect(err).ToNot(HaveOccurred())

	etcdCert, etcdKey, err := newCertificateAuthority()
	g.Expect(err).ToNot(HaveOccurred())

	apiServerPod1 := "apiserver1"
	err = wcmux.AddAPIServer(wcl, apiServerPod1, caCert, caKey)
	g.Expect(err).ToNot(HaveOccurred())

	etcdPodMember1 := "etcd1"
	err = wcmux.AddEtcdMember(wcl, etcdPodMember1, etcdCert, etcdKey)
	g.Expect(err).ToNot(HaveOccurred())

	apiServerPod2 := "apiserver2"
	err = wcmux.AddAPIServer(wcl, apiServerPod2, caCert, caKey)
	g.Expect(err).ToNot(HaveOccurred())

	etcdPodMember2 := "etcd2"
	err = wcmux.AddEtcdMember(wcl, etcdPodMember2, etcdCert, etcdKey)
	g.Expect(err).ToNot(HaveOccurred())

	err = wcmux.DeleteAPIServer(wcl, apiServerPod2)
	g.Expect(err).ToNot(HaveOccurred())

	err = wcmux.DeleteEtcdMember(wcl, etcdPodMember2)
	g.Expect(err).ToNot(HaveOccurred())

	err = wcmux.DeleteAPIServer(wcl, apiServerPod1)
	g.Expect(err).ToNot(HaveOccurred())

	err = wcmux.DeleteEtcdMember(wcl, etcdPodMember1)
	g.Expect(err).ToNot(HaveOccurred())

	err = wcmux.DeleteWorkloadClusterListener(wcl)
	g.Expect(err).ToNot(HaveOccurred())

	err = wcmux.Shutdown(ctx)
	g.Expect(err).ToNot(HaveOccurred())
}

func TestAPI_corev1_CRUD(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	wcmux, c := setupWorkloadClusterListener(g, CustomPorts{
		// NOTE: make sure to use ports different than other tests, so we can run tests in parallel
		MinPort:   DefaultMinPort + 100,
		MaxPort:   DefaultMinPort + 199,
		DebugPort: DefaultDebugPort + 1,
	})

	// create

	n := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "foo"},
	}
	err := c.Create(ctx, n)
	g.Expect(err).ToNot(HaveOccurred())

	// list

	nl := &corev1.NodeList{}
	err = c.List(ctx, nl)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(nl.Items).To(HaveLen(1))
	g.Expect(nl.Items[0].Name).To(Equal("foo"))

	// list with nodeName selector on pod
	g.Expect(c.Create(ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "bar", Namespace: metav1.NamespaceDefault},
		Spec:       corev1.PodSpec{NodeName: n.Name},
	})).To(Succeed())
	g.Expect(c.Create(ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "labelSelectedPod", Namespace: metav1.NamespaceDefault, Labels: map[string]string{"name": "labelSelectedPod"}},
	})).To(Succeed())

	pl := &corev1.PodList{}
	nodeNameSelector := &client.ListOptions{
		FieldSelector: fields.SelectorFromSet(fields.Set{"spec.nodeName": n.Name}),
	}
	g.Expect(c.List(ctx, pl, nodeNameSelector)).To(Succeed())
	g.Expect(pl.Items).To(HaveLen(1))
	g.Expect(pl.Items[0].Name).To(Equal("bar"))

	// list with label selector on pod
	labelSelector := &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set{"name": "labelSelectedPod"}),
	}
	g.Expect(c.List(ctx, pl, labelSelector)).To(Succeed())
	g.Expect(pl.Items).To(HaveLen(1))
	g.Expect(pl.Items[0].Name).To(Equal("labelSelectedPod"))

	// get
	n = &corev1.Node{}
	err = c.Get(ctx, client.ObjectKey{Name: "foo"}, n)
	g.Expect(err).ToNot(HaveOccurred())

	// patch

	n2 := n.DeepCopy()
	n2.Annotations = map[string]string{"foo": "bar"}
	err = c.Patch(ctx, n2, client.MergeFrom(n))
	g.Expect(err).ToNot(HaveOccurred())

	n3 := n2.DeepCopy()
	taints := []corev1.Taint{{Key: "foo"}}

	n3.Spec.Taints = taints
	err = c.Patch(ctx, n3, client.StrategicMergeFrom(n2))
	g.Expect(err).ToNot(HaveOccurred())

	node := &corev1.Node{}
	g.Expect(c.Get(ctx, client.ObjectKeyFromObject(n3), node)).To(Succeed())
	g.Expect(node.Spec.Taints).To(BeComparableTo(taints))

	// delete

	err = c.Delete(ctx, n)
	g.Expect(err).ToNot(HaveOccurred())

	err = wcmux.Shutdown(ctx)
	g.Expect(err).ToNot(HaveOccurred())
}

func TestAPI_rbacv1_CRUD(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	wcmux, c := setupWorkloadClusterListener(g, CustomPorts{
		// NOTE: make sure to use ports different than other tests, so we can run tests in parallel
		MinPort:   DefaultMinPort + 200,
		MaxPort:   DefaultMinPort + 299,
		DebugPort: DefaultDebugPort + 2,
	})

	// create

	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "foo"},
	}

	err := c.Create(ctx, cr)
	g.Expect(err).ToNot(HaveOccurred())

	// list

	crl := &rbacv1.ClusterRoleList{}
	err = c.List(ctx, crl)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(crl.Items).To(HaveLen(1))
	g.Expect(crl.Items[0].Name).To(Equal("foo"))

	// get

	err = c.Get(ctx, client.ObjectKey{Name: "foo"}, cr)
	g.Expect(err).ToNot(HaveOccurred())

	// patch

	cr2 := cr.DeepCopy()
	cr2.Annotations = map[string]string{"foo": "bar"}
	err = c.Patch(ctx, cr2, client.MergeFrom(cr))
	g.Expect(err).ToNot(HaveOccurred())

	// delete

	err = c.Delete(ctx, cr)
	g.Expect(err).ToNot(HaveOccurred())

	err = wcmux.Shutdown(ctx)
	g.Expect(err).ToNot(HaveOccurred())
}

func TestAPI_PortForward(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	manager := inmemoryruntime.NewManager(scheme)

	// TODO: deduplicate this setup code with the test above
	host := "127.0.0.1"
	wcmux, err := NewWorkloadClustersMux(manager, host, CustomPorts{
		// NOTE: make sure to use ports different than other tests, so we can run tests in parallel
		MinPort:   DefaultMinPort + 300,
		MaxPort:   DefaultMinPort + 399,
		DebugPort: DefaultDebugPort + 3,
	})
	g.Expect(err).ToNot(HaveOccurred())

	// InfraCluster controller >> when "creating the load balancer"
	wcl1 := "workload-cluster1-controlPlaneEndpoint"
	listener, err := wcmux.InitWorkloadClusterListener(wcl1)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(listener.Host()).To(Equal(host))
	g.Expect(listener.Port()).ToNot(BeZero())

	caCert, caKey, err := newCertificateAuthority()
	g.Expect(err).ToNot(HaveOccurred())

	// InfraMachine controller >> when "creating the API Server pod"
	apiServerPod1 := "kube-apiserver-1"
	err = wcmux.AddAPIServer(wcl1, apiServerPod1, caCert, caKey)
	g.Expect(err).ToNot(HaveOccurred())

	etcdCert, etcdKey, err := newCertificateAuthority()
	g.Expect(err).ToNot(HaveOccurred())

	// InfraMachine controller >> when "creating the Etcd member pod"
	etcdPodMember1 := "etcd-1"
	err = wcmux.AddEtcdMember(wcl1, etcdPodMember1, etcdCert, etcdKey)
	g.Expect(err).ToNot(HaveOccurred())

	// Setup resource group
	resourceGroup := "workload-cluster1-resourceGroup"
	manager.AddResourceGroup(resourceGroup)
	err = wcmux.RegisterResourceGroup(wcl1, resourceGroup)
	g.Expect(err).ToNot(HaveOccurred())

	etcdPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: metav1.NamespaceSystem,
			Name:      "etcd-1",
			Labels: map[string]string{
				"component": "etcd",
				"tier":      "control-plane",
			},
			Annotations: map[string]string{
				cloudv1.EtcdClusterIDAnnotationName:  "1",
				cloudv1.EtcdMemberIDAnnotationName:   "2",
				cloudv1.EtcdLeaderFromAnnotationName: time.Now().Format(time.RFC3339),
			},
		},
	}
	err = manager.GetResourceGroup(resourceGroup).GetClient().Create(ctx, etcdPod)
	g.Expect(err).ToNot(HaveOccurred())

	// Test API server TLS handshake via port forward.

	restConfig, err := listener.RESTConfig()
	g.Expect(err).ToNot(HaveOccurred())

	p1 := inmemoryproxy.Proxy{
		Kind:       "pods",
		Namespace:  metav1.NamespaceSystem,
		KubeConfig: restConfig,
		Port:       1234,
	}

	dialer1, err := inmemoryproxy.NewDialer(p1)
	g.Expect(err).ToNot(HaveOccurred())

	rawConn, err := dialer1.DialContextWithAddr(ctx, "kube-apiserver-foo")
	g.Expect(err).ToNot(HaveOccurred())
	defer rawConn.Close()

	conn := tls.Client(rawConn, &tls.Config{InsecureSkipVerify: true}) //nolint:gosec // Intentionally not verifying the server cert here.
	err = conn.HandshakeContext(ctx)
	g.Expect(err).ToNot(HaveOccurred())
	defer conn.Close()

	// Test Etcd via port forward

	caPool := x509.NewCertPool()
	caPool.AddCert(etcdCert)

	config := apiServerEtcdClientCertificateConfig()
	cert, key, err := newCertAndKey(etcdCert, etcdKey, config)
	g.Expect(err).ToNot(HaveOccurred())

	clientCert, err := tls.X509KeyPair(certs.EncodeCertPEM(cert), certs.EncodePrivateKeyPEM(key))
	g.Expect(err).ToNot(HaveOccurred())

	p2 := inmemoryproxy.Proxy{
		Kind:       "pods",
		Namespace:  metav1.NamespaceSystem,
		KubeConfig: restConfig,
		Port:       2379,
	}

	dialer2, err := inmemoryproxy.NewDialer(p2)
	g.Expect(err).ToNot(HaveOccurred())

	etcdClient1, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{etcdPodMember1},
		DialTimeout: 2 * time.Second,

		DialOptions: []grpc.DialOption{
			grpc.WithContextDialer(dialer2.DialContextWithAddr),
		},
		TLS: &tls.Config{
			RootCAs:      caPool,
			Certificates: []tls.Certificate{clientCert},
			MinVersion:   tls.VersionTLS12,
		},
	})
	g.Expect(err).ToNot(HaveOccurred())

	ml, err := etcdClient1.MemberList(ctx)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ml.Members).To(HaveLen(1))
	g.Expect(ml.Members[0].Name).To(Equal("1"))

	err = etcdClient1.Close()
	g.Expect(err).ToNot(HaveOccurred())

	err = wcmux.Shutdown(ctx)
	g.Expect(err).ToNot(HaveOccurred())
}

func TestAPI_corev1_Watch(t *testing.T) {
	g := NewWithT(t)

	_, c := setupWorkloadClusterListener(g, CustomPorts{
		// NOTE: make sure to use ports different than other tests, so we can run tests in parallel
		MinPort:   DefaultMinPort + 400,
		MaxPort:   DefaultMinPort + 499,
		DebugPort: DefaultDebugPort + 4,
	})

	ctx := context.Background()

	nodeWatcher, err := c.Watch(ctx, &corev1.NodeList{})
	g.Expect(err).ToNot(HaveOccurred())

	podWatcher, err := c.Watch(ctx, &corev1.PodList{})
	g.Expect(err).ToNot(HaveOccurred())

	expectedEvents := []string{"ADDED/foo", "MODIFIED/foo", "DELETED/foo", "ADDED/bar", "MODIFIED/bar", "DELETED/bar"}
	receivedEvents := []string{}
	done := make(chan bool)
	go func() {
		for {
			select {
			case event := <-nodeWatcher.ResultChan():
				o, ok := event.Object.(client.Object)
				if !ok {
					return
				}
				receivedEvents = append(receivedEvents, fmt.Sprintf("%s/%s", event.Type, o.GetName()))
			case event := <-podWatcher.ResultChan():
				o, ok := event.Object.(client.Object)
				if !ok {
					return
				}
				receivedEvents = append(receivedEvents, fmt.Sprintf("%s/%s", event.Type, o.GetName()))
			case <-done:
				nodeWatcher.Stop()
			}
		}
	}()

	// Test watcher on Node resources.

	node1 := &corev1.Node{}
	node1.SetName("foo")

	// create node
	err = c.Create(ctx, node1)
	g.Expect(err).ToNot(HaveOccurred())

	// get node
	n := &corev1.Node{}
	err = c.Get(ctx, client.ObjectKey{Name: "foo"}, n)
	g.Expect(err).ToNot(HaveOccurred())

	// patch node
	nodeWithAnnotations := n.DeepCopy()
	nodeWithAnnotations.SetAnnotations(map[string]string{"foo": "bar"})
	err = c.Patch(ctx, nodeWithAnnotations, client.MergeFrom(n))
	g.Expect(err).ToNot(HaveOccurred())

	// delete node
	err = c.Delete(ctx, n)
	g.Expect(err).ToNot(HaveOccurred())

	// Test watcher on Pod resources.
	pod1 := &corev1.Pod{}
	pod1.SetName("bar")
	pod1.SetNamespace("one")

	// create pod
	err = c.Create(ctx, pod1)
	g.Expect(err).ToNot(HaveOccurred())

	// patch pod
	p := &corev1.Pod{}
	err = c.Get(ctx, client.ObjectKey{Name: "bar", Namespace: "one"}, p)
	g.Expect(err).ToNot(HaveOccurred())
	podWithAnnotations := p.DeepCopy()
	podWithAnnotations.SetAnnotations(map[string]string{"foo": "bar"})
	err = c.Patch(ctx, podWithAnnotations, client.MergeFrom(p))
	g.Expect(err).ToNot(HaveOccurred())

	// delete pod
	err = c.Delete(ctx, p)
	g.Expect(err).ToNot(HaveOccurred())

	// Wait a second to ensure all events have been flushed.
	time.Sleep(time.Second)

	// Send a done signal to close the test goroutine.
	done <- true

	// Each event should be the same and in the same order.
	g.Expect(receivedEvents).To(Equal(expectedEvents))
}

func setupWorkloadClusterListener(g Gomega, ports CustomPorts) (*WorkloadClustersMux, client.WithWatch) {
	manager := inmemoryruntime.NewManager(scheme)

	host := "127.0.0.1"
	wcmux, err := NewWorkloadClustersMux(manager, host, ports)
	g.Expect(err).ToNot(HaveOccurred())

	// InfraCluster controller >> when "creating the load balancer"
	wcl1 := "workload-cluster1-controlPlaneEndpoint"

	listener, err := wcmux.InitWorkloadClusterListener(wcl1)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(listener.Host()).To(Equal(host))
	g.Expect(listener.Port()).ToNot(BeZero())

	resourceGroup := "workload-cluster1-resourceGroup"
	manager.AddResourceGroup(resourceGroup)
	err = wcmux.RegisterResourceGroup(wcl1, resourceGroup)
	g.Expect(err).ToNot(HaveOccurred())

	caCert, caKey, err := newCertificateAuthority()
	g.Expect(err).ToNot(HaveOccurred())

	// InfraMachine controller >> when "creating the API Server pod"
	apiServerPod1 := "kube-apiserver-1"
	err = wcmux.AddAPIServer(wcl1, apiServerPod1, caCert, caKey)
	g.Expect(err).ToNot(HaveOccurred())

	etcdCert, etcdKey, err := newCertificateAuthority()
	g.Expect(err).ToNot(HaveOccurred())

	// InfraMachine controller >> when "creating the Etcd member pod"
	etcdPodMember1 := "etcd-1"
	err = wcmux.AddEtcdMember(wcl1, etcdPodMember1, etcdCert, etcdKey)
	g.Expect(err).ToNot(HaveOccurred())

	// Test API using a controller runtime client to call it.
	c, err := listener.GetClient()
	g.Expect(err).ToNot(HaveOccurred())

	return wcmux, c
}

// newCertificateAuthority creates new certificate and private key for the certificate authority.
func newCertificateAuthority() (*x509.Certificate, *rsa.PrivateKey, error) {
	key, err := certs.NewPrivateKey()
	if err != nil {
		return nil, nil, err
	}

	c, err := newSelfSignedCACert(key)
	if err != nil {
		return nil, nil, err
	}

	return c, key, nil
}

// newSelfSignedCACert creates a CA certificate.
func newSelfSignedCACert(key *rsa.PrivateKey) (*x509.Certificate, error) {
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
		NotAfter:              now.Add(time.Hour * 24 * 365 * 10), // 10 years
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		MaxPathLenZero:        true,
		BasicConstraintsValid: true,
		MaxPathLen:            0,
		IsCA:                  true,
	}

	b, err := x509.CreateCertificate(cryptorand.Reader, &tmpl, &tmpl, key.Public(), key)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create self signed CA certificate: %+v", tmpl)
	}

	c, err := x509.ParseCertificate(b)
	return c, errors.WithStack(err)
}

func apiServerEtcdClientCertificateConfig() *certs.Config {
	return &certs.Config{
		CommonName:   "apiserver-etcd-client",
		Organization: []string{"system:masters"}, // TODO: check if we can drop
		Usages:       []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
}
