package goai

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"
)

type TaggedSchemaFields struct {
	Value string `json:"value"`
}
type privateTaggedSchemaFields struct {
	Value string `json:"value"`
}

func checkEmbeddedSchemaKeys(t *testing.T, value any, raw json.RawMessage, nested string, nullable bool) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		t.Fatal(err)
	}
	schema := mustUnmarshal(t, raw)
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("missing properties")
	}
	var jsonKeys, schemaKeys []string
	for key := range object {
		jsonKeys = append(jsonKeys, key)
	}
	for key := range properties {
		schemaKeys = append(schemaKeys, key)
	}
	slices.Sort(jsonKeys)
	slices.Sort(schemaKeys)
	if !reflect.DeepEqual(jsonKeys, schemaKeys) {
		t.Fatalf("schema keys = %v, encoding/json keys = %v", schemaKeys, jsonKeys)
	}
	required := requiredList(t, schema)
	slices.Sort(required)
	if !reflect.DeepEqual(required, schemaKeys) {
		t.Fatalf("required = %v, want %v", required, schemaKeys)
	}
	if nested != "" {
		property := prop(t, schema, nested)
		if nullable {
			assertNullableType(t, property, "object")
		} else {
			assertType(t, property, "object")
		}
		assertHasProperty(t, property, "value")
		if property["additionalProperties"] != false {
			t.Fatal("nested object must remain strict")
		}
	}
}

func TestSchemaFrom_EmbeddedJSONTags(t *testing.T) {
	type Named struct {
		TaggedSchemaFields `json:"nested"`
	}
	type NamedPointer struct {
		*TaggedSchemaFields `json:"nested"`
	}
	type Hidden struct {
		TaggedSchemaFields `json:"-"`
		Keep               bool `json:"keep"`
	}
	type HiddenPointer struct {
		*TaggedSchemaFields `json:"-"`
		Keep                bool `json:"keep"`
	}
	type OptionsOnly struct {
		TaggedSchemaFields `json:",omitempty"`
	}
	type Hyphen struct {
		TaggedSchemaFields `json:"-,"` //nolint:staticcheck // SA5008: exercise Go 1.25 literal-hyphen JSON tags.
	}
	type HyphenOptions struct {
		TaggedSchemaFields `json:"-,omitempty"` //nolint:staticcheck // SA5008: exercise Go 1.25 literal-hyphen JSON tags.
	}
	type PrivateNamed struct {
		privateTaggedSchemaFields `json:"nested"`
	}
	type PrivateNamedPointer struct {
		*privateTaggedSchemaFields `json:"nested"`
	}
	type InvalidName struct {
		TaggedSchemaFields `json:"😀"`
	}
	type InvalidNamePointer struct {
		*TaggedSchemaFields `json:"😀"`
	}
	type Plain struct{ TaggedSchemaFields }
	for _, tt := range []struct {
		name     string
		value    any
		raw      json.RawMessage
		nested   string
		nullable bool
	}{
		{"named", Named{TaggedSchemaFields{"item"}}, SchemaFrom[Named](), "nested", false},
		{"named pointer", NamedPointer{&TaggedSchemaFields{"item"}}, SchemaFrom[NamedPointer](), "nested", true},
		{"ignored", Hidden{TaggedSchemaFields{"item"}, true}, SchemaFrom[Hidden](), "", false},
		{"ignored pointer", HiddenPointer{&TaggedSchemaFields{"item"}, true}, SchemaFrom[HiddenPointer](), "", false},
		{"options only", OptionsOnly{TaggedSchemaFields{"item"}}, SchemaFrom[OptionsOnly](), "", false},
		{"literal hyphen", Hyphen{TaggedSchemaFields{"item"}}, SchemaFrom[Hyphen](), "-", false},
		{"literal hyphen with options", HyphenOptions{TaggedSchemaFields{"item"}}, SchemaFrom[HyphenOptions](), "-", false},
		{"private named", PrivateNamed{privateTaggedSchemaFields{"item"}}, SchemaFrom[PrivateNamed](), "nested", false},
		{"private named pointer", PrivateNamedPointer{&privateTaggedSchemaFields{"item"}}, SchemaFrom[PrivateNamedPointer](), "nested", true},
		{"invalid name", InvalidName{TaggedSchemaFields{"item"}}, SchemaFrom[InvalidName](), "", false},
		{"invalid name pointer", InvalidNamePointer{&TaggedSchemaFields{"item"}}, SchemaFrom[InvalidNamePointer](), "", false},
		{"plain", Plain{TaggedSchemaFields{"item"}}, SchemaFrom[Plain](), "", false},
	} {
		t.Run(tt.name, func(t *testing.T) { checkEmbeddedSchemaKeys(t, tt.value, tt.raw, tt.nested, tt.nullable) })
	}
}

func TestSchemaFrom_LiteralHyphenWithOptions(t *testing.T) {
	type Result struct {
		Value string `json:"-,omitempty"` //nolint:staticcheck // SA5008: exercise Go 1.25 literal-hyphen JSON tags.
	}
	checkEmbeddedSchemaKeys(t, Result{"item"}, SchemaFrom[Result](), "", false)
}
