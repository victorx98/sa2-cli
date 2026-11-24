package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"sort"
	"strings"

	"github.com/victorx98/sa2-cli/internal/api"
)

// Check the advertised wire contract independently of release metadata and other clients' routes.
func checkRuntime(target string) error {
	origin, err := api.Origin(target)
	if err != nil {
		return err
	}
	var operations []operation
	if err := readJSON("contracts/operations.json", &operations); err != nil {
		return err
	}
	var pinned map[string]any
	if err := readJSON(snapshot, &pinned); err != nil {
		return err
	}
	res, err := api.NewHTTPClient().Get(origin + "/api/docs-json")
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("OpenAPI returned HTTP %d", res.StatusCode)
	}
	const maxDocument = 16 << 20
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxDocument+1))
	if err != nil {
		return err
	}
	if len(raw) > maxDocument {
		return errors.New("runtime OpenAPI exceeds 16 MiB")
	}
	var live map[string]any
	if err := json.Unmarshal(raw, &live); err != nil {
		return err
	}
	if err := compareOperations(operations, pinned, live); err != nil {
		return err
	}
	fmt.Printf("%s: all %d CLI operations match\n", origin, len(operations))
	return nil
}

func compareOperations(operations []operation, pinned, live map[string]any) error {
	if len(operations) == 0 {
		return errors.New("empty CLI operation list")
	}
	var differences []error
	for _, op := range operations {
		label := strings.ToUpper(op.Method) + " " + op.Path
		want, err := project(pinned, []operation{op})
		if err != nil {
			return fmt.Errorf("CLI snapshot: %w", err)
		}
		got, err := project(live, []operation{op})
		if err != nil {
			differences = append(differences, fmt.Errorf("%s: %w", label, err))
			continue
		}
		delete(want, "info")
		delete(got, "info")
		if !reflect.DeepEqual(wireShape(want, "", false), wireShape(got, "", false)) {
			differences = append(differences, fmt.Errorf("%s: deployed contract differs", label))
		}
	}
	return errors.Join(differences...)
}

// Names inside properties, schemas and similar dictionaries can themselves be "description".
func wireShape(value any, field string, names bool) any {
	switch node := value.(type) {
	case map[string]any:
		out := make(map[string]any)
		for key, child := range node {
			if !names {
				switch key {
				case "description", "summary", "title", "examples", "example", "tags", "operationId", "externalDocs":
					continue
				case "properties", "schemas", "responses", "headers", "content", "paths", "securitySchemes":
					out[key] = wireShape(child, key, true)
					continue
				}
			}
			out[key] = wireShape(child, key, false)
		}
		return out
	case []any:
		out := make([]any, len(node))
		for i, item := range node {
			out[i] = wireShape(item, "", false)
		}
		switch field {
		case "parameters", "required", "enum":
			ordered := make([]string, len(out))
			for i, item := range out {
				b, _ := json.Marshal(item)
				ordered[i] = string(b)
			}
			sort.Strings(ordered)
			return ordered
		}
		return out
	default:
		return value
	}
}
