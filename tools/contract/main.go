package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
)

const snapshot = "contracts/openapi.cli.json"

type operation struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

func main() {
	source := flag.String("source", "", "pinned upstream OpenAPI document")
	check := flag.Bool("check", false, "check the committed snapshot")
	runtime := flag.String("runtime", "", "compare the deployed API at this origin to the CLI snapshot (read only)")
	flag.Parse()
	var err error
	switch {
	case *runtime != "" && *source != "":
		err = errors.New("choose -runtime or -source")
	case *runtime != "":
		err = checkRuntime(*runtime)
	default:
		err = run(*source, *check)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(source string, check bool) error {
	var operations []operation
	if err := readJSON("contracts/operations.json", &operations); err != nil {
		return err
	}
	if source == "" && !check {
		return errors.New("provide -source to generate the snapshot")
	}
	if source == "" {
		source = snapshot
	}
	raw, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if source != snapshot {
		var metadata struct {
			Blob string `json:"blob"`
		}
		if err := readJSON("contracts/source.json", &metadata); err != nil {
			return err
		}
		digest := sha1.New()
		fmt.Fprintf(digest, "blob %d\x00", len(raw))
		digest.Write(raw)
		if fmt.Sprintf("%x", digest.Sum(nil)) != metadata.Blob {
			return errors.New("upstream OpenAPI Git blob differs from contracts/source.json")
		}
	}

	doc := make(map[string]any)
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&doc); err != nil {
		return err
	}
	selected, err := project(doc, operations)
	if err != nil {
		return err
	}
	formatted, err := json.MarshalIndent(selected, "", "  ")
	if err != nil {
		return err
	}
	formatted = append(formatted, '\n')
	if !check {
		return os.WriteFile(snapshot, formatted, 0644)
	}
	onDisk, err := os.ReadFile(snapshot)
	if err != nil {
		return err
	}
	if !bytes.Equal(onDisk, formatted) {
		return errors.New("CLI OpenAPI snapshot must be regenerated from the pinned source")
	}
	return nil
}

func readJSON(path string, target any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}

func project(source map[string]any, operations []operation) (map[string]any, error) {
	if len(operations) == 0 || source["openapi"] != "3.0.0" || source["info"] == nil {
		return nil, errors.New("expected an OpenAPI 3.0.0 source and a nonempty list of operations")
	}
	paths := make(map[string]any)
	components := make(map[string]any)
	selected := map[string]any{"openapi": source["openapi"], "info": source["info"], "paths": paths, "components": components}
	p := projector{source: source, components: components}
	for _, op := range operations {
		original := object(object(source["paths"])[op.Path])
		entry := object(original[op.Method])
		if entry == nil {
			return nil, fmt.Errorf("missing operation %s %s", op.Method, op.Path)
		}
		item := object(paths[op.Path])
		if item == nil {
			item = make(map[string]any)
			paths[op.Path] = item
			if parameters, ok := original["parameters"]; ok {
				item["parameters"] = parameters
				if err := p.references(parameters); err != nil {
					return nil, err
				}
			}
		}
		if item[op.Method] != nil {
			return nil, fmt.Errorf("duplicate operation %s %s", op.Method, op.Path)
		}
		item[op.Method] = entry
		if err := p.references(entry); err != nil {
			return nil, err
		}
		security, declared := entry["security"]
		if !declared {
			security = source["security"]
			if security != nil {
				selected["security"] = security
			}
		}
		if schemes, ok := security.([]any); ok {
			for _, scheme := range schemes {
				for name := range object(scheme) {
					if err := p.include("securitySchemes", name); err != nil {
						return nil, err
					}
				}
			}
		}
	}
	return selected, nil
}

type projector struct {
	source     map[string]any
	components map[string]any
}

func (p projector) references(value any) error {
	switch node := value.(type) {
	case map[string]any:
		for key, child := range node {
			if key != "$ref" {
				if err := p.references(child); err != nil {
					return err
				}
				continue
			}
			ref, ok := child.(string)
			parts := strings.Split(ref, "/")
			if !ok || len(parts) != 4 || parts[0] != "#" || parts[1] != "components" {
				return fmt.Errorf("unsupported OpenAPI reference %q", ref)
			}
			unescape := strings.NewReplacer("~1", "/", "~0", "~")
			if err := p.include(parts[2], unescape.Replace(parts[3])); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range node {
			if err := p.references(child); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p projector) include(kind, name string) error {
	group := object(p.components[kind])
	if group == nil {
		group = make(map[string]any)
		p.components[kind] = group
	}
	if group[name] != nil {
		return nil
	}
	component := object(object(p.source["components"])[kind])[name]
	if component == nil {
		return fmt.Errorf("OpenAPI component %s/%s is missing", kind, name)
	}
	group[name] = component
	return p.references(component)
}

func object(value any) map[string]any {
	object, _ := value.(map[string]any)
	return object
}
