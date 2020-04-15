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

package bootstrap

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"

	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/exec"
	kind "sigs.k8s.io/kind/pkg/cluster"
	kindnodes "sigs.k8s.io/kind/pkg/cluster/nodes"
	kindnodesutils "sigs.k8s.io/kind/pkg/cluster/nodeutils"
)

// CreateKindBootstrapClusterAndLoadImagesInput is the input for CreateKindBootstrapClusterAndLoadImages.
type CreateKindBootstrapClusterAndLoadImagesInput struct {
	// Name of the cluster
	Name string

	// RequiresDockerSock defines if the cluster requires the docker sock
	RequiresDockerSock bool

	// Images to be loaded in the cluster (this is kind specific)
	Images []framework.ContainerImage
}

// CreateKindBootstrapClusterAndLoadImages returns a new Kubernetes cluster with pre-loaded images.
func CreateKindBootstrapClusterAndLoadImages(ctx context.Context, input CreateKindBootstrapClusterAndLoadImagesInput) ClusterProvider {
	Expect(ctx).NotTo(BeNil(), "ctx is required for CreateKindBootstrapClusterAndLoadImages")
	Expect(input.Name).ToNot(BeEmpty(), "Invalid argument. Name can't be empty when calling CreateKindBootstrapClusterAndLoadImages")

	fmt.Fprintf(GinkgoWriter, "Creating a kind cluster with name %q\n", input.Name)

	options := []KindClusterOption{}
	if input.RequiresDockerSock {
		options = append(options, WithDockerSockMount())
	}
	clusterProvider := NewKindClusterProvider(input.Name, options...)
	Expect(clusterProvider).ToNot(BeNil(), "Failed to create a kind cluster")

	clusterProvider.Create(ctx)
	Expect(clusterProvider.GetKubeconfigPath()).To(BeAnExistingFile(), "The kubeconfig file for the kind cluster with name %q does not exists at %q as expected", input.Name, clusterProvider.GetKubeconfigPath())

	LoadImagesToKindCluster(ctx, LoadImagesToKindClusterInput{
		Name:   input.Name,
		Images: input.Images,
	})

	return clusterProvider
}

// LoadImagesToKindClusterInput is the input for LoadImagesToKindCluster.
type LoadImagesToKindClusterInput struct {
	// Name of the cluster
	Name string

	// Images to be loaded in the cluster (this is kind specific)
	Images []framework.ContainerImage
}

// LoadImagesToKindCluster provides a utility for loading images into a kind cluster.
func LoadImagesToKindCluster(ctx context.Context, input LoadImagesToKindClusterInput) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for LoadImagesToKindCluster")
	Expect(input.Name).ToNot(BeEmpty(), "Invalid argument. Name can't be empty when calling LoadImagesToKindCluster")

	for _, image := range input.Images {
		fmt.Fprintf(GinkgoWriter, "Loading image: %q\n", image.Name)
		err := loadImage(ctx, input.Name, image.Name)
		switch image.LoadBehavior {
		case framework.MustLoadImage:
			Expect(err).ToNot(HaveOccurred(), "Failed to load image %q into the kind cluster %q", image.Name, input.Name)
		case framework.TryLoadImage:
			if err != nil {
				fmt.Fprintf(GinkgoWriter, "[WARNING] Unable to load image %q into the kind cluster %q: %v \n", image.Name, input.Name, err)
			}
		}
	}
}

// LoadImage will put a local image onto the kind node
func loadImage(ctx context.Context, cluster, image string) error {
	// Save the image into a tar
	dir, err := ioutil.TempDir("", "image-tar")
	if err != nil {
		return errors.Wrap(err, "failed to create tempdir")
	}
	defer os.RemoveAll(dir)
	imageTarPath := filepath.Join(dir, "image.tar")

	err = save(ctx, image, imageTarPath)
	if err != nil {
		return err
	}

	// Gets the nodes in the cluster
	provider := kind.NewProvider()
	nodeList, err := provider.ListInternalNodes(cluster)
	if err != nil {
		return err
	}

	// Load the image on the selected nodes
	for _, node := range nodeList {
		if err := load(imageTarPath, node); err != nil {
			return err
		}
	}

	return nil
}

// copied from kind https://github.com/kubernetes-sigs/kind/blob/v0.7.0/pkg/cmd/kind/load/docker-image/docker-image.go#L168
// save saves image to dest, as in `docker save`
func save(ctx context.Context, image, dest string) error {
	_, _, err := exec.NewCommand(
		exec.WithCommand("docker"),
		exec.WithArgs("save", "-o", dest, image)).Run(ctx)
	return err
}

// copied from kind https://github.com/kubernetes-sigs/kind/blob/v0.7.0/pkg/cmd/kind/load/docker-image/docker-image.go#L158
// loads an image tarball onto a node
func load(imageTarName string, node kindnodes.Node) error {
	f, err := os.Open(imageTarName)
	if err != nil {
		return errors.Wrap(err, "failed to open image")
	}
	defer f.Close()
	return kindnodesutils.LoadImageArchive(node, f)
}
