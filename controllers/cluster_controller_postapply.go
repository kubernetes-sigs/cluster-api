package controllers

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"github.com/pkg/errors"
	"io"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controllers/remote"
	"sigs.k8s.io/cluster-api/util/secret"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

var (
	errUninitializedControlPlane = errors.New("control plane is uninitialized")
)

func (r *ClusterReconciler) reconcilePostApply(ctx context.Context, cluster *clusterv1.Cluster) error {
	logger := r.Log.WithValues("cluster", cluster.Name, "namespace", cluster.Namespace)

	if !cluster.Status.ControlPlaneInitialized {
		return errUninitializedControlPlane
	}

	c, err := remote.NewClusterClient(ctx, r.Client, cluster, r.scheme)
	// Failed to get remote cluster client: Kubeconfig secret may be missing for the cluster.
	if err != nil {
		return err
	}

	addons := cluster.Spec.PostApplyAddons
	for _, addon := range addons {
		annotations := cluster.GetAnnotations()
		PostApplyAnnotation := clusterv1.PostApplyAnnotationPrefix + "-" + addon.Namespace + "-" + addon.Name
		// Do not apply if the current addon has already been applied.
		if _, exists := annotations[PostApplyAnnotation]; exists {
			continue
		}
		logger.Info("Trying to post-apply ", "secret", addon.Name)

		typedName := types.NamespacedName{Name: addon.Name, Namespace: addon.Namespace}

		addonSecret, err := secret.GetAnySecretFromNamespacedName(ctx, r.Client, typedName)
		if err != nil {
			logger.Error(err, "Failed to fetch PostApply addon secret", "Secret", addon.Namespace+"/"+addon.Name)
			continue
		}

		data, ok := addonSecret.Data[secret.PostApplyDataKey]
		if !ok {
			err = errors.New(fmt.Sprintf("Missing key %q in secret %q", secret.PostApplyDataKey, addon.Name))
			logger.Error(err, "Failed to retrieve PostApply data")
			continue
		}
		err = ApplyYAMLWithNamespace(ctx, c, data, "")
		if err != nil {
			logger.Error(err, "Failed applying PostApply secret to cluster")
			continue
		}

		logger.Info("Successfully applied post-apply addon", "Secret", addon.Namespace+"/"+addon.Name)

		cluster.GetAnnotations()

		if annotations == nil {
			annotations = make(map[string]string)
		}
		annotations[PostApplyAnnotation] = time.Now().Format(time.RFC3339)
	}
	return nil
}

// ApplyYAMLWithNamespace applies the provided YAML as unstructured data with the given client.
// The data may be a single YAML document or multidoc YAML. This function is idempotent.
// When a non-empty namespace is provided then all objects are assigned the namespace prior to being created.
func ApplyYAMLWithNamespace(ctx context.Context, c client.Client, data []byte, namespace string) error {
	return ForEachObjectInYAML(ctx, c, data, namespace, func(ctx context.Context, c client.Client, obj *unstructured.Unstructured) error {
		// Create the object on the API server.
		if err := c.Create(ctx, obj); err != nil {
			// The create call is idempotent, so if the object already exists
			// then do not consider it to be an error.
			if !apierrors.IsAlreadyExists(err) {
				return errors.Wrapf(
					err,
					"failed to create object %s %s/%s",
					obj.GroupVersionKind(),
					obj.GetNamespace(),
					obj.GetName())
			}
		}
		return nil
	})
}

// ForEachObjectInYAMLActionFunc is a function that is executed against each
// object found in a YAML document.
// When a non-empty namespace is provided then the object is assigned the
// namespace prior to any other actions being performed with or to the object.
type ForEachObjectInYAMLActionFunc func(context.Context, client.Client, *unstructured.Unstructured) error

// ForEachObjectInYAML excutes actionFn for each object in the provided YAML.
// If an error is returned then no further objects are processed.
// The data may be a single YAML document or multidoc YAML.
// When a non-empty namespace is provided then all objects are assigned the
// the namespace prior to any other actions being performed with or to the
// object.
func ForEachObjectInYAML(
	ctx context.Context,
	c client.Client,
	data []byte,
	namespace string,
	actionFn ForEachObjectInYAMLActionFunc) error {

	chanObj, chanErr := DecodeYAML(data)
	for {
		select {
		case obj := <-chanObj:
			if obj == nil {
				return nil
			}
			if namespace != "" {
				obj.SetNamespace(namespace)
			}
			if err := actionFn(ctx, c, obj); err != nil {
				return err
			}
		case err := <-chanErr:
			if err == nil {
				return nil
			}
			return errors.Wrap(err, "received error while decoding yaml to delete from server")
		}
	}
}

// DecodeYAML unmarshals a YAML document or multidoc YAML as unstructured
// objects, placing each decoded object into a channel.
func DecodeYAML(data []byte) (<-chan *unstructured.Unstructured, <-chan error) {

	var (
		chanErr        = make(chan error)
		chanObj        = make(chan *unstructured.Unstructured)
		multidocReader = utilyaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(data)))
	)

	go func() {
		defer close(chanErr)
		defer close(chanObj)

		// Iterate over the data until Read returns io.EOF. Every successful
		// read returns a complete YAML document.
		for {
			buf, err := multidocReader.Read()
			if err != nil {
				if err == io.EOF {
					return
				}
				chanErr <- errors.Wrap(err, "failed to read yaml data")
				return
			}

			// Do not use this YAML doc if it is unkind.
			var typeMeta runtime.TypeMeta
			if err := yaml.Unmarshal(buf, &typeMeta); err != nil {
				continue
			}
			if typeMeta.Kind == "" {
				continue
			}

			// Define the unstructured object into which the YAML document will be
			// unmarshaled.
			obj := &unstructured.Unstructured{
				Object: map[string]interface{}{},
			}

			// Unmarshal the YAML document into the unstructured object.
			if err := yaml.Unmarshal(buf, &obj.Object); err != nil {
				chanErr <- errors.Wrap(err, "failed to unmarshal yaml data")
				return
			}

			// Place the unstructured object into the channel.
			chanObj <- obj
		}
	}()

	return chanObj, chanErr
}
