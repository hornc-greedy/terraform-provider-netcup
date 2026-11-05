// SPDX-License-Identifier: MPL-2.0

package mockapi

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"
)

// TestEndpointTable fails when spec/ holds an operation the table does not list,
// or when the table names objects for an operation the mock does not implement.
func TestEndpointTable(t *testing.T) {
	listed := map[string]Endpoint{}
	for _, e := range Endpoints {
		key := e.Method + " " + e.Path
		if _, dup := listed[key]; dup {
			t.Errorf("%s is listed twice", key)
		}
		listed[key] = e
	}

	var inSpec []string
	for _, name := range []string{"spec/scp.json", "spec/netcup.json"} {
		doc, err := loadSpec(name)
		if err != nil {
			t.Fatalf("load %s: %v", name, err)
		}
		for path, item := range doc.Paths.Map() {
			for method := range item.Operations() {
				inSpec = append(inSpec, method+" "+path)
			}
		}
	}
	for _, key := range inSpec {
		if _, ok := listed[key]; !ok {
			t.Errorf("%s is in the specification but not in Endpoints", key)
		}
	}
	for key := range listed {
		if !slices.Contains(inSpec, key) {
			t.Errorf("%s is in Endpoints but not in the specification", key)
		}
	}

	handlers := New(t).handlers
	for key, e := range listed {
		_, implemented := handlers[key]
		if e.Object != "" && !implemented {
			t.Errorf("%s names %q, which the mock does not implement", key, e.Object)
		}
	}
	for key := range handlers {
		if _, ok := listed[key]; !ok {
			t.Errorf("mock implements %s, which is not in Endpoints", key)
		}
	}
}

// The CCP has no specification to compare with, so this names the mock's scope.
func TestCCPFunctions(t *testing.T) {
	want := []string{"infoDnsRecords", "infoDnsZone", "login", "logout", "updateDnsRecords", "updateDnsZone"}
	got := slices.Sorted(maps.Keys(New(t).ccpFunctions()))
	if !slices.Equal(got, want) {
		t.Fatalf("functions = %v, want %v", got, want)
	}
}

// coverageObjects renders the objects of an operation, naming only real ones as
// code.
func coverageObjects(objects string) string {
	var out []string
	for object := range strings.SplitSeq(objects, ", ") {
		name, note, noted := strings.Cut(object, " (")
		switch {
		case !strings.HasPrefix(name, "netcup_"):
			out = append(out, object)
		case noted:
			out = append(out, "`"+name+"` ("+note)
		default:
			out = append(out, "`"+name+"`")
		}
	}
	return strings.Join(out, ", ")
}

// coverageGuide renders templates/guides/api-coverage.md.
func coverageGuide() string {
	var out strings.Builder
	out.WriteString(`---
page_title: "API coverage"
subcategory: ""
description: |-
  Which netcup API operation each provider object uses.
---

# API coverage

The test suite runs real Terraform against a mock API that implements every
operation below. It checks each request and response against netcup's OpenAPI
specifications, and CCP requests against the CCP function list. The repository
also has live tests against the real API, see the README. They need credentials
and do not run by default.

## CCP API

| Function | Object |
|---|---|
| ` + "`login`" + ` | all CCP objects |
| ` + "`logout`" + ` | client only, the provider lets sessions expire |
| ` + "`infoDnsRecords`" + ` | ` + "`netcup_dns_record`, `netcup_dns_records`" + ` |
| ` + "`updateDnsRecords`" + ` | ` + "`netcup_dns_record`" + ` |
| ` + "`infoDnsZone`, `updateDnsZone`" + ` | ` + "`netcup_dns_zone`" + ` |

The other CCP functions (domains, handles, poll, prices) are not part of this
provider.

## SCP and netcup API

| Operation | Object |
|---|---|
`)
	var unmapped []string
	for _, e := range Endpoints {
		key := "`" + e.Method + " " + e.Path + "`"
		if e.Object == "" {
			unmapped = append(unmapped, key)
			continue
		}
		fmt.Fprintf(&out, "| %s | %s |\n", key, coverageObjects(e.Object))
	}
	out.WriteString("\n## Not mapped\n\nThese operations manage nothing, or form a draft workflow that a single record resource does not use:\n\n")
	for _, key := range unmapped {
		fmt.Fprintf(&out, "- %s\n", key)
	}
	return out.String()
}

// TestCoverageGuide keeps the guide equal to the table. Run it with
// UPDATE_COVERAGE=1 to write the guide.
func TestCoverageGuide(t *testing.T) {
	const path = "../../templates/guides/api-coverage.md"
	want := coverageGuide()
	if os.Getenv("UPDATE_COVERAGE") != "" {
		if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatal("the guide differs from Endpoints, run this test with UPDATE_COVERAGE=1")
	}
}
