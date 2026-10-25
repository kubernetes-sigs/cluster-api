/*
Copyright 2024 The Kubernetes Authors.

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

// Package clustercache implements the ClusterCache.
//
// The ClusterCache is used to create and cache clients, REST configs, etc. for workload clusters.
//
// # Setup
//
// ClusterCache can be set up via the SetupWithManager function. It will add the ClusterCache as a
// reconciler to the Manager and then return a ClusterCache object.
//
// # Usage
//
// Typically the ClusterCache is passed to controllers and then it can be used by e.g. simply calling
// GetClient to retrieve a client for the given Cluster (see the ClusterCache interface for more details).
// If GetClient does return an error, controllers should not requeue, requeue after or return an error and
// go in exponential backoff. Instead, they should register a source (via GetClusterSource) to receive reconnect
// events from the ClusterCache.
//
// # Implementation details
//
// The ClusterCache internally runs a reconciler that:
// - tries to create a connection to a workload cluster
//   - if it fails, it will retry after roughly the ConnectionCreationRetryInterval
//
// - if the connection is established it will run continuous health checking every HealthProbe.Interval.
//   - if the health checking fails more than HealthProbe.FailureThreshold times consecutively or if an
//     unauthorized error occurs, the connection will be disconnected (a subsequent Reconcile will try
//     to connect again)
//
// - if other reconcilers (e.g. the Machine controller) got a source to watch for events, they will get notified if:
//   - a connect or disconnect happened
//   - the health probe didn't succeed for a certain amount of time (if the WatchForProbeFailure option was used)
//
// # Implementation considerations
//
// Some considerations about the trade-offs that have been made:
//   - There is intentionally only one Reconciler that handles both the connect/disconnect and health checking.
//     This is a lot easier from a locking perspective than trying to coordinate between multiple reconcilers.
//   - We are never holding the write lock on the clusterAccessor for an extended period of time (e.g. during
//     connection creation, which can time out after a few seconds). This is important so that other reconcilers
//     that are calling GetClient etc. are never blocked.
package clustercache
