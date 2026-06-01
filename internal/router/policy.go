package router

type Policy interface {
	Select(plan RoutePlan, health map[string]Health) ([]Target, string)
}

type PriorityPolicy struct{}

func (PriorityPolicy) Select(plan RoutePlan, health map[string]Health) ([]Target, string) {
	if len(plan.Targets) == 0 {
		return nil, "no_targets"
	}
	return plan.Targets[:1], "priority_first"
}

type FallbackPolicy struct{}

func (FallbackPolicy) Select(plan RoutePlan, health map[string]Health) ([]Target, string) {
	return append([]Target(nil), plan.Targets...), "fallback_order"
}

func PolicyFor(strategy string) Policy {
	if strategy == "fallback" {
		return FallbackPolicy{}
	}
	return PriorityPolicy{}
}
