/*
Copyright 2020 The Kubernetes Authors.

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

package controllers

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	certificates "k8s.io/api/certificates/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clientcmdlatest "k8s.io/client-go/tools/clientcmd/api/latest"
	"k8s.io/client-go/util/cert"
	"k8s.io/client-go/util/certificate"
	"k8s.io/client-go/util/keyutil"
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha4"
	"sigs.k8s.io/cluster-api/controllers/remote"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/kubemark/api/v1alpha4"
	capkcert "sigs.k8s.io/cluster-api/test/infrastructure/kubemark/util/certificate"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// KubemarkMachineReconciler reconciles a KubemarkMachine object
type KubemarkMachineReconciler struct {
	client.Client
	Log           logr.Logger
	Scheme        *runtime.Scheme
	KubemarkImage string
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=kubemarkmachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=kubemarkmachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines;machines/status,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets;,verbs=get;list;watch
// +kubebuilder:rbac:groups=bootstrap.cluster.x-k8s.io,resources=kubeadmconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=create;delete
// +kubebuilder:rbac:groups="",resources=pods,verbs=create;delete

func (r *KubemarkMachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.WithValues("kubemarkmachine", req.NamespacedName)

	kubemarkMachine := &infrav1.KubemarkMachine{}
	err := r.Get(ctx, req.NamespacedName, kubemarkMachine)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "error finding kubemark machine")
		return ctrl.Result{}, err
	}
	helper, err := patch.NewHelper(kubemarkMachine, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to init patch helper: %w", err)
	}

	controllerutil.AddFinalizer(kubemarkMachine, infrav1.MachineFinalizer)
	if err := helper.Patch(ctx, kubemarkMachine); err != nil {
		logger.Error(err, "failed to add finalizer")
		return ctrl.Result{}, err
	}

	defer func() {
		if err := helper.Patch(ctx, kubemarkMachine); err != nil {
			if !apierrors.IsNotFound(err) {
				logger.Error(err, "failed to patch kubemarkMachine")
			}
		}
	}()

	if !kubemarkMachine.ObjectMeta.DeletionTimestamp.IsZero() {
		logger.Info("deleting machine")

		if err := r.Delete(ctx, &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      kubemarkMachine.Name,
				Namespace: kubemarkMachine.Namespace,
			},
		}); err != nil {
			if !apierrors.IsNotFound(err) {
				logger.Error(err, "error deleting kubemark pod")
				return ctrl.Result{}, err
			}
		}
		if err := r.Delete(ctx, &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      kubemarkMachine.Name,
				Namespace: kubemarkMachine.Namespace,
			},
		}); err != nil {
			if !apierrors.IsNotFound(err) {
				logger.Error(err, "error deleting kubemark configMap")
				return ctrl.Result{}, err
			}
		}
		controllerutil.RemoveFinalizer(kubemarkMachine, infrav1.MachineFinalizer)
		return ctrl.Result{}, nil
	}

	if kubemarkMachine.Status.Ready {
		logger.Info("machine already ready, skipping reconcile")
		return ctrl.Result{}, err
	}

	// Fetch the Machine.
	machine, err := util.GetOwnerMachine(ctx, r.Client, kubemarkMachine.ObjectMeta)
	if err != nil {
		logger.Error(err, "error finding owner machine")
		return ctrl.Result{}, err
	}
	if machine == nil {
		logger.Info("Machine Controller has not yet set OwnerRef")
		return ctrl.Result{}, nil
	}
	machinePatchHelper, err := patch.NewHelper(machine, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to init patch helper: %w", err)
	}
	defer func() {
		if err := machinePatchHelper.Patch(ctx, machine); err != nil {
			if !apierrors.IsNotFound(err) {
				logger.Error(err, "failed to patch machine")
			}
		}
	}()

	logger = logger.WithValues("machine", machine.Name)

	// Fetch the Cluster.
	cluster, err := util.GetClusterFromMetadata(ctx, r.Client, machine.ObjectMeta)
	if err != nil {
		logger.Info("Machine is missing cluster label or cluster does not exist")
		return ctrl.Result{}, nil
	}
	logger = logger.WithValues("cluster", cluster.Name)

	restConfig, _, err := getRemoteCluster(ctx, logger, r.Client, cluster)
	if err != nil {
		logger.Error(err, "error getting remote cluster")
		return ctrl.Result{}, err
	}

	if !cluster.Status.InfrastructureReady {
		logger.Info("Cluster infrastructure is not ready yet")
		return ctrl.Result{}, nil
	}
	if machine.Spec.Bootstrap.DataSecretName == nil {
		logger.Info("Bootstrap data secret reference is not yet available")
		return ctrl.Result{}, nil
	}

	var kubeadmConfig bootstrapv1.KubeadmConfig
	if err := r.Get(ctx, types.NamespacedName{
		Name:      machine.Spec.Bootstrap.ConfigRef.Name,
		Namespace: machine.Spec.Bootstrap.ConfigRef.Namespace,
	}, &kubeadmConfig); err != nil {
		logger.Error(err, "error getting bootstrap config")
		return ctrl.Result{}, err
	}

	cfg, err := RetrieveValidatedConfigInfo(ctx, kubeadmConfig.Spec.JoinConfiguration)
	if err != nil {
		logger.Error(err, "error validating token")
		return ctrl.Result{}, err
	}

	clusterinfo := cfg.Clusters[""]
	cfg = CreateWithToken(
		clusterinfo.Server,
		DefaultClusterName,
		TokenUser,
		clusterinfo.CertificateAuthorityData,
		kubeadmConfig.Spec.JoinConfiguration.Discovery.BootstrapToken.Token,
	)
	certificateStore := &capkcert.MemoryStore{}

	newClientFn := func(current *tls.Certificate) (clientset.Interface, error) {
		client, err := clientset.NewForConfig(restConfig)
		if err != nil {
			logger.Error(err, "error creating clientset")
			return nil, err
		}
		return client, nil
	}
	mgr, err := certificate.NewManager(&certificate.Config{
		BootstrapCertificatePEM: cfg.AuthInfos[TokenUser].ClientCertificateData,
		BootstrapKeyPEM:         cfg.AuthInfos[TokenUser].ClientKeyData,
		CertificateStore:        certificateStore,
		SignerName:              certificates.KubeAPIServerClientKubeletSignerName,
		Template: &x509.CertificateRequest{
			Subject: pkix.Name{
				CommonName:   fmt.Sprintf("system:node:%s", kubemarkMachine.Name),
				Organization: []string{"system:nodes"},
			},
		},
		Usages: []certificates.KeyUsage{
			certificates.UsageDigitalSignature,
			certificates.UsageKeyEncipherment,
			certificates.UsageClientAuth,
		},
		ClientsetFn: newClientFn,
	})
	if err != nil {
		logger.Error(err, "error creating cert manager")
		return ctrl.Result{}, err
	}

	mgr.Start()

	for {
		_, err := certificateStore.Current()
		if err != nil {
			if _, ok := err.(*certificate.NoCertKeyError); !ok {
				logger.Error(err, "err fetching certificate")
				return ctrl.Result{}, err
			}

			logger.Info("waiting for certificate")
			time.Sleep(time.Second)

			continue
		}

		break
	}
	mgr.Stop()

	kubeconfig, err := generateCertificateKubeconfig(restConfig, "/kubeconfig/cert.pem")
	if err != nil {
		logger.Error(err, "err generating certificate kubeconfig")
		return ctrl.Result{}, err
	}

	stackedCert := bytes.Buffer{}
	if err := pem.Encode(&stackedCert, &pem.Block{Type: cert.CertificateBlockType, Bytes: certificateStore.Certificate.Leaf.Raw}); err != nil {
		logger.Error(err, "err encoding certificate")
		return ctrl.Result{}, err
	}
	keyBytes, err := keyutil.MarshalPrivateKeyToPEM(certificateStore.Certificate.PrivateKey)
	if err != nil {
		logger.Error(err, "err encoding key")
		return ctrl.Result{}, err
	}
	stackedCert.Write(keyBytes)

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubemarkMachine.Name,
			Namespace: kubemarkMachine.Namespace,
		},
		Data: map[string][]byte{
			"kubeconfig": kubeconfig,
			"cert.pem":   stackedCert.Bytes(),
		},
	}
	if err := r.Create(ctx, secret); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			logger.Error(err, "failed to create secret")
			return ctrl.Result{}, err
		}
	}
	version := machine.Spec.Version
	if version == nil {
		err := errors.New("Machine has no spec.version")
		logger.Error(err, "")
		return ctrl.Result{}, err
	}

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubemarkMachine.Name,
			Labels:    map[string]string{"app": kubemarkName},
			Namespace: kubemarkMachine.Namespace,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  kubemarkName,
					Image: fmt.Sprintf("%s:%s", r.KubemarkImage, *version),
					Args: []string{
						"--v=3",
						"--morph=kubelet",
						"--log-file=/var/log/kubelet.log",
						"--logtostderr=false",
						fmt.Sprintf("--name=%s", kubemarkMachine.Name),
					},
					Command: []string{"/kubemark"},
					SecurityContext: &v1.SecurityContext{
						Privileged: pointer.BoolPtr(true),
					},
					VolumeMounts: []v1.VolumeMount{
						{
							MountPath: "/kubeconfig",
							Name:      "kubeconfig",
						},
					},
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceCPU:    resource.MustParse("40m"),
							v1.ResourceMemory: resource.MustParse("10240Ki"),
						},
					},
				},
			},
			Tolerations: []v1.Toleration{
				{
					Key:    "node-role.kubernetes.io/master",
					Effect: v1.TaintEffectNoSchedule,
				},
			},
			Volumes: []v1.Volume{
				{
					Name: "kubeconfig",
					VolumeSource: v1.VolumeSource{
						Secret: &v1.SecretVolumeSource{
							SecretName: secret.Name,
						},
					},
				},
			},
		},
	}

	if err = r.Create(ctx, pod); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			logger.Error(err, "failed to create pod")
			return ctrl.Result{}, err
		}
	}

	machine.Spec.ProviderID = pointer.StringPtr(fmt.Sprintf("kubemark://%s", kubemarkMachine.Name))
	kubemarkMachine.Status.Ready = true

	return ctrl.Result{}, nil
}

func (r *KubemarkMachineReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	clusterToKubemarkMachines, err := util.ClusterToObjectsMapper(mgr.GetClient(), &infrav1.KubemarkMachineList{}, mgr.GetScheme())
	if err != nil {
		return err
	}
	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.KubemarkMachine{}).
		Watches(
			&source.Kind{Type: &clusterv1.Machine{}},
			handler.EnqueueRequestsFromMapFunc(util.MachineToInfrastructureMapFunc(infrav1.GroupVersion.WithKind("KubemarkMachine"))),
		).
		Build(r)
	if err != nil {
		return err
	}
	return c.Watch(
		&source.Kind{Type: &clusterv1.Cluster{}},
		handler.EnqueueRequestsFromMapFunc(clusterToKubemarkMachines),
		predicates.ClusterUnpausedAndInfrastructureReady(ctrl.LoggerFrom(ctx)),
	)
}

func generateCertificateKubeconfig(bootstrapClientConfig *restclient.Config, pemPath string) ([]byte, error) {
	// Get the CA data from the bootstrap client config.
	caFile, caData := bootstrapClientConfig.CAFile, []byte{}
	if len(caFile) == 0 {
		caData = bootstrapClientConfig.CAData
	}

	// Build resulting kubeconfig.
	kubeconfigData := &clientcmdapi.Config{
		// Define a cluster stanza based on the bootstrap kubeconfig.
		Clusters: map[string]*clientcmdapi.Cluster{"default-cluster": {
			Server:                   bootstrapClientConfig.Host,
			InsecureSkipTLSVerify:    bootstrapClientConfig.Insecure,
			CertificateAuthority:     caFile,
			CertificateAuthorityData: caData,
		}},
		// Define auth based on the obtained client cert.
		AuthInfos: map[string]*clientcmdapi.AuthInfo{"default-auth": {
			ClientCertificate: pemPath,
			ClientKey:         pemPath,
		}},
		// Define a context that connects the auth info and cluster, and set it as the default
		Contexts: map[string]*clientcmdapi.Context{"default-context": {
			Cluster:   "default-cluster",
			AuthInfo:  "default-auth",
			Namespace: "default",
		}},
		CurrentContext: "default-context",
	}

	// Marshal to disk
	return runtime.Encode(clientcmdlatest.Codec, kubeconfigData)
}

func getRemoteCluster(ctx context.Context, logger logr.Logger, mgmtClient client.Client, cluster *clusterv1.Cluster) (*restclient.Config, client.Client, error) {
	restConfig, err := remote.RESTConfig(ctx, mgmtClient, util.ObjectKey(cluster))
	if err != nil {
		logger.Error(err, "error getting restconfig")
		return nil, nil, err
	}
	restConfig.Timeout = 30 * time.Second

	c, err := client.New(restConfig, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		logger.Error(err, "error creating client")
		return nil, nil, err
	}
	return restConfig, c, err
}
