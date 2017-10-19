// Copyright © 2017 The Kubicorn Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package awsSdkGo

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/s3"
)

type Sdk struct {
	Ec2 *ec2.EC2
	S3  *s3.S3
	ASG *autoscaling.AutoScaling
}

func NewSdk(region string, profile string) (*Sdk, error) {
	sdk := &Sdk{}
	session, err := session.NewSessionWithOptions(session.Options{
		Config: aws.Config{Region: aws.String(region)},
		// Support MFA when authing using assumed roles.
		SharedConfigState:       session.SharedConfigEnable,
		AssumeRoleTokenProvider: stscreds.StdinTokenProvider,
		Profile:                 profile,
	})
	if err != nil {
		return nil, err
	}
	sdk.Ec2 = ec2.New(session)
	sdk.ASG = autoscaling.New(session)
	sdk.S3 = s3.New(session)
	return sdk, nil
}
