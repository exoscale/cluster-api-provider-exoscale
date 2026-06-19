package service

import (
	"testing"

	"github.com/google/uuid"

	infrav1alpha1 "github.com/exoscale/cluster-api-provider-exoscale/api/v1alpha1"
	"github.com/exoscale/cluster-api-provider-exoscale/internal/domain"
	egoscale "github.com/exoscale/egoscale/v3"
)

func strPtr(s string) *string { return &s }

func TestRulesEqual(t *testing.T) {
	tcp := domain.SecurityGroupRuleProtocolTCP
	ingress := domain.SecurityGroupRuleFlowDirectionIngress

	a := domain.SecurityGroupRule{
		Description:   "ssh",
		FlowDirection: ingress,
		Protocol:      tcp,
		StartPort:     22,
		EndPort:       22,
		Network:       strPtr("0.0.0.0/0"),
	}

	t.Run("identical rules match", func(t *testing.T) {
		b := a
		b.ID = uuid.New()                       // ID is excluded from equality.
		b.Description = "different description" // Description is excluded.
		if !rulesEqual(a, b) {
			t.Fatal("expected equal")
		}
	})

	t.Run("different protocol", func(t *testing.T) {
		b := a
		b.Protocol = domain.SecurityGroupRuleProtocolUDP
		if rulesEqual(a, b) {
			t.Fatal("expected not equal")
		}
	})

	t.Run("different direction", func(t *testing.T) {
		b := a
		b.FlowDirection = domain.SecurityGroupRuleFlowDirectionEgress
		if rulesEqual(a, b) {
			t.Fatal("expected not equal")
		}
	})

	t.Run("different port", func(t *testing.T) {
		b := a
		b.EndPort = 2222
		if rulesEqual(a, b) {
			t.Fatal("expected not equal")
		}
	})

	t.Run("network nil vs set", func(t *testing.T) {
		b := a
		b.Network = nil
		if rulesEqual(a, b) {
			t.Fatal("expected not equal")
		}
	})

	t.Run("network both set differ", func(t *testing.T) {
		b := a
		other := "10.0.0.0/8"
		b.Network = &other
		if rulesEqual(a, b) {
			t.Fatal("expected not equal")
		}
	})

	t.Run("security group nil vs set", func(t *testing.T) {
		id := uuid.New()
		b := a
		b.Network = nil
		b.SecurityGroup = &id
		if rulesEqual(a, b) {
			t.Fatal("expected not equal")
		}
	})
}

func TestDiffRules(t *testing.T) {
	tcp := domain.SecurityGroupRuleProtocolTCP
	ingress := domain.SecurityGroupRuleFlowDirectionIngress

	mk := func(port int64, network string) domain.SecurityGroupRule {
		return domain.SecurityGroupRule{
			FlowDirection: ingress,
			Protocol:      tcp,
			StartPort:     port,
			EndPort:       port,
			Network:       &network,
		}
	}

	t.Run("no diff when sets match", func(t *testing.T) {
		desired := []domain.SecurityGroupRule{mk(22, "0.0.0.0/0"), mk(80, "10.0.0.0/8")}
		existing := []domain.SecurityGroupRule{mk(22, "0.0.0.0/0"), mk(80, "10.0.0.0/8")}
		add, del := diffRules(desired, existing)
		if len(add) != 0 || len(del) != 0 {
			t.Fatalf("expected empty diff, got add=%d del=%d", len(add), len(del))
		}
	})

	t.Run("add new rule", func(t *testing.T) {
		desired := []domain.SecurityGroupRule{mk(22, "0.0.0.0/0"), mk(443, "0.0.0.0/0")}
		existing := []domain.SecurityGroupRule{mk(22, "0.0.0.0/0")}
		add, del := diffRules(desired, existing)
		if len(add) != 1 || add[0].StartPort != 443 {
			t.Fatalf("expected one add for 443, got %+v", add)
		}
		if len(del) != 0 {
			t.Fatalf("expected no deletes, got %+v", del)
		}
	})

	t.Run("delete removed rule", func(t *testing.T) {
		desired := []domain.SecurityGroupRule{mk(22, "0.0.0.0/0")}
		existing := []domain.SecurityGroupRule{mk(22, "0.0.0.0/0"), mk(80, "10.0.0.0/8")}
		add, del := diffRules(desired, existing)
		if len(add) != 0 {
			t.Fatalf("expected no adds, got %+v", add)
		}
		if len(del) != 1 || del[0].StartPort != 80 {
			t.Fatalf("expected one delete for 80, got %+v", del)
		}
	})

	t.Run("ignore description and id when comparing", func(t *testing.T) {
		r1 := mk(22, "0.0.0.0/0")
		r1.Description = "before"
		r2 := mk(22, "0.0.0.0/0")
		r2.Description = "after"
		r2.ID = uuid.New()
		add, del := diffRules([]domain.SecurityGroupRule{r1}, []domain.SecurityGroupRule{r2})
		if len(add) != 0 || len(del) != 0 {
			t.Fatalf("expected empty diff, got add=%d del=%d", len(add), len(del))
		}
	})
}

func TestSpecRuleToDomain(t *testing.T) {
	itself := uuid.New()
	other := uuid.New()

	t.Run("plain cidr rule", func(t *testing.T) {
		rule := infrav1alpha1.SecurityGroupRule{
			FlowDirection: egoscale.SecurityGroupRuleFlowDirectionIngress,
			Protocol:      egoscale.SecurityGroupRuleProtocolTCP,
			StartPort:     22,
			EndPort:       22,
			Network:       strPtr("0.0.0.0/0"),
			Description:   "ssh",
		}
		got, err := specRuleToDomain(rule, itself)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.SecurityGroup != nil {
			t.Fatalf("expected no SG ref, got %v", *got.SecurityGroup)
		}
		if got.Network == nil || *got.Network != "0.0.0.0/0" {
			t.Fatalf("unexpected network: %+v", got.Network)
		}
	})

	t.Run("itself sentinel resolves to owning sg id", func(t *testing.T) {
		rule := infrav1alpha1.SecurityGroupRule{
			FlowDirection: egoscale.SecurityGroupRuleFlowDirectionIngress,
			Protocol:      egoscale.SecurityGroupRuleProtocolTCP,
			StartPort:     10250,
			EndPort:       10250,
			SecurityGroup: strPtr("itself"),
			Description:   "kubelet",
		}
		got, err := specRuleToDomain(rule, itself)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.SecurityGroup == nil || *got.SecurityGroup != itself {
			t.Fatalf("expected itself id %v, got %v", itself, got.SecurityGroup)
		}
	})

	t.Run("explicit uuid is parsed", func(t *testing.T) {
		rule := infrav1alpha1.SecurityGroupRule{
			FlowDirection: egoscale.SecurityGroupRuleFlowDirectionIngress,
			Protocol:      egoscale.SecurityGroupRuleProtocolTCP,
			StartPort:     10250,
			EndPort:       10250,
			SecurityGroup: strPtr(other.String()),
			Description:   "kubelet",
		}
		got, err := specRuleToDomain(rule, itself)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.SecurityGroup == nil || *got.SecurityGroup != other {
			t.Fatalf("expected %v, got %v", other, got.SecurityGroup)
		}
	})

	t.Run("invalid uuid rejected", func(t *testing.T) {
		rule := infrav1alpha1.SecurityGroupRule{
			FlowDirection: egoscale.SecurityGroupRuleFlowDirectionIngress,
			Protocol:      egoscale.SecurityGroupRuleProtocolTCP,
			StartPort:     10250,
			EndPort:       10250,
			SecurityGroup: strPtr("not-a-uuid"),
			Description:   "kubelet",
		}
		if _, err := specRuleToDomain(rule, itself); err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}
