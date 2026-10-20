// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
)

func int64RequiresReplace() planmodifier.Int64 {
	return int64planmodifier.RequiresReplace()
}

func boolRequiresReplace() planmodifier.Bool {
	return boolplanmodifier.RequiresReplace()
}
