/*
Copyright 2018 The Kubernetes Authors.

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

package clients

import (
	"context"
	"fmt"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/cloudresourcemanager/v1"
	"net/http"
	"net/url"
)

// CloudResourceManagerService is a pass through wrapper for google.golang.org/api/cloudresourcemanager/v1/
// The purpose of the CloudResourceManagerService's wrap of the cloudresourcemanager.Service client is to enable tests to
// mock this struct and control behavior.
type CloudResourceManagerService struct {
	service *cloudresourcemanager.Service
}

func NewCloudResourceManagerService() (*CloudResourceManagerService, error) {
	client, err := google.DefaultClient(context.TODO(), cloudresourcemanager.CloudPlatformScope)
	if err != nil {
		return nil, err
	}
	return NewCloudResourceManagerServiceForClient(client)
}

func NewCloudResourceManagerServiceForClient(client *http.Client) (*CloudResourceManagerService, error) {
	service, err := cloudresourcemanager.New(client)
	if err != nil {
		return nil, err
	}
	return &CloudResourceManagerService{
		service: service,
	}, nil
}

func NewCloudResourceManagerServiceForURL(client *http.Client, baseURL string) (*CloudResourceManagerService, error) {
	crmService, err := NewCloudResourceManagerServiceForClient(client)
	if err != nil {
		return nil, err
	}
	url, err := url.Parse(crmService.service.BasePath)
	if err != nil {
		return nil, err
	}
	crmService.service.BasePath = baseURL + url.Path
	return crmService, err
}

// A pass through wrapper for cloudresourcemanager.Operations.Get(...)
func (crm *CloudResourceManagerService) OperationsGet(name string) (*cloudresourcemanager.Operation, error) {
	return crm.service.Operations.Get(name).Do()
}

// A pass through wrapper for cloudresourcemanager.Projects.Create(...)
func (crm *CloudResourceManagerService) ProjectsCreate(project *cloudresourcemanager.Project) (*cloudresourcemanager.Operation, error) {
	return crm.service.Projects.Create(project).Do()
}

// A pass through wrapper for cloudresourcemanager.Projects.Get(...)
func (crm *CloudResourceManagerService) ProjectsGet(id string) (*cloudresourcemanager.Project, error) {
	project, err := crm.service.Projects.Get(id).Do()
	if err != nil {
		return nil, fmt.Errorf("unable to get project with id '%v': %v", id, err)
	}
	return project, nil
}

// Calls cloudresourcemanager.Projects.List(...) with the given filter applied. Paginated results are combined into a single
// slice. When not applying a filter use with care as every visible project will be returned.
func (crm *CloudResourceManagerService) ProjectsList(filter string) ([]*cloudresourcemanager.Project, error) {
	var projects []*cloudresourcemanager.Project
	request := crm.service.Projects.List().Filter(filter)
	for {
		response, err := request.Do()
		if err != nil {
			return nil, err
		}
		projects = append(projects, response.Projects...)
		if response.NextPageToken == "" {
			break
		}
		request.PageToken(response.NextPageToken)
	}
	return projects, nil
}
