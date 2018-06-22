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
	"golang.org/x/net/context"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/servicemanagement/v1"
	"net/http"
	"net/url"
)

// ServiceManagementService is a pass through wrapper for google.golang.org/api/servicemanagement/v1/
// The purpose of the ServiceManagementService's wrap of the servicemanagement.ServiceManagementService client is to enable tests to
// mock and control behavior.
type ServiceManagementService struct {
	service *servicemanagement.APIService
}

func NewServiceManagementService() (*ServiceManagementService, error) {
	client, err := google.DefaultClient(context.TODO(), servicemanagement.CloudPlatformScope)
	if err != nil {
		return nil, err
	}
	return NewServiceManagementServiceForClient(client)
}

func NewServiceManagementServiceForClient(client *http.Client) (*ServiceManagementService, error) {
	service, err := servicemanagement.New(client)
	if err != nil {
		return nil, err
	}
	return &ServiceManagementService{
		service: service,
	}, nil
}

func NewServiceManagementServiceForURL(client *http.Client, baseURL string) (*ServiceManagementService, error) {
	service, err := NewServiceManagementServiceForClient(client)
	if err != nil {
		return nil, err
	}
	url, err := url.Parse(service.service.BasePath)
	if err != nil {
		return nil, err
	}
	service.service.BasePath = baseURL + url.Path
	return service, err
}

func (sms *ServiceManagementService) OperationsGet(name string) (*servicemanagement.Operation, error) {
	return sms.service.Operations.Get(name).Do()
}

func (sms *ServiceManagementService) ServicesEnableForProject(serviceName string, projectId string) (*servicemanagement.Operation, error) {
	enableServiceRequest := servicemanagement.EnableServiceRequest{
		ConsumerId: GetConsumerIdForProject(projectId),
	}
	return sms.service.Services.Enable(serviceName, &enableServiceRequest).Do()
}

// Calls servicemanagement.Services.List(...). If a projectId is supplied results are limited to the services enabled for the given projectId.
// Paginated results are combined into a single slice. Large results are not a concern because the number of GCP services is limited.
func (sms *ServiceManagementService) ServicesList(projectId string) ([]*servicemanagement.ManagedService, error) {
	var services []*servicemanagement.ManagedService
	request := sms.service.Services.List()
	if projectId != "" {
		request.ConsumerId(GetConsumerIdForProject(projectId))
	}
	for {
		response, err := request.Do()
		if err != nil {
			return nil, err
		}
		services = append(services, response.Services...)
		if response.NextPageToken == "" {
			break
		}
		request.PageToken(response.NextPageToken)
	}
	return services, nil
}
