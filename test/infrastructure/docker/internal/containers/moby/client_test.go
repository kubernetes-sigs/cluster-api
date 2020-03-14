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

package client

import (
	"context"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/internal/containers"
)

func TestMobyClient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Moby Spec")
}

var _ = Describe("Moby Client	", func() {
	var moby *Moby
	var err error
	var ctx context.Context

	BeforeEach(func() {
		moby, err = NewMoby()
		Expect(err).NotTo(HaveOccurred())
		ctx = context.Background()
		_, err := moby.Info(ctx)
		if err != nil {
			Skip("docker is likely not available. Please enable and run the test suite again.")
		}
	})

	Context("Requiring an existing container", func() {
		var id string

		BeforeEach(func() {
			runConfig := containers.RunConfig{
				Cmd:   []string{"sleep", "100000"},
				Image: "busybox",
			}
			id, err = moby.Run(ctx, runConfig, containers.HostConfig{}, "")
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			// Ignore the error since it may already have been removed
			_ = moby.Remove(ctx, id)
		})

		Describe("Container inspection", func() {
			It("should successfully inspect a running container", func() {
				inspection, err := moby.Inspect(ctx, id)
				Expect(err).NotTo(HaveOccurred())
				Expect(inspection.IPv4).NotTo(BeEmpty())
			})
		})

		Describe("Container removal", func() {
			It("should be able to remove a running container", func() {
				Expect(moby.Remove(ctx, id)).To(Succeed())
			})
		})

		Describe("Container kill", func() {
			It("should be able to send a signal to a running container", func() {
				Expect(moby.Kill(ctx, id, "SIGKILL")).To(Succeed())
			})
		})

		Describe("Write file", func() {
			It("should be able to write a file then read it back", func() {
				Expect(moby.WriteFile(ctx, id, "my-file", strings.NewReader("my contents"))).To(Succeed())
				execConfig := containers.ExecConfig{
					Cmd: []string{"cat", "/tmp/my-file"},
				}
				result, err := moby.Exec(ctx, id, execConfig)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.StdOut).To(Equal("my contents"))
			})
		})

		Describe("Container list", func() {
			It("should be able to list running containers", func() {
				list, err := moby.ContainerList(ctx, containers.ListOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(list).To(HaveLen(1))
			})
		})

	})
})
