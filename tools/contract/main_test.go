package main

import (
	"encoding/json"
	"strings"
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

func TestRuntimeChecksAllOperationsAndReferencedProperties(t *testing.T) {
	var pinned, live map[string]any
	docs := `{"openapi":"3.0.0","info":{},"paths":{"/book":{"get":{"responses":{"200":{"$ref":"#/components/schemas/Receipt"}}}},"/other":{"post":{}}},"components":{"schemas":{"Receipt":{"description":"old","properties":{"description":{"type":"string"}}}}}}`
	for _, target := range []*map[string]any{&pinned, &live} {
		if err := json.Unmarshal([]byte(docs), target); err != nil {
			t.Fatal(err)
		}
	}
	ops := []operation{{"get", "/book"}, {"post", "/other"}}
	receipt := object(object(object(live["components"])["schemas"])["Receipt"])
	receipt["description"] = "new documentation"
	if err := compareOperations(ops, pinned, live); err != nil {
		t.Fatal(err)
	}
	delete(object(receipt["properties"]), "description")
	delete(object(live["paths"]), "/other")
	err := compareOperations(ops, pinned, live)
	if err == nil || !strings.Contains(err.Error(), "GET /book") || !strings.Contains(err.Error(), "POST /other") {
		t.Fatalf("missing independent differences: %v", err)
	}
}
