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

/*
Package etcd provides a connection to an etcd member.

There is a backoff adapter that adds retries to the standard etcd v3 go client.

An example usage of this package:

	// create a dialer function
	func myDialer(ctx context.Context, addr string) (net.Conn, error) {
		...
	}

	// An example helper in client code
	func getBackoffClient(endpoint string, dialer GRPCDial, cfg *tls.Config, timeout time.Duration) (*etcd.Client, error) {
		etcdClient, _ := etcd.NewEtcdClient(endpoint, dialer, cfg)
		adapter, _ := etcd.NewEtcdBackoffAdapter(etcdClient, WithTimeout(timeout))
		return etcd.NewClientWithEtcd(adapter)
	}

	// usage
	func talkToEtcd() {
		client, _, := getBackoffClient("localhost", myDialer, cfg, 1*time.Second)
		client.Status(context.TODO())
	}

The adapter is a helper and not necessary. Use the default clientv3 if you do not need retries or stub out etcd for unit
tests.
*/
package etcd
