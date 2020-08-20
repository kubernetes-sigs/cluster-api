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

package ignition

import (
	"encoding/json"
	"errors"
	"github.com/coreos/ignition/config/util"
	ignTypes "github.com/coreos/ignition/config/v2_2/types"
	"github.com/coreos/ignition/config/validate"
	"github.com/vincent-petithory/dataurl"
	"net/url"
	"reflect"
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha3"
	kubeadmv1beta1 "sigs.k8s.io/cluster-api/exp/kubeadm-ignition/types/v1beta1"
	"strconv"
)

type TemplateBackend interface {
	getIgnitionConfigTemplate(node *Node) (*ignTypes.Config, error)
	applyConfig(out *ignTypes.Config) (*ignTypes.Config, error)
}

func NewFactory(backend TemplateBackend) *Factory {
	return &Factory{backend}
}

type Factory struct {
	dataSource TemplateBackend
}

func (factory *Factory) GenerateUserData(node *Node) ([]byte, error) {
	out, err := factory.dataSource.getIgnitionConfigTemplate(node)
	if err != nil {
		return nil, err
	}

	config, err := factory.BuildIgnitionConfig(out, node)
	if err != nil {
		return nil, err
	}
	config, err = factory.dataSource.applyConfig(config)
	if err != nil {
		return nil, err
	}
	return json.Marshal(config)
}

func (factory *Factory) BuildIgnitionConfig(out *ignTypes.Config, node *Node) (*ignTypes.Config, error) {
	out.Systemd = getSystemd(node.Services)
	var err error
	if out.Storage, err = getStorage(node.Files); err != nil {
		return nil, err
	}
	//validate output
	validationReport := validate.ValidateWithoutSource(reflect.ValueOf(*out))
	if validationReport.IsFatal() {
		return nil, errors.New(validationReport.String())
	}
	return out, nil
}

func getStorage(files []v1alpha3.File) (out ignTypes.Storage, err error) {
	for _, file := range files {
		newFile := ignTypes.File{
			Node: ignTypes.Node{
				Filesystem: "root",
				Path:       file.Path,
				Overwrite:  boolToPtr(true),
			},
			FileEmbedded1: ignTypes.FileEmbedded1{
				Append: false,
				Mode:   intToPtr(DefaultFileMode),
			},
		}
		if file.Permissions != "" {
			value, err := strconv.ParseInt(file.Permissions, 8, 32)
			if err != nil {
				return ignTypes.Storage{}, err
			}
			newFile.FileEmbedded1.Mode = util.IntToPtr(int(value))
		}
		if file.Content != "" {
			newFile.Contents = ignTypes.FileContents{
				Source: (&url.URL{
					Scheme: "data",
					Opaque: "," + dataurl.EscapeString(file.Content),
				}).String(),
			}
		}
		out.Files = append(out.Files, newFile)
	}
	return out, nil
}

func getSystemd(services []kubeadmv1beta1.ServiceUnit) (out ignTypes.Systemd) {
	for _, service := range services {
		newUnit := ignTypes.Unit{
			Name:     service.Name,
			Enabled:  boolToPtr(service.Enabled),
			Contents: service.Content,
		}

		for _, dropIn := range service.Dropins {
			newUnit.Dropins = append(newUnit.Dropins, ignTypes.SystemdDropin{
				Name:     dropIn.Name,
				Contents: dropIn.Content,
			})
		}

		out.Units = append(out.Units, newUnit)
	}
	return
}
