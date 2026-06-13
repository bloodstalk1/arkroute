package router

// Policy picks one or more targets from a [RoutePlan]. Select returns
// the chosen targets in execution order plus a short string that
// identifies which branch the policy took (used for tracing).
type Policy interface {
	Select(plan RoutePlan, health map[string]Health) ([]Target, string)
}

// PriorityPolicy returns only the first candidate target. The runtime
// never falls back on a priority route; if the first target fails the
// request errors out.
type PriorityPolicy struct{}

func (PriorityPolicy) Select(plan RoutePlan, health map[string]Health) ([]Target, string) {
	if len(plan.Targets) == 0 {
		return nil, "no_targets"
	}
	return plan.Targets[:1], "priority_first"
}

// FallbackPolicy returns every target in plan order. The runtime tries
// each in turn and only stops once a target succeeds or the list is
// exhausted.
type FallbackPolicy struct{}

func (FallbackPolicy) Select(plan RoutePlan, health map[string]Health) ([]Target, string) {
	return append([]Target(nil), plan.Targets...), "fallback_order"
}

// PolicyFor returns the [Policy] that matches strategy. Unknown
// strategies fall back to PriorityPolicy so a malformed config still
// produces a working selection.
func PolicyFor(strategy string) Policy {
	if strategy == "fallback" {
		return FallbackPolicy{}
	}
	return PriorityPolicy{}
}
