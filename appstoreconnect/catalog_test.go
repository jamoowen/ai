package appstoreconnect

import (
	"strings"
	"testing"
)

func TestCatalogSearchAndDescribe(t *testing.T) {
	c, err := LoadCatalog([]byte(fixture))
	if err != nil {
		t.Fatal(err)
	}
	got := c.Search("filter name", "GET", 10)
	if len(got) != 1 || got[0].OperationID != "apps_get" {
		t.Fatalf("search: %#v", got)
	}
	if got := c.Search("get filter name", "", 10); len(got) != 1 {
		t.Fatalf("method/parameter search: %#v", got)
	}
	if got := c.Search("path identifier", "", 10); len(got) != 3 {
		t.Fatalf("path parameter search: %#v", got)
	}
	if got := c.Search("operation filter", "", 10); len(got) != 1 {
		t.Fatalf("operation parameter search: %#v", got)
	}
	if got := c.Search("get", "GET", 10); len(got) != 2 || got[0].OperationID != "apps_get" || got[1].OperationID != "apps_getCollection" {
		t.Fatalf("deterministic filter: %#v", got)
	}
	d, err := c.Describe("apps_get")
	if err != nil || d["operation"] == nil {
		t.Fatalf("describe: %v %#v", err, d)
	}
}

func TestCheckedInAppleSpecCompatibility(t *testing.T) {
	c, err := LoadCatalogFile("../api/apple/app-store-connect.openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Operation("apps_getCollection"); err != nil {
		t.Fatal(err)
	}
	description, err := c.Describe("accessibilityDeclarations_getInstance")
	if err != nil {
		t.Fatal(err)
	}
	parameters := description["operation"].(map[string]any)["parameters"].([]any)
	foundID := false
	for _, value := range parameters {
		parameter := value.(map[string]any)
		foundID = foundID || (parameter["name"] == "id" && parameter["in"] == "path")
	}
	if !foundID {
		t.Fatalf("Apple inherited path parameter missing: %#v", parameters)
	}
	mutation, err := c.Operation("apps_updateInstance")
	if err != nil || mutation.Method != "PATCH" {
		t.Fatalf("mutation: %#v %v", mutation, err)
	}
}

func TestCatalogRejectsMalformedMissingAndDuplicateOperationIDs(t *testing.T) {
	if _, err := LoadCatalog([]byte("{")); err == nil {
		t.Fatal("malformed spec accepted")
	}
	missing := strings.Replace(fixture, `"operationId":"apps_get",`, "", 1)
	if _, err := LoadCatalog([]byte(missing)); err == nil {
		t.Fatal("missing operation ID accepted")
	}
	duplicate := strings.Replace(fixture, "apps_patch", "apps_get", 1)
	if _, err := LoadCatalog([]byte(duplicate)); err == nil {
		t.Fatal("duplicate operation ID accepted")
	}
}

func TestDescribeIncludesTransitiveCyclicSchemas(t *testing.T) {
	spec := `{"openapi":"3.0.1","info":{"title":"cycle","version":"1"},"paths":{"/x":{"get":{"operationId":"cycle_get","responses":{"200":{"description":"ok","content":{"application/json":{"schema":{"$ref":"#/components/schemas/A"}}}}}}}},"components":{"schemas":{"A":{"type":"object","properties":{"b":{"$ref":"#/components/schemas/B"}}},"B":{"type":"object","properties":{"a":{"$ref":"#/components/schemas/A"}}}}}}`
	c, err := LoadCatalog([]byte(spec))
	if err != nil {
		t.Fatal(err)
	}
	d, err := c.Describe("cycle_get")
	if err != nil {
		t.Fatal(err)
	}
	schemas := d["schemas"].(map[string]any)
	if len(schemas) != 2 || schemas["A"] == nil || schemas["B"] == nil {
		t.Fatalf("schemas %#v", schemas)
	}
	if _, err := c.Describe("unknown"); err == nil {
		t.Fatal("unknown operation described")
	}
}

func TestDescribeReturnsEffectiveParameters(t *testing.T) {
	spec := strings.Replace(fixture, `"parameters":[{"name":"id","description":"path identifier","in":"path","required":true,"schema":{"type":"string"}}]`, `"parameters":[{"name":"id","description":"path identifier","in":"path","required":true,"schema":{"type":"string"}},{"name":"mode","description":"inherited mode","in":"query","schema":{"type":"string"}}]`, 1)
	spec = strings.Replace(spec, `{"name":"mode","in":"query","required":false,"schema":{"type":"string","enum":["one"]}}`, `{"name":"mode","description":"operation mode","in":"query","required":false,"schema":{"type":"string","enum":["one"]}}`, 1)
	c, err := LoadCatalog([]byte(spec))
	if err != nil {
		t.Fatal(err)
	}
	d, err := c.Describe("apps_get")
	if err != nil {
		t.Fatal(err)
	}
	parameters := d["operation"].(map[string]any)["parameters"].([]any)
	got := map[string]map[string]any{}
	for _, value := range parameters {
		parameter := value.(map[string]any)
		key := parameter["in"].(string) + ":" + parameter["name"].(string)
		if _, exists := got[key]; exists {
			t.Fatalf("duplicate parameter %q: %#v", key, parameters)
		}
		got[key] = parameter
	}
	if got["path:id"] == nil || got["query:filter[name]"] == nil || got["query:include"] == nil {
		t.Fatalf("effective parameters missing: %#v", got)
	}
	if got["query:mode"]["description"] != "operation mode" {
		t.Fatalf("operation parameter did not override inherited one: %#v", got["query:mode"])
	}
}

func TestDescribeResolvesReferencedEffectiveParameters(t *testing.T) {
	spec := `{"openapi":"3.0.1","info":{"title":"references","version":"1"},"paths":{"/v1/widgets/{id}":{"parameters":[{"$ref":"#/components/parameters/ID"},{"$ref":"#/components/parameters/Locale"}],"get":{"operationId":"widgets_get","parameters":[{"name":"locale","in":"query","description":"operation locale","schema":{"type":"string"}}],"responses":{"200":{"description":"ok"}}}}},"components":{"parameters":{"ID":{"name":"id","in":"path","required":true,"description":"component id","schema":{"type":"string"}},"Locale":{"name":"locale","in":"query","description":"component locale","schema":{"type":"string"}}}}}`
	c, err := LoadCatalog([]byte(spec))
	if err != nil {
		t.Fatal(err)
	}
	description, err := c.Describe("widgets_get")
	if err != nil {
		t.Fatal(err)
	}
	parameters := description["operation"].(map[string]any)["parameters"].([]any)
	got := map[string]map[string]any{}
	for _, value := range parameters {
		parameter := value.(map[string]any)
		if parameter["$ref"] != nil {
			t.Fatalf("parameter was not resolved: %#v", parameter)
		}
		key := parameter["in"].(string) + ":" + parameter["name"].(string)
		if _, exists := got[key]; exists {
			t.Fatalf("duplicate parameter %q: %#v", key, parameters)
		}
		got[key] = parameter
	}
	if got["path:id"] == nil || got["path:id"]["required"] != true || got["path:id"]["description"] != "component id" {
		t.Fatalf("referenced path parameter was not usable: %#v", got["path:id"])
	}
	if got["query:locale"] == nil || got["query:locale"]["description"] != "operation locale" {
		t.Fatalf("operation parameter did not override reference: %#v", got["query:locale"])
	}
}
