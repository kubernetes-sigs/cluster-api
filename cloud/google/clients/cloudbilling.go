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
	"google.golang.org/api/cloudbilling/v1"
	"net/http"
	"net/url"
)

// CloudBillingService is a pass through wrapper for google.golang.org/api/cloudbilling/v1/
// The purpose of the CloudBillingService's wrap of the cloudbilling.APIService client is to enable tests to mock this struct and control behavior.
type CloudBillingService struct {
	service *cloudbilling.APIService
}

func NewCloudBillingService() (*CloudBillingService, error) {
	client, err := google.DefaultClient(context.TODO(), cloudbilling.CloudPlatformScope)
	if err != nil {
		return nil, err
	}
	return NewCloudBillingServiceForClient(client)
}

func NewCloudBillingServiceForClient(client *http.Client) (*CloudBillingService, error) {
	service, err := cloudbilling.New(client)
	if err != nil {
		return nil, err
	}
	return &CloudBillingService{
		service: service,
	}, nil
}

func NewCloudBillingServiceForURL(client *http.Client, baseURL string) (*CloudBillingService, error) {
	billingService, err := NewCloudBillingServiceForClient(client)
	if err != nil {
		return nil, err
	}
	url, err := url.Parse(billingService.service.BasePath)
	if err != nil {
		return nil, err
	}
	billingService.service.BasePath = baseURL + url.Path
	return billingService, err
}

func (cbs *CloudBillingService) BillingAccountsList() ([]*cloudbilling.BillingAccount, error) {
	var accounts []*cloudbilling.BillingAccount
	request := cbs.service.BillingAccounts.List()
	for {
		response, err := request.Do()
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, response.BillingAccounts...)
		if response.NextPageToken == "" {
			break
		}
		request.PageToken(response.NextPageToken)
	}
	return accounts, nil
}

// A pass through wrapper for cloudbilling.Projects.GetBillingInfo(...)
func (cbs *CloudBillingService) ProjectsGetBillingInfo(name string) (*cloudbilling.ProjectBillingInfo, error) {
	name = NormalizeProjectNameOrId(name)
	return cbs.service.Projects.GetBillingInfo(name).Do()
}

// A pass through wrapper for cloudbilling.Projects.UpdateBillingInfo(...)
func (cbs *CloudBillingService) ProjectsUpdateBillingInfo(name string, projectBillingInfo *cloudbilling.ProjectBillingInfo) (*cloudbilling.ProjectBillingInfo, error) {
	name = NormalizeProjectNameOrId(name)
	return cbs.service.Projects.UpdateBillingInfo(name, projectBillingInfo).Do()
}
