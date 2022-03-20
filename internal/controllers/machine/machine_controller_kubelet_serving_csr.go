package machine

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"time"

	certificatesv1 "k8s.io/api/certificates/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/remote"
	"sigs.k8s.io/cluster-api/util"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// KubeletServingSignerName defines the name for the kubelet serving certificate signer
	KubeletServingSignerName = "kubernetes.io/kubelet-serving"

	// NodesPrefix defines the prefix name for a node.
	NodesPrefix = "system:node:"

	// NodesGroup defines the group name for a node.
	NodesGroup = "system:nodes"
)

func (r *Reconciler) reconcileKubletServingCertificateSigningRequest(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx, "machine", machine.Name, "namespace", machine.Namespace)
	log = log.WithValues("cluster", cluster.Name)

	// TODO can NodeRef be nil at this point?
	nodeRef := machine.Status.NodeRef

	remoteClient, err := r.Tracker.GetClient(ctx, util.ObjectKey(cluster))
	if err != nil {
		return ctrl.Result{}, err
	}
	restConfig, err := remote.RESTConfig(ctx, controllerName, r.Client, util.ObjectKey(cluster))
	if err != nil {
		return ctrl.Result{}, nil
	}
	kubeClient, err := kubernetes.NewForConfig(restConfig)

	node := corev1.Node{}
	if err := remoteClient.Get(ctx, client.ObjectKey{Name: nodeRef.Name}, &node); err != nil {
		return ctrl.Result{}, err
	}

	// Find the corresponding CSRs for this node
	// TODO: Add filter options
	csrList := certificatesv1.CertificateSigningRequestList{}
	if err := remoteClient.List(ctx, &csrList, &client.ListOptions{
		FieldSelector: fields.AndSelectors(
			fields.OneTermEqualSelector("spec.username", NodesPrefix+node.Name),
			fields.OneTermEqualSelector("spec.signer", KubeletServingSignerName),
		),
	}); err != nil {
		return ctrl.Result{}, err
	}

	for _, csr := range csrList.Items {

		block, _ := pem.Decode(csr.Spec.Request)
		if block.Type != "CERTIFICATE REQUEST" {
			return ctrl.Result{}, fmt.Errorf("Unexpected PEM type")
		}
		cr, err := x509.ParseCertificateRequest(block.Bytes)
		if err != nil {
			return ctrl.Result{}, err
		}

		// TODO: Does this case happen where we have a CSR whose contents dont match the object?
		if cr.Subject.CommonName != csr.Spec.Username {
			return ctrl.Result{}, fmt.Errorf("csr Username didnt match CN")
		}
		for i, org := range cr.Subject.OrganizationalUnit {
			if org != csr.Spec.Groups[i] {
				return ctrl.Result{}, fmt.Errorf("csr Groups didn't match O")
			}
		}

		condition := certificatesv1.CertificateSigningRequestCondition{
			LastUpdateTime: metav1.Time{Time: time.Now()},
		}
		if err := validateCSR(cr, machine.Status.Addresses); err != nil {
			// TODO: we do not want to fail early, just skip this CSR
			condition.Type = certificatesv1.CertificateDenied
			condition.Reason = "CSRValidationFailed"
			condition.Status = "True"
			condition.Message = fmt.Sprintf("Validation failed: %s", err)
		} else {
			condition.Type = certificatesv1.CertificateApproved
			condition.Reason = "CSRValidationSucceed"
			condition.Status = "True"
			condition.Message = "Validation was successful"

		}
		csr.Status.Conditions = append(csr.Status.Conditions, condition)

		if _, err := kubeClient.CertificatesV1().CertificateSigningRequests().UpdateApproval(ctx, csr.Name, &csr, metav1.UpdateOptions{}); err != nil {
			return ctrl.Result{}, err
		}

	}
	return ctrl.Result{}, nil
}

// validateCSR checks that all IPAddresses and DNSNames in csr are what we expect from the Machine definition and that the CSR is part of NodesGroup
// TODO:
func validateCSR(csr *x509.CertificateRequest, addresses []clusterv1.MachineAddress) error {
	containsGroup := false

	for _, group := range csr.Subject.OrganizationalUnit {
		if group == NodesGroup {
			containsGroup = true
			break
		}
	}
	if !containsGroup {
		return fmt.Errorf("need to have %s as group", NodesGroup)
	}
	// At least one DNS or IP subjectAltName must be present.
	if len(csr.IPAddresses)+len(csr.DNSNames) == 0 {
		return fmt.Errorf("need at least one of DNSNames or IPAddresses")
	}
	for _, ipAddress := range csr.IPAddresses {
		found := false
		for _, address := range addresses {
			switch address.Type {
			case clusterv1.MachineExternalIP:
			case clusterv1.MachineInternalIP:
				found = found || ipAddress.Equal(net.IP(address.Address))
			}
		}
		if !found {
			return fmt.Errorf("ip address %s was not allowed", ipAddress)
		}
	}
	for _, dnsName := range csr.DNSNames {
		found := false
		for _, address := range addresses {
			switch address.Type {
			case clusterv1.MachineExternalDNS:
			case clusterv1.MachineInternalDNS:
			case clusterv1.MachineHostName:
				found = found || dnsName == address.Address
			}
		}
		if !found {
			return fmt.Errorf("dns name %s was not allowed", dnsName)
		}
	}
	return nil
}
