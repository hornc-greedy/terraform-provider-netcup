// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
)

// int64RequiresReplace is a small helper to keep schemas terse.
func int64RequiresReplace() planmodifier.Int64 {
	return int64planmodifier.RequiresReplace()
}

// boolRequiresReplace is a small helper to keep schemas terse.
func boolRequiresReplace() planmodifier.Bool {
	return boolplanmodifier.RequiresReplace()
}
