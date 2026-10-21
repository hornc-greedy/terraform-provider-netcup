// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hornc-greedy/terraform-provider-netcup/internal/scp"
)

func testStringList(t *testing.T, values ...string) types.List {
	t.Helper()
	list, diags := types.ListValueFrom(context.Background(), types.StringType, values)
	if diags.HasError() {
		t.Fatalf("list: %v", diags)
	}
	return list
}

func TestAddressListKeepsPriorOrder(t *testing.T) {
	var diags diag.Diagnostics
	prior := testStringList(t, "b", "a")
	got := addressList(context.Background(), prior, []string{"a", "b"}, &diags)
	if !got.Equal(prior) {
		t.Fatalf("got %v, want %v", got, prior)
	}
}

func TestAddressListTakesChangedAddresses(t *testing.T) {
	var diags diag.Diagnostics
	got := addressList(context.Background(), testStringList(t, "a", "b"), []string{"a", "c"}, &diags)
	if want := testStringList(t, "a", "c"); !got.Equal(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestAddressListKeepsEmptyList(t *testing.T) {
	var diags diag.Diagnostics
	prior := types.ListValueMust(types.StringType, []attr.Value{})
	got := addressList(context.Background(), prior, nil, &diags)
	if got.IsNull() || !got.Equal(prior) {
		t.Fatalf("got %v, want empty list", got)
	}
}

func TestAddressListNullStaysNull(t *testing.T) {
	var diags diag.Diagnostics
	got := addressList(context.Background(), types.ListNull(types.StringType), nil, &diags)
	if !got.IsNull() {
		t.Fatalf("got %v, want null", got)
	}
}

func TestKeptString(t *testing.T) {
	if got := keptString(types.StringValue(""), ""); got.IsNull() || got.ValueString() != "" {
		t.Fatalf("empty string not kept: %v", got)
	}
	if got := keptString(types.StringNull(), ""); !got.IsNull() {
		t.Fatalf("null not kept: %v", got)
	}
	if got := keptString(types.StringValue(""), "x"); got.ValueString() != "x" {
		t.Fatalf("api value ignored: %v", got)
	}
}

func TestRulesToModelWithoutPriorRule(t *testing.T) {
	var diags diag.Diagnostics
	got := rulesToModel(context.Background(), []scp.FirewallRule{{Sources: []string{"b", "a"}}}, nil, &diags)
	if want := testStringList(t, "b", "a"); !got[0].Sources.Equal(want) {
		t.Fatalf("got %v, want %v", got[0].Sources, want)
	}
	if !got[0].Destinations.IsNull() || !got[0].Description.IsNull() {
		t.Fatalf("unset values not null: %+v", got[0])
	}
}

func TestToAPISendsEmptyRules(t *testing.T) {
	var diags diag.Diagnostics
	m := firewallPolicyModel{Name: types.StringValue("p")}
	body, err := json.Marshal(m.toAPI(context.Background(), &diagAppender{diags: &diags}))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(body), `"rules":[]`) {
		t.Fatalf("rules not sent: %s", body)
	}
}
