package converter

import clusterv1beta1 "sigs.k8s.io/cluster-api/api/core/v1beta1"

func dropEmptyStringsClusterClass(dst *clusterv1beta1.ClusterClass) {
	if dst.Spec.InfrastructureNamingStrategy != nil {
		dropEmptyString(&dst.Spec.InfrastructureNamingStrategy.Template)
	}

	if dst.Spec.ControlPlane.NamingStrategy != nil {
		dropEmptyString(&dst.Spec.ControlPlane.NamingStrategy.Template)
	}

	for i, md := range dst.Spec.Workers.MachineDeployments {
		if md.NamingStrategy != nil {
			dropEmptyString(&md.NamingStrategy.Template)
		}
		dropEmptyString(&md.FailureDomain)
		dst.Spec.Workers.MachineDeployments[i] = md
	}

	for i, mp := range dst.Spec.Workers.MachinePools {
		if mp.NamingStrategy != nil {
			dropEmptyString(&mp.NamingStrategy.Template)
		}

		dst.Spec.Workers.MachinePools[i] = mp
	}

	for i, p := range dst.Spec.Patches {
		dropEmptyString(&p.EnabledIf)
		if p.External != nil {
			dropEmptyString(&p.External.GenerateExtension)
			dropEmptyString(&p.External.ValidateExtension)
			dropEmptyString(&p.External.DiscoverVariablesExtension)
		}

		for j, d := range p.Definitions {
			for k, jp := range d.JSONPatches {
				if jp.ValueFrom != nil {
					dropEmptyString(&jp.ValueFrom.Variable)
					dropEmptyString(&jp.ValueFrom.Template)
				}
				d.JSONPatches[k] = jp
			}
			p.Definitions[j] = d
		}

		dst.Spec.Patches[i] = p
	}
}

func dropEmptyStringsMachineSpec(spec *clusterv1beta1.MachineSpec) {
	dropEmptyString(&spec.Version)
	dropEmptyString(&spec.ProviderID)
	dropEmptyString(&spec.FailureDomain)
}
