/*
Copyright 2017 The Kubernetes Authors.

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

package config

import (
	"time"

	"github.com/spf13/pflag"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

// This is a COPY from kubernetes source tree to avoid importing the entire kubernetes tree.

// LeaderElectionConfiguration defines the configuration of leader election
// clients for components that can run with leader election enabled.
type LeaderElectionConfiguration struct {
	// LeaderElect enables a leader election client to gain leadership
	// before executing the main loop. Enable this when running replicated
	// components for high availability.
	LeaderElect bool
	// LeaseDuration is the duration that non-leader candidates will wait
	// after observing a leadership renewal until attempting to acquire
	// leadership of a led but unrenewed leader slot. This is effectively the
	// maximum duration that a leader can be stopped before it is replaced
	// by another candidate. This is only applicable if leader election is
	// enabled.
	LeaseDuration metav1.Duration
	// RenewDeadline is the interval between attempts by the acting control plane to
	// renew a leadership slot before it stops leading. This must be less
	// than or equal to the lease duration. This is only applicable if leader
	// election is enabled.
	RenewDeadline metav1.Duration
	// RetryPeriod is the duration the clients should wait between attempting
	// acquisition and renewal of a leadership. This is only applicable if
	// leader election is enabled.
	RetryPeriod metav1.Duration
	// ResourceLock indicates the resource object type that will be used to lock
	// during leader election cycles.
	ResourceLock string
}

type Configuration struct {
	Kubeconfig           string
	WorkerCount          int
	leaderElectionConfig *LeaderElectionConfiguration
}

const (
	// DefaultLeaseDuration is the default lease duration for leader election
	DefaultLeaseDuration = 15 * time.Second
	// DefaultRenewDeadline is the default renew duration for leader election
	DefaultRenewDeadline = 10 * time.Second
	// DefaultRetryPeriod is the default retry period for leader election
	DefaultRetryPeriod = 2 * time.Second
)

var ControllerConfig = Configuration{
	WorkerCount: 5, // Default 5 worker.
	leaderElectionConfig: &LeaderElectionConfiguration{
		LeaderElect:   false,
		LeaseDuration: metav1.Duration{Duration: DefaultLeaseDuration},
		RenewDeadline: metav1.Duration{Duration: DefaultRenewDeadline},
		RetryPeriod:   metav1.Duration{Duration: DefaultRetryPeriod},
		ResourceLock:  resourcelock.EndpointsResourceLock,
	},
}

func GetLeaderElectionConfig() *LeaderElectionConfiguration {
	return ControllerConfig.leaderElectionConfig
}

func (c *Configuration) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&c.Kubeconfig, "kubeconfig", c.Kubeconfig, "Path to kubeconfig file with authorization and control plane location information.")
	fs.IntVar(&c.WorkerCount, "workers", c.WorkerCount, "The number of workers for controller.")

	AddLeaderElectionFlags(c.leaderElectionConfig, fs)
}

func AddLeaderElectionFlags(l *LeaderElectionConfiguration, fs *pflag.FlagSet) {
	fs.BoolVar(&l.LeaderElect, "leader-elect", l.LeaderElect, ""+
		"Start a leader election client and gain leadership before "+
		"executing the main loop. Enable this when running replicated "+
		"components for high availability.")
	fs.DurationVar(&l.LeaseDuration.Duration, "leader-elect-lease-duration", l.LeaseDuration.Duration, ""+
		"The duration that non-leader candidates will wait after observing a leadership "+
		"renewal until attempting to acquire leadership of a led but unrenewed leader "+
		"slot. This is effectively the maximum duration that a leader can be stopped "+
		"before it is replaced by another candidate. This is only applicable if leader "+
		"election is enabled.")
	fs.DurationVar(&l.RenewDeadline.Duration, "leader-elect-renew-deadline", l.RenewDeadline.Duration, ""+
		"The interval between attempts by the acting control plane to renew a leadership slot "+
		"before it stops leading. This must be less than or equal to the lease duration. "+
		"This is only applicable if leader election is enabled.")
	fs.DurationVar(&l.RetryPeriod.Duration, "leader-elect-retry-period", l.RetryPeriod.Duration, ""+
		"The duration the clients should wait between attempting acquisition and renewal "+
		"of a leadership. This is only applicable if leader election is enabled.")
	fs.StringVar(&l.ResourceLock, "leader-elect-resource-lock", l.ResourceLock, ""+
		"The type of resource object that is used for locking during "+
		"leader election. Supported options are `endpoints` (default) and `configmap`.")
}
