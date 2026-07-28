package controller

import (
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrav1alpha1 "github.com/exoscale/cluster-api-provider-exoscale/api/v1alpha1"
)

var _ = Describe("ExoscaleMachine validation", func() {
	It("keeps creation fields immutable while allowing providerID updates", func() {
		machine := &infrav1alpha1.ExoscaleMachine{
			ObjectMeta: metav1.ObjectMeta{GenerateName: "immutable-machine-", Namespace: "default"},
			Spec: infrav1alpha1.ExoscaleMachineSpec{
				Template:     "ubuntu",
				InstanceType: "small",
			},
		}
		Expect(k8sClient.Create(ctx, machine)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, machine))).To(Succeed()) })

		machine.Spec.Template = "debian"
		Expect(k8sClient.Update(ctx, machine)).To(MatchError(ContainSubstring("instance creation fields are immutable")))

		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(machine), machine)).To(Succeed())
		providerID := "exoscale://8a991b98-e12e-4ef5-98ea-b44dd829be2f"
		machine.Spec.ProviderID = &providerID
		Expect(k8sClient.Update(ctx, machine)).To(Succeed())

		providerID = "exoscale://dfb08c5f-e85c-46c4-a037-8031d70532eb"
		machine.Spec.ProviderID = &providerID
		Expect(k8sClient.Update(ctx, machine)).To(Succeed())
	})

	It("accepts a template UUID and rootVolumeSizeGiB", func() {
		rootVolumeSize := int64(20)
		machine := &infrav1alpha1.ExoscaleMachine{
			ObjectMeta: metav1.ObjectMeta{GenerateName: "legacy-machine-", Namespace: "default"},
			Spec: infrav1alpha1.ExoscaleMachineSpec{
				Template:          uuid.NewString(),
				InstanceType:      "small",
				RootVolumeSizeGiB: &rootVolumeSize,
			},
		}
		Expect(k8sClient.Create(ctx, machine)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, machine))).To(Succeed()) })

		providerID := "exoscale://8a991b98-e12e-4ef5-98ea-b44dd829be2f"
		machine.Spec.ProviderID = &providerID
		Expect(k8sClient.Update(ctx, machine)).To(Succeed())

		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(machine), machine)).To(Succeed())
		Expect(k8sClient.Update(ctx, machine)).To(Succeed())
	})

	It("requires a template", func() {
		machine := &infrav1alpha1.ExoscaleMachine{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "invalid-machine-",
				Namespace:    "default",
			},
			Spec: infrav1alpha1.ExoscaleMachineSpec{
				InstanceType: "small",
			},
		}

		Expect(k8sClient.Create(ctx, machine)).
			To(MatchError(ContainSubstring("spec.template")))
	})
})
