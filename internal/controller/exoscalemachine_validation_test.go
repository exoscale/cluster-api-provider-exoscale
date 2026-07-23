package controller

import (
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrav1alpha1 "github.com/exoscale/cluster-api-provider-exoscale/api/v1alpha1"
)

const validationTestProviderID = "exoscale://8a991b98-e12e-4ef5-98ea-b44dd829be2f"

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
		providerID := validationTestProviderID
		machine.Spec.ProviderID = &providerID
		Expect(k8sClient.Update(ctx, machine)).To(Succeed())

		providerID = "exoscale://dfb08c5f-e85c-46c4-a037-8031d70532eb"
		machine.Spec.ProviderID = &providerID
		Expect(k8sClient.Update(ctx, machine)).To(Succeed())
	})

	It("accepts legacy fields and an equal canonical migration", func() {
		rootVolumeSize := int64(20)
		machine := &infrav1alpha1.ExoscaleMachine{
			ObjectMeta: metav1.ObjectMeta{GenerateName: "legacy-machine-", Namespace: "default"},
			Spec: infrav1alpha1.ExoscaleMachineSpec{
				TemplateID:       uuid.NewString(),
				InstanceType:     "small",
				RootVolumeSizeGB: &rootVolumeSize,
			},
		}
		Expect(k8sClient.Create(ctx, machine)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, machine))).To(Succeed()) })

		providerID := validationTestProviderID
		machine.Spec.ProviderID = &providerID
		Expect(k8sClient.Update(ctx, machine)).To(Succeed())

		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(machine), machine)).To(Succeed())
		machine.Spec.Template = machine.Spec.TemplateID
		machine.Spec.TemplateID = ""
		machine.Spec.RootVolumeSizeGiB = machine.Spec.RootVolumeSizeGB
		machine.Spec.RootVolumeSizeGB = nil
		Expect(k8sClient.Update(ctx, machine)).To(Succeed())
	})

	It("rejects ambiguous or missing compatibility fields", func() {
		base := func() *infrav1alpha1.ExoscaleMachine {
			return &infrav1alpha1.ExoscaleMachine{
				ObjectMeta: metav1.ObjectMeta{GenerateName: "invalid-machine-", Namespace: "default"},
				Spec:       infrav1alpha1.ExoscaleMachineSpec{InstanceType: "small"},
			}
		}

		machine := base()
		Expect(k8sClient.Create(ctx, machine)).To(MatchError(ContainSubstring("exactly one of template or templateID must be set")))

		machine = base()
		machine.Spec.Template = "ubuntu"
		machine.Spec.TemplateID = uuid.NewString()
		Expect(k8sClient.Create(ctx, machine)).To(MatchError(ContainSubstring("exactly one of template or templateID must be set")))

		rootVolumeSize := int64(20)
		machine = base()
		machine.Spec.Template = "ubuntu"
		machine.Spec.RootVolumeSizeGiB = &rootVolumeSize
		machine.Spec.RootVolumeSizeGB = &rootVolumeSize
		Expect(k8sClient.Create(ctx, machine)).To(MatchError(ContainSubstring("rootVolumeSizeGiB and rootVolumeSizeGB are mutually exclusive")))
	})
})

var _ = Describe("ExoscaleMachineTemplate CRD validation", func() {
	It("rejects providerID in templates", func() {
		providerID := validationTestProviderID
		template := &infrav1alpha1.ExoscaleMachineTemplate{
			ObjectMeta: metav1.ObjectMeta{GenerateName: "invalid-machine-template-", Namespace: "default"},
			Spec: infrav1alpha1.ExoscaleMachineTemplateSpec{
				Template: infrav1alpha1.ExoscaleMachineTemplateResource{
					Spec: infrav1alpha1.ExoscaleMachineSpec{
						Template:     "ubuntu",
						InstanceType: "small",
						ProviderID:   &providerID,
					},
				},
			},
		}

		Expect(k8sClient.Create(ctx, template)).To(MatchError(ContainSubstring("providerID must not be set")))
	})

	It("leaves template spec immutability to the topology-aware webhook", func() {
		template := &infrav1alpha1.ExoscaleMachineTemplate{
			ObjectMeta: metav1.ObjectMeta{GenerateName: "mutable-machine-template-", Namespace: "default"},
			Spec: infrav1alpha1.ExoscaleMachineTemplateSpec{
				Template: infrav1alpha1.ExoscaleMachineTemplateResource{
					Spec: infrav1alpha1.ExoscaleMachineSpec{
						Template:     "ubuntu",
						InstanceType: "small",
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, template)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, template))).To(Succeed()) })

		template.Spec.Template.Spec.InstanceType = "medium"
		Expect(k8sClient.Update(ctx, template)).To(Succeed())
	})
})
