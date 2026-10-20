// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"slices"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hornc-greedy/terraform-provider-netcup/internal/scp"
)

// diagAppender lets helper methods collect diagnostics while returning plain
// values.
type diagAppender struct {
	diags *diag.Diagnostics
}

// stringList converts a types.List of strings to a Go slice. A null/unknown
// list yields nil.
func (d *diagAppender) stringList(ctx context.Context, l types.List) []string {
	if l.IsNull() || l.IsUnknown() {
		return nil
	}
	var out []string
	d.diags.Append(l.ElementsAs(ctx, &out, false)...)
	return out
}

// stringListValue builds a types.List from a Go slice, mapping an empty slice to
// a null list so it round-trips with Optional attributes that were never set.
func stringListValue(ctx context.Context, in []string, diags *diag.Diagnostics) types.List {
	if len(in) == 0 {
		return types.ListNull(types.StringType)
	}
	l, d := types.ListValueFrom(ctx, types.StringType, in)
	diags.Append(d...)
	return l
}

// sameAddresses reports whether a and b hold the same addresses. netcup defines
// them as unique items and specifies no order.
func sameAddresses(a, b []string) bool {
	return slices.Equal(slices.Sorted(slices.Values(a)), slices.Sorted(slices.Values(b)))
}

// addressList keeps the prior list while netcup returns the same addresses, so
// the configured order and an empty list stay stable.
func addressList(ctx context.Context, prior types.List, in []string, diags *diag.Diagnostics) types.List {
	if !prior.IsNull() {
		var priorAddresses []string
		diags.Append(prior.ElementsAs(ctx, &priorAddresses, false)...)
		if sameAddresses(priorAddresses, in) {
			return prior
		}
	}
	return stringListValue(ctx, in, diags)
}

// optString maps an empty API string to a null attribute value, so optional
// fields the user left unset do not show as "" drift.
func optString(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

// rulesToModel converts API rules into the resource's rule models, comparing
// each rule with the prior rule at the same position.
func rulesToModel(ctx context.Context, rules []scp.FirewallRule, prior []firewallRuleModel, diags *diag.Diagnostics) []firewallRuleModel {
	out := make([]firewallRuleModel, 0, len(rules))
	for i, r := range rules {
		var priorRule firewallRuleModel
		if i < len(prior) {
			priorRule = prior[i]
		}
		out = append(out, firewallRuleModel{
			Action:           types.StringValue(r.Action),
			Protocol:         types.StringValue(r.Protocol),
			Direction:        types.StringValue(r.Direction),
			Sources:          addressList(ctx, priorRule.Sources, r.Sources, diags),
			Destinations:     addressList(ctx, priorRule.Destinations, r.Destinations, diags),
			SourcePorts:      optString(r.SourcePorts),
			DestinationPorts: optString(r.DestinationPorts),
			Description:      optString(r.Description),
		})
	}
	return out
}
