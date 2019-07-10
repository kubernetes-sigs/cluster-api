package controlplane

import (
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/kind/pkg/cluster"
)

// TODO: Add version number?
const userAgent = "capdctl"

func GetKubeconfig(clusterName string) (*rest.Config, error) {
	ctx := cluster.NewContext(clusterName)
	path := ctx.KubeConfigPath()

	cfg, err := clientcmd.BuildConfigFromFlags("", path)
	if err != nil {
		return nil, err
	}

	return rest.AddUserAgent(cfg, userAgent), err
}
