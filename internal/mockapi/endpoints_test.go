// SPDX-License-Identifier: MPL-2.0

package mockapi

import (
	"slices"
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
