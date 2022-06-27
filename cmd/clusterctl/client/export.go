package client

import (
	"bytes"
	"fmt"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	logf "sigs.k8s.io/cluster-api/cmd/clusterctl/log"
	"sigs.k8s.io/cluster-api/util/yaml"
)

// ExportOptions carries the options supported by Export.
type ExportOptions struct {
	// CoreProvider version (e.g. cluster-api:v0.3.0) to add to the management cluster. If unspecified, the
	// cluster-api core provider's latest release is used.
	CoreProvider string

	// BootstrapProviders and versions (e.g. kubeadm:v0.3.0) to add to the management cluster.
	// If unspecified, the kubeadm bootstrap provider's latest release is used.
	BootstrapProviders []string

	// InfrastructureProviders and versions (e.g. aws:v0.5.0) to add to the management cluster.
	InfrastructureProviders []string

	// ControlPlaneProviders and versions (e.g. kubeadm:v0.3.0) to add to the management cluster.
	// If unspecified, the kubeadm control plane provider latest release is used.
	ControlPlaneProviders []string

	// TargetNamespace defines the namespace where the providers should be deployed. If unspecified, each provider
	// will be installed in a provider's default namespace.
	TargetNamespace string

	// SkipTemplateProcess allows for skipping the call to the template processor, including also variable replacement in the component YAML.
	// NOTE this works only if the rawYaml is a valid yaml by itself, like e.g when using envsubst/the simple processor.
	SkipTemplateProcess bool
}

func (c *clusterctlClient) Export(options ExportOptions) ([]byte, error) {
	log := logf.Log

	// gets access to the management cluster
	clusterClient, err := c.clusterClientFactory(ClusterClientFactoryInput{})
	if err != nil {
		return nil, err
	}

	log.Info("Adding default providers")
	if options.CoreProvider == "" {
		options.CoreProvider = config.ClusterAPIProviderName
	}
	if len(options.BootstrapProviders) == 0 {
		options.BootstrapProviders = append(options.BootstrapProviders, config.KubeadmBootstrapProviderName)
	}
	if len(options.ControlPlaneProviders) == 0 {
		options.ControlPlaneProviders = append(options.ControlPlaneProviders, config.KubeadmControlPlaneProviderName)
	}

	initOptions := InitOptions{
		skipTemplateProcess:     options.SkipTemplateProcess,
		CoreProvider:            options.CoreProvider,
		BootstrapProviders:      options.BootstrapProviders,
		ControlPlaneProviders:   options.ControlPlaneProviders,
		InfrastructureProviders: options.InfrastructureProviders,
		TargetNamespace:         options.TargetNamespace,
	}

	// create an installer service, add the requested providers to the install queue and then perform validation
	// of the target state of the management cluster before starting the installation.
	installer, err := c.setupInstaller(clusterClient, initOptions)
	if err != nil {
		return nil, err
	}

	// Before installing the providers, validates the management cluster resulting by the planned installation. The following checks are performed:
	// - There should be only one instance of the same provider.
	// - All the providers must support the same API Version of Cluster API (contract)
	if err := installer.Validate(true); err != nil {
		return nil, err
	}

	// Before installing the providers, ensure the cert-manager Webhook is in place.
	certManager := clusterClient.CertManager()
	certManagerComp, err := certManager.GetManifestObjects()
	if err != nil {
		return nil, err
	}

	data := bytes.Buffer{}

	// Write the certmanager
	data.WriteString("# Start: cert-manager\n")
	cmData, err := yaml.FromUnstructured(certManagerComp)
	if err != nil {
		return nil, err
	}
	data.Write(cmData)
	data.WriteString("\n# End: cert-manager\n")
	data.WriteString("---\n")

	components := installer.Components()
	if err != nil {
		return nil, err
	}

	// Write the core first
	for _, comp := range components {
		if comp.Type() == v1alpha3.CoreProviderType {
			data.WriteString("# Start: core\n")

			coreData, err := yaml.FromUnstructured(comp.Objs())
			if err != nil {
				return nil, err
			}
			data.Write(coreData)

			data.WriteString("\n# End: core\n")
			data.WriteString("---\n")
		}
	}

	// Write the rest of the components
	for _, comp := range components {
		if comp.Type() != v1alpha3.CoreProviderType {
			data.WriteString(fmt.Sprintf("# Start: provider %s (%s)\n", comp.Name(), comp.Type()))

			coreData, err := yaml.FromUnstructured(comp.Objs())
			if err != nil {
				return nil, err
			}
			data.Write(coreData)

			data.WriteString(fmt.Sprintf("\n# End: provider %s (%s)\n", comp.Name(), comp.Type()))
			data.WriteString("---\n")
		}
	}

	return data.Bytes(), nil
}
