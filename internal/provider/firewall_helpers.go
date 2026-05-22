// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

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

// optString maps an empty API string to a null attribute value, so optional
// fields the user left unset do not show as "" drift.
func optString(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

// rulesToModel converts API rules into the resource's rule models.
func rulesToModel(ctx context.Context, rules []scp.FirewallRule, diags *diag.Diagnostics) []firewallRuleModel {
	out := make([]firewallRuleModel, 0, len(rules))
	for _, r := range rules {
		out = append(out, firewallRuleModel{
			Action:           types.StringValue(r.Action),
			Protocol:         types.StringValue(r.Protocol),
			Direction:        types.StringValue(r.Direction),
			Sources:          stringListValue(ctx, r.Sources, diags),
			Destinations:     stringListValue(ctx, r.Destinations, diags),
			SourcePorts:      optString(r.SourcePorts),
			DestinationPorts: optString(r.DestinationPorts),
			Description:      optString(r.Description),
		})
	}
	return out
}
