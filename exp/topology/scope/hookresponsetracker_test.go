/*
Copyright 2021 The Kubernetes Authors.

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

package scope

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
)

func TestHookResponseTracker_AggregateRetryAfter(t *testing.T) {
	nonBlockingBeforeClusterCreateResponse := &runtimehooksv1.BeforeClusterCreateResponse{
		CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
			RetryAfterSeconds: int32(0),
			CommonResponse: runtimehooksv1.CommonResponse{
				Status: runtimehooksv1.ResponseStatusSuccess,
			},
		},
	}
	blockingBeforeClusterCreateResponse := &runtimehooksv1.BeforeClusterCreateResponse{
		CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
			RetryAfterSeconds: int32(10),
			CommonResponse: runtimehooksv1.CommonResponse{
				Status: runtimehooksv1.ResponseStatusSuccess,
			},
		},
	}

	nonBlockingBeforeClusterUpgradeResponse := &runtimehooksv1.BeforeClusterUpgradeResponse{
		CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
			RetryAfterSeconds: int32(0),
			CommonResponse: runtimehooksv1.CommonResponse{
				Status: runtimehooksv1.ResponseStatusSuccess,
			},
		},
	}
	blockingBeforeClusterUpgradeResponse := &runtimehooksv1.BeforeClusterUpgradeResponse{
		CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
			RetryAfterSeconds: int32(5),
			CommonResponse: runtimehooksv1.CommonResponse{
				Status: runtimehooksv1.ResponseStatusSuccess,
			},
		},
	}

	t.Run("AggregateRetryAfter should return non zero value if there are any blocking hook responses", func(t *testing.T) {
		g := NewWithT(t)

		hrt := NewHookResponseTracker()
		hrt.Add(runtimehooksv1.BeforeClusterCreate, blockingBeforeClusterCreateResponse)
		hrt.Add(runtimehooksv1.BeforeClusterUpgrade, nonBlockingBeforeClusterUpgradeResponse)

		g.Expect(hrt.AggregateRetryAfter()).To(Equal(time.Duration(10) * time.Second))
	})
	t.Run("AggregateRetryAfter should return zero value if there are no blocking hook responses", func(t *testing.T) {
		g := NewWithT(t)

		hrt := NewHookResponseTracker()
		hrt.Add(runtimehooksv1.BeforeClusterCreate, nonBlockingBeforeClusterCreateResponse)
		hrt.Add(runtimehooksv1.BeforeClusterUpgrade, nonBlockingBeforeClusterUpgradeResponse)

		g.Expect(hrt.AggregateRetryAfter()).To(Equal(time.Duration(0)))
	})
	t.Run("AggregateRetryAfter should return the lowest non-zero value if there are multiple blocking hook responses", func(t *testing.T) {
		g := NewWithT(t)

		hrt := NewHookResponseTracker()
		hrt.Add(runtimehooksv1.BeforeClusterCreate, blockingBeforeClusterCreateResponse)
		hrt.Add(runtimehooksv1.BeforeClusterUpgrade, blockingBeforeClusterUpgradeResponse)

		g.Expect(hrt.AggregateRetryAfter()).To(Equal(time.Duration(5) * time.Second))
	})
}

func TestHookResponseTracker_AggregateMessage(t *testing.T) {
	nonBlockingBeforeClusterCreateResponse := &runtimehooksv1.BeforeClusterCreateResponse{
		CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
			RetryAfterSeconds: int32(0),
			CommonResponse: runtimehooksv1.CommonResponse{
				Status: runtimehooksv1.ResponseStatusSuccess,
			},
		},
	}
	blockingBeforeClusterCreateResponse := &runtimehooksv1.BeforeClusterCreateResponse{
		CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
			RetryAfterSeconds: int32(10),
			CommonResponse: runtimehooksv1.CommonResponse{
				Status: runtimehooksv1.ResponseStatusSuccess,
			},
		},
	}

	nonBlockingBeforeClusterUpgradeResponse := &runtimehooksv1.BeforeClusterUpgradeResponse{
		CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
			RetryAfterSeconds: int32(0),
			CommonResponse: runtimehooksv1.CommonResponse{
				Status: runtimehooksv1.ResponseStatusSuccess,
			},
		},
	}
	blockingBeforeClusterUpgradeResponse := &runtimehooksv1.BeforeClusterUpgradeResponse{
		CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
			RetryAfterSeconds: int32(5),
			CommonResponse: runtimehooksv1.CommonResponse{
				Status: runtimehooksv1.ResponseStatusSuccess,
			},
		},
	}

	t.Run("AggregateMessage should return a message with the names of all the blocking hooks", func(t *testing.T) {
		g := NewWithT(t)

		hrt := NewHookResponseTracker()
		hrt.Add(runtimehooksv1.BeforeClusterCreate, blockingBeforeClusterCreateResponse)
		hrt.Add(runtimehooksv1.BeforeClusterUpgrade, blockingBeforeClusterUpgradeResponse)

		g.Expect(hrt.AggregateMessage()).To(ContainSubstring(runtimecatalog.HookName(runtimehooksv1.BeforeClusterCreate)))
		g.Expect(hrt.AggregateMessage()).To(ContainSubstring(runtimecatalog.HookName(runtimehooksv1.BeforeClusterUpgrade)))
	})
	t.Run("AggregateMessage should return empty string if there are no blocking hook responses", func(t *testing.T) {
		g := NewWithT(t)

		hrt := NewHookResponseTracker()
		hrt.Add(runtimehooksv1.BeforeClusterCreate, nonBlockingBeforeClusterCreateResponse)
		hrt.Add(runtimehooksv1.BeforeClusterUpgrade, nonBlockingBeforeClusterUpgradeResponse)

		g.Expect(hrt.AggregateMessage()).To(Equal(""))
	})
}

func TestHookResponseTracker_IsBlocking(t *testing.T) {
	nonBlockingBeforeClusterCreateResponse := &runtimehooksv1.BeforeClusterCreateResponse{
		CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
			RetryAfterSeconds: int32(0),
			CommonResponse: runtimehooksv1.CommonResponse{
				Status: runtimehooksv1.ResponseStatusSuccess,
			},
		},
	}
	blockingBeforeClusterCreateResponse := &runtimehooksv1.BeforeClusterCreateResponse{
		CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
			RetryAfterSeconds: int32(10),
			CommonResponse: runtimehooksv1.CommonResponse{
				Status: runtimehooksv1.ResponseStatusSuccess,
			},
		},
	}

	afterClusterUpgradeResponse := &runtimehooksv1.AfterClusterUpgradeResponse{
		TypeMeta:       metav1.TypeMeta{},
		CommonResponse: runtimehooksv1.CommonResponse{},
	}

	t.Run("should return true if the tracker received a blocking response for the hook", func(t *testing.T) {
		g := NewWithT(t)

		hrt := NewHookResponseTracker()
		hrt.Add(runtimehooksv1.BeforeClusterCreate, blockingBeforeClusterCreateResponse)

		g.Expect(hrt.IsBlocking(runtimehooksv1.BeforeClusterCreate)).To(BeTrue())
	})

	t.Run("should return false if the tracker received a non blocking response for the hook", func(t *testing.T) {
		g := NewWithT(t)

		hrt := NewHookResponseTracker()
		hrt.Add(runtimehooksv1.BeforeClusterCreate, nonBlockingBeforeClusterCreateResponse)

		g.Expect(hrt.IsBlocking(runtimehooksv1.BeforeClusterCreate)).To(BeFalse())
	})

	t.Run("should return false if the tracker did not receive a response for the hook", func(t *testing.T) {
		g := NewWithT(t)
		hrt := NewHookResponseTracker()
		g.Expect(hrt.IsBlocking(runtimehooksv1.BeforeClusterCreate)).To(BeFalse())
	})

	t.Run("should return false if the hook is non-blocking", func(t *testing.T) {
		g := NewWithT(t)
		hrt := NewHookResponseTracker()
		// AfterClusterUpgradeHook is non-blocking.
		hrt.Add(runtimehooksv1.AfterClusterUpgrade, afterClusterUpgradeResponse)
		g.Expect(hrt.IsBlocking(runtimehooksv1.AfterClusterUpgrade)).To(BeFalse())
	})
}
