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
	"bytes"
	"encoding/json"
	"errors"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	ignTypes "github.com/coreos/ignition/config/v2_2/types"
	"github.com/google/uuid"
	"net/url"
	"strings"
	"time"
)

var (
	baseIgnitionUri = map[string]string{
		"v1.15.11": "ignition-config/k8s-v1.15.11.ign",
		"v1.16.8":  "ignition-config/k8s-v1.16.8.ign",
		"v1.17.4":  "ignition-config/k8s-v1.17.4.ign",
	}
)

const (
	KubernetesDefaultVersion = "v1.17.4"
	IngitionSchemaVersion    = "2.2.0"
)

func NewS3TemplateBackend(userdataDir string, userDataBucket string) (*S3TemplateBackend, error) {
	session, err := session.NewSession()
	if err != nil {
		ignitionLogger.Error(err, "failed to initialize s3 session")
		return nil, err
	}
	return &S3TemplateBackend{
		userdataDir:    userdataDir,
		userDataBucket: userDataBucket,
		session:        session,
	}, nil
}

type S3TemplateBackend struct {
	userdataDir    string
	userDataBucket string
	session        *session.Session
}

func (factory *S3TemplateBackend) getIgnitionConfigTemplate(node *Node) (*ignTypes.Config, error) {
	templateConfigUri, ok := baseIgnitionUri[node.Version]
	if !ok {
		err := errors.New("kubernetes version is not supported.")
		ignitionLogger.Error(err, "kubernetes version is not supported.")
		templateConfigUri = baseIgnitionUri[KubernetesDefaultVersion]
	}
	baseIgnitionUrl := &url.URL{
		Scheme: "s3",
		Host:   factory.userDataBucket,
		Path:   templateConfigUri,
	}
	out := factory.getIgnitionBaseConfig()
	out.Ignition.Config = ignTypes.IgnitionConfig{
		Append: []ignTypes.ConfigReference{
			{
				Source: baseIgnitionUrl.String(),
			},
		},
	}
	return out, nil
}

func (factory *S3TemplateBackend) applyConfig(config *ignTypes.Config) (*ignTypes.Config, error) {
	userdata, err := json.Marshal(config)
	if err != nil {
		ignitionLogger.Error(err, "failed to marshal ignition file")
		return nil, err
	}

	uploader := s3manager.NewUploader(factory.session)
	filePath := strings.Join([]string{factory.userdataDir, uuid.New().String()}, "/")
	_, err = uploader.Upload(&s3manager.UploadInput{
		Body:         bytes.NewReader(userdata),
		Bucket:       aws.String(factory.userDataBucket),
		Expires:      aws.Time(time.Now().Add(time.Hour * 168)),
		Key:          aws.String(filePath),
		StorageClass: aws.String(s3.StorageClassIntelligentTiering),
	})
	if err != nil {
		ignitionLogger.Error(err, "failed to upload ignition file to bucket")
		return nil, err
	}

	userDataUrl := url.URL{
		Scheme: "s3",
		Host:   factory.userDataBucket,
		Path:   filePath,
	}
	out := factory.getIgnitionBaseConfig()
	out.Ignition.Config = ignTypes.IgnitionConfig{
		Replace: &ignTypes.ConfigReference{
			Source: userDataUrl.String(),
		},
	}
	return out, nil
}

func (factory *S3TemplateBackend) getIgnitionBaseConfig() *ignTypes.Config {
	return &ignTypes.Config{
		Ignition: ignTypes.Ignition{
			Version: IngitionSchemaVersion,
		},
	}
}
