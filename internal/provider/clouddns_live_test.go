// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"slices"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/hornc-greedy/terraform-provider-netcup/internal/clouddns"
)

// This test only reads, so it also runs with a key whose domains are not on
// CloudDNS.
func TestAccCloudDNSDataSources(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckCloudDNS(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: `
data "netcup_clouddns_record_types" "all" {}

data "netcup_clouddns_domains" "all" {}
`,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttrSet("data.netcup_clouddns_record_types.all", "types.#"),
				resource.TestCheckResourceAttrSet("data.netcup_clouddns_domains.all", "domains.#"),
			),
		}},
	})
}

func testAccCloudDNSRecordConfig(fqdn string, ttl int) string {
	return fmt.Sprintf(`
data "netcup_clouddns_domain" "test" {
  fqdn = %q
}

resource "netcup_clouddns_record" "test" {
  domain_id = data.netcup_clouddns_domain.test.id
  name      = "tf-acc-test"
  type      = "A"
  rdata     = "192.0.2.1"
  ttl       = %d
}
`, fqdn, ttl)
}

// The TTL step checks the assumption that one changeset can delete and create the
// same record.
func TestAccCloudDNSRecord(t *testing.T) {
	fqdn := os.Getenv("NETCUP_TEST_CLOUDDNS_DOMAIN")
	if fqdn == "" {
		t.Skip("NETCUP_TEST_CLOUDDNS_DOMAIN not set")
	}
	const name = "netcup_clouddns_record.test"
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckCloudDNS(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccCloudDNSRecordConfig(fqdn, 300),
				Check:  resource.TestCheckResourceAttr(name, "ttl", "300"),
			},
			{
				Config: testAccCloudDNSRecordConfig(fqdn, 600),
				Check:  resource.TestCheckResourceAttr(name, "ttl", "600"),
			},
			{ResourceName: name, ImportState: true, ImportStateVerify: true},
		},
	})
}

func TestAccCloudDNSAcmeChallenge(t *testing.T) {
	fqdn := os.Getenv("NETCUP_TEST_CLOUDDNS_DOMAIN")
	if fqdn == "" {
		t.Skip("NETCUP_TEST_CLOUDDNS_DOMAIN not set")
	}
	const name = "netcup_clouddns_acme_challenge.test"
	value := acctest.RandStringFromCharSet(43, acctest.CharSetAlphaNum)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckCloudDNS(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(s *terraform.State) error {
			domainID, err := strconv.ParseInt(s.RootModule().Resources["data.netcup_clouddns_domain.test"].Primary.Attributes["id"], 10, 64)
			if err != nil {
				return err
			}
			client := clouddns.New(cmp.Or(os.Getenv("NETCUP_CLOUDDNS_BASE_URL"), clouddns.DefaultBaseURL), os.Getenv("NETCUP_CLOUDDNS_API_KEY"))
			challenges, err := client.Challenges(context.Background(), domainID)
			if err != nil {
				return err
			}
			if slices.ContainsFunc(challenges, func(c clouddns.Challenge) bool { return c.Value == value }) {
				return fmt.Errorf("challenge %s survived the destroy", value)
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
data "netcup_clouddns_domain" "test" {
  fqdn = %q
}

resource "netcup_clouddns_acme_challenge" "test" {
  domain_id = data.netcup_clouddns_domain.test.id
  scope     = "tf-acc"
  value     = %q
}

data "netcup_clouddns_acme_challenges" "test" {
  domain_id  = data.netcup_clouddns_domain.test.id
  depends_on = [netcup_clouddns_acme_challenge.test]
}
`, fqdn, value),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(name, "fqdn"),
					resource.TestCheckTypeSetElemNestedAttrs("data.netcup_clouddns_acme_challenges.test", "challenges.*", map[string]string{"value": value}),
				),
			},
			{ResourceName: name, ImportState: true, ImportStateVerify: true},
		},
	})
}
