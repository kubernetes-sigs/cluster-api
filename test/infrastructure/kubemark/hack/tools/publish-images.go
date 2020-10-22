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

package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"

	"github.com/Masterminds/semver"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/spf13/pflag"
)

type PublishImages struct {
	ImageName       string
	KubeDir         string
	StartingVersion string
	Push            bool
}

func bindFlags(p *PublishImages) error {
	flags := pflag.NewFlagSet("publish-images", pflag.ExitOnError)
	flags.StringVar(&p.ImageName, "image-name", "kubemark", "name for the image to be tagged")
	flags.StringVar(&p.KubeDir, "kube-dir", "", "path to your kubernetes checkout")
	flags.StringVar(&p.StartingVersion, "starting-version", "1.17.0", "version (inclusive) from which to start building images")
	flags.BoolVar(&p.Push, "push", false, "whether the script should push the images after building")
	return flags.Parse(os.Args)
}

func main() {
	p := &PublishImages{}
	if err := bindFlags(p); err != nil {
		log.Fatalf("failed to parse flags: %v", err)
	}
	if p.KubeDir == "" {
		log.Fatalf("--kube-dir must be set")
	}

	constraint, err := semver.NewConstraint(fmt.Sprintf(">= %s", p.StartingVersion))
	if err != nil {
		log.Fatalf("err parsing %s: %v", p.StartingVersion, err)
	}

	cmd := exec.Command("git", "-C", p.KubeDir, "fetch", "--tags")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("err fetching remote tags: %v", err)
	}
	repo, err := name.NewRepository("k8s.gcr.io/kube-proxy")
	if err != nil {
		log.Fatalf("err parsing %v", err)
	}
	tags, err := remote.List(repo)
	if err != nil {
		log.Fatalf("err fetching tags %v", err)
	}
	for _, t := range tags {
		v, err := semver.NewVersion(t)
		if err != nil {
			log.Printf("err parsing %s, %v", t, err)
		}
		if constraint.Check(v) {
			exists, err := p.imageExists(v)
			if err != nil {
				log.Fatalf("err checking if image exists for %v: %v", v, err)
			}
			if exists {
				log.Printf("skipping version %s, image already exists", v.Original())
				continue
			}
			if err := p.dockerBuild(v); err != nil {
				log.Fatalf("err building docker image for %v: %v", v, err)
			}
		}
	}
	if err := p.dockerPush(); err != nil {
		log.Fatalf("err pushing docker image: %v", err)
	}
}

func (p *PublishImages) dockerBuild(v *semver.Version) error {
	cmd := exec.Command("git", "-C", p.KubeDir, "fetch", "--tags")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	cmd = exec.Command("git", "-C", p.KubeDir, "checkout", v.Original())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	cmd = exec.Command("make", "-C", p.KubeDir, "clean", "all", "WHAT=cmd/kubemark")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	dockerfilePath := path.Join(p.KubeDir, "cluster/images/kubemark")
	symlinkPath := path.Join(dockerfilePath, "kubemark")
	if err := os.Link(path.Join(p.KubeDir, "_output/bin/kubemark"), symlinkPath); err != nil {
		return err
	}
	defer func() {
		os.Remove(symlinkPath)
	}()
	cmd = exec.Command("docker", "build", "--pull",
		fmt.Sprintf("--tag=%s:%s", p.ImageName, v.Original()),
		dockerfilePath,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (p *PublishImages) dockerPush() error {
	if !p.Push {
		return nil
	}
	cmd := exec.Command("docker", "push", p.ImageName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (p *PublishImages) imageExists(v *semver.Version) (bool, error) {
	cmd := exec.Command("docker", "image", "inspect",
		fmt.Sprintf("%s:%s", p.ImageName, v.Original()))
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			if exitError.ExitCode() == 1 {
				return false, nil
			}
		}
		return false, err
	}
	return true, nil
}
