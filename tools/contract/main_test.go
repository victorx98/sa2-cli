package main

import (
	"encoding/json"
	"testing"
)

func TestProjectDependencies(t *testing.T) {
	var doc map[string]any
	raw := `{
		"openapi":"3.0.0", "info":{"title":"SA2","version":"1"},
		"security":[{"bearer":[]}],
		"paths":{"/used":{"post":{"responses":{"200":{"$ref":"#/components/responses/OK"}}},"get":{}}, "/other":{"get":{}}},
		"components":{
			"responses":{"OK":{"schema":{"$ref":"#/components/schemas/User"}}},
			"schemas":{"User":{"properties":{"manager":{"$ref":"#/components/schemas/User"}}},"Unrelated":{}},
			"securitySchemes":{"bearer":{"type":"http","scheme":"bearer"}}
		}
	}`
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatal(err)
	}
	operations := []operation{{Method: "post", Path: "/used"}}
	selected, err := project(doc, operations)
	if err != nil {
		t.Fatal(err)
	}
	paths := object(selected["paths"])
	components := object(selected["components"])
	if len(paths) != 1 || len(object(paths["/used"])) != 1 ||
		len(object(components["schemas"])) != 1 ||
		object(components["schemas"])["User"] == nil ||
		object(components["securitySchemes"])["bearer"] == nil {
		t.Fatal("projection must include only selected operations and their transitive dependencies")
	}
	delete(object(object(doc["components"])["schemas"]), "User")
	if _, err := project(doc, operations); err == nil {
		t.Fatal("projection accepted a missing component")
	}
	if _, err := project(doc, []operation{{Method: "delete", Path: "/used"}}); err == nil {
		t.Fatal("projection accepted a missing operation")
	}
}
