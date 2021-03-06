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

package machinefilters_test

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha3"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/machinefilters"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func falseFilter(_ *clusterv1.Machine) bool {
	return false
}

func trueFilter(_ *clusterv1.Machine) bool {
	return true
}

func TestNot(t *testing.T) {
	t.Run("returns false given a machine filter that returns true", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		g.Expect(machinefilters.Not(trueFilter)(m)).To(BeFalse())
	})
	t.Run("returns true given a machine filter that returns false", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		g.Expect(machinefilters.Not(falseFilter)(m)).To(BeTrue())
	})
}

func TestAnd(t *testing.T) {
	t.Run("returns true if both given machine filters return true", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		g.Expect(machinefilters.And(trueFilter, trueFilter)(m)).To(BeTrue())
	})
	t.Run("returns false if either given machine filter returns false", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		g.Expect(machinefilters.And(trueFilter, falseFilter)(m)).To(BeFalse())
	})
}

func TestOr(t *testing.T) {
	t.Run("returns true if either given machine filters return true", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		g.Expect(machinefilters.Or(trueFilter, falseFilter)(m)).To(BeTrue())
	})
	t.Run("returns false if both given machine filter returns false", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		g.Expect(machinefilters.Or(falseFilter, falseFilter)(m)).To(BeFalse())
	})
}

func TestHasDeletionTimestamp(t *testing.T) {
	t.Run("machine with deletion timestamp returns true", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		now := metav1.Now()
		m.SetDeletionTimestamp(&now)
		g.Expect(machinefilters.HasDeletionTimestamp(m)).To(BeTrue())
	})
	t.Run("machine with nil deletion timestamp returns false", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		g.Expect(machinefilters.HasDeletionTimestamp(m)).To(BeFalse())
	})
	t.Run("machine with zero deletion timestamp returns false", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		zero := metav1.NewTime(time.Time{})
		m.SetDeletionTimestamp(&zero)
		g.Expect(machinefilters.HasDeletionTimestamp(m)).To(BeFalse())
	})
}

func TestShouldRolloutAfter(t *testing.T) {
	t.Run("if upgradeAfter is nil, return false", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		m.SetCreationTimestamp(metav1.NewTime(time.Now().Add(-2 * time.Hour)))
		g.Expect(machinefilters.ShouldRolloutAfter(nil)(m)).To(BeFalse())
	})
	t.Run("if upgradeAfter is in the future, return false", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		m.SetCreationTimestamp(metav1.NewTime(time.Now().Add(-2 * time.Hour)))
		upgradeAfter := metav1.NewTime(time.Now().Add(+1 * time.Hour)) // upgrade after in the future
		g.Expect(machinefilters.ShouldRolloutAfter(&upgradeAfter)(m)).To(BeFalse())
	})
	t.Run("if upgradeAfter is in the past and the machine was created before upgradeAfter, return true", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		m.SetCreationTimestamp(metav1.NewTime(time.Now().Add(-2 * time.Hour))) // machine was created before upgradeAfter
		upgradeAfter := metav1.NewTime(time.Now().Add(-1 * time.Hour))         // upgrade after in the past
		g.Expect(machinefilters.ShouldRolloutAfter(&upgradeAfter)(m)).To(BeTrue())
	})
	t.Run("if upgradeAfter is in the past and the machine was created after upgradeAfter, return false", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		m.SetCreationTimestamp(metav1.NewTime(time.Now().Add(+1 * time.Hour))) // machine was created after upgradeAfter
		upgradeAfter := metav1.NewTime(time.Now().Add(-1 * time.Hour))         // upgrade after in the past
		g.Expect(machinefilters.ShouldRolloutAfter(&upgradeAfter)(m)).To(BeFalse())
	})
}

func TestHashAnnotationKey(t *testing.T) {
	t.Run("machine with specified annotation returns true", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		m.SetAnnotations(map[string]string{"test": ""})
		g.Expect(machinefilters.HasAnnotationKey("test")(m)).To(BeTrue())
	})
	t.Run("machine with specified annotation with non-empty value returns true", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		m.SetAnnotations(map[string]string{"test": "blue"})
		g.Expect(machinefilters.HasAnnotationKey("test")(m)).To(BeTrue())
	})
	t.Run("machine without specified annotation returns false", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		g.Expect(machinefilters.HasAnnotationKey("foo")(m)).To(BeFalse())
	})
}

func TestInFailureDomain(t *testing.T) {
	t.Run("nil machine returns false", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(machinefilters.InFailureDomains(pointer.StringPtr("test"))(nil)).To(BeFalse())
	})
	t.Run("machine with given failure domain returns true", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{Spec: clusterv1.MachineSpec{FailureDomain: pointer.StringPtr("test")}}
		g.Expect(machinefilters.InFailureDomains(pointer.StringPtr("test"))(m)).To(BeTrue())
	})
	t.Run("machine with a different failure domain returns false", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{Spec: clusterv1.MachineSpec{FailureDomain: pointer.StringPtr("notTest")}}
		g.Expect(machinefilters.InFailureDomains(
			pointer.StringPtr("test"),
			pointer.StringPtr("test2"),
			pointer.StringPtr("test3"),
			nil,
			pointer.StringPtr("foo"))(m)).To(BeFalse())
	})
	t.Run("machine without failure domain returns false", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		g.Expect(machinefilters.InFailureDomains(pointer.StringPtr("test"))(m)).To(BeFalse())
	})
	t.Run("machine without failure domain returns true, when nil used for failure domain", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		g.Expect(machinefilters.InFailureDomains(nil)(m)).To(BeTrue())
	})
	t.Run("machine with failure domain returns true, when one of multiple failure domains match", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{Spec: clusterv1.MachineSpec{FailureDomain: pointer.StringPtr("test")}}
		g.Expect(machinefilters.InFailureDomains(pointer.StringPtr("foo"), pointer.StringPtr("test"))(m)).To(BeTrue())
	})
}

func TestMatchesKubernetesVersion(t *testing.T) {
	t.Run("nil machine returns false", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(machinefilters.MatchesKubernetesVersion("some_ver")(nil)).To(BeFalse())
	})

	t.Run("nil machine.Spec.Version returns false", func(t *testing.T) {
		g := NewWithT(t)
		machine := &clusterv1.Machine{
			Spec: clusterv1.MachineSpec{
				Version: nil,
			},
		}
		g.Expect(machinefilters.MatchesKubernetesVersion("some_ver")(machine)).To(BeFalse())
	})

	t.Run("machine.Spec.Version returns true if matches", func(t *testing.T) {
		g := NewWithT(t)
		kversion := "some_ver"
		machine := &clusterv1.Machine{
			Spec: clusterv1.MachineSpec{
				Version: &kversion,
			},
		}
		g.Expect(machinefilters.MatchesKubernetesVersion("some_ver")(machine)).To(BeTrue())
	})

	t.Run("machine.Spec.Version returns false if does not match", func(t *testing.T) {
		g := NewWithT(t)
		kversion := "some_ver_2"
		machine := &clusterv1.Machine{
			Spec: clusterv1.MachineSpec{
				Version: &kversion,
			},
		}
		g.Expect(machinefilters.MatchesKubernetesVersion("some_ver")(machine)).To(BeFalse())
	})
}

func TestMatchesTemplateClonedFrom(t *testing.T) {
	t.Run("nil machine returns false", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(
			machinefilters.MatchesTemplateClonedFrom(context.TODO(), nil, nil)(nil),
		).To(BeFalse())
	})

	t.Run("returns true if machine not found", func(t *testing.T) {
		g := NewWithT(t)
		client := fake.NewFakeClient()
		kcp := &controlplanev1.KubeadmControlPlane{}
		machine := &clusterv1.Machine{
			Spec: clusterv1.MachineSpec{
				InfrastructureRef: corev1.ObjectReference{
					Kind:       "KubeadmConfig",
					Namespace:  "default",
					Name:       "test",
					APIVersion: bootstrapv1.GroupVersion.String(),
				},
			},
		}
		g.Expect(
			machinefilters.MatchesTemplateClonedFrom(context.TODO(), client, kcp)(machine),
		).To(BeTrue())
	})
}

func TestMatchesTemplateClonedFrom_WithClonedFromAnnotations(t *testing.T) {
	kcp := &controlplanev1.KubeadmControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
		},
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			InfrastructureTemplate: corev1.ObjectReference{
				Kind:       "GenericMachineTemplate",
				Namespace:  "default",
				Name:       "infra-foo",
				APIVersion: "generic.io/v1",
			},
		},
	}
	machine := &clusterv1.Machine{
		Spec: clusterv1.MachineSpec{
			InfrastructureRef: corev1.ObjectReference{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha3",
				Kind:       "InfrastructureMachine",
				Name:       "infra-config1",
				Namespace:  "default",
			},
		},
	}
	tests := []struct {
		name        string
		annotations map[string]interface{}
		expectMatch bool
	}{
		{
			name:        "returns true if annotations don't exist",
			annotations: map[string]interface{}{},
			expectMatch: true,
		},
		{
			name: "returns false if annotations don't match anything",
			annotations: map[string]interface{}{
				clusterv1.TemplateClonedFromNameAnnotation:      "barfoo1",
				clusterv1.TemplateClonedFromGroupKindAnnotation: "barfoo2",
			},
			expectMatch: false,
		},
		{
			name: "returns false if TemplateClonedFromNameAnnotation matches but TemplateClonedFromGroupKindAnnotation doesn't",
			annotations: map[string]interface{}{
				clusterv1.TemplateClonedFromNameAnnotation:      "infra-foo",
				clusterv1.TemplateClonedFromGroupKindAnnotation: "barfoo2",
			},
			expectMatch: false,
		},
		{
			name: "returns true if both annotations match",
			annotations: map[string]interface{}{
				clusterv1.TemplateClonedFromNameAnnotation:      "infra-foo",
				clusterv1.TemplateClonedFromGroupKindAnnotation: "GenericMachineTemplate.generic.io",
			},
			expectMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			infraConfig := unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "InfrastructureMachine",
					"apiVersion": "infrastructure.cluster.x-k8s.io/v1alpha3",
					"metadata": map[string]interface{}{
						"name":        "infra-config1",
						"namespace":   "default",
						"annotations": tt.annotations,
					},
				},
			}
			scheme := runtime.NewScheme()
			client := fake.NewFakeClientWithScheme(scheme, &infraConfig)

			g.Expect(
				machinefilters.MatchesTemplateClonedFrom(context.TODO(), client, kcp)(machine),
			).To(Equal(tt.expectMatch))
		})
	}
}
