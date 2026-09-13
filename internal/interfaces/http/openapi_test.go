package httpinterface

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
	"olixops-permission-center/pkg/config"
)

func TestOpenAPICoversRegisteredRoutes(t *testing.T) {
	raw, err := os.ReadFile("../../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Paths      map[string]map[string]any `yaml:"paths"`
		Components map[string]map[string]any `yaml:"components"`
	}
	if err = yaml.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	router := NewRouter(config.Config{})
	registered := make(map[string]bool)
	for _, route := range router.Routes() {
		key := strings.ToLower(route.Method)
		if _, ok := spec.Paths[route.Path][key]; !ok {
			t.Errorf("missing OpenAPI operation %s %s", route.Method, route.Path)
		}
		registered[key+" "+route.Path] = true
	}
	for path, operations := range spec.Paths {
		for method := range operations {
			if !registered[method+" "+path] {
				t.Errorf("documented route not registered: %s %s", method, path)
			}
		}
	}
	var checkRefs func(any)
	checkRefs = func(value any) {
		switch v := value.(type) {
		case map[string]any:
			for key, child := range v {
				if key == "$ref" {
					target, ok := child.(string)
					if !ok {
						t.Error("non-string OpenAPI reference")
						continue
					}
					parts := strings.Split(target, "/")
					if len(parts) != 4 || parts[0] != "#" || parts[1] != "components" {
						t.Errorf("unexpected reference %s", target)
						continue
					}
					if _, ok = spec.Components[parts[2]][parts[3]]; !ok {
						t.Errorf("unresolved reference %s", target)
					}
				}
				checkRefs(child)
			}
		case []any:
			for _, child := range v {
				checkRefs(child)
			}
		}
	}
	var document any
	if err = yaml.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	checkRefs(document)
}
