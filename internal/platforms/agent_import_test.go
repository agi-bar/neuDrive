package platforms

import "testing"

func TestAgentExportSchemaUsesStrictObjectRequirements(t *testing.T) {
	assertStrictObjectSchema(t, "agentExportSchema", agentExportSchema())
}

func assertStrictObjectSchema(t *testing.T, path string, node interface{}) {
	t.Helper()

	switch value := node.(type) {
	case map[string]interface{}:
		if schemaType, _ := value["type"].(string); schemaType == "object" {
			propertiesValue, hasProperties := value["properties"]
			if hasProperties {
				properties, ok := propertiesValue.(map[string]interface{})
				if !ok {
					t.Fatalf("%s properties should be an object", path)
				}
				required, ok := stringSlice(value["required"])
				if !ok {
					t.Fatalf("%s object schema must declare required keys", path)
				}
				if len(required) != len(properties) {
					t.Fatalf("%s required keys mismatch: got %v, want all property keys %v", path, required, mapKeys(properties))
				}
				requiredSet := make(map[string]struct{}, len(required))
				for _, key := range required {
					requiredSet[key] = struct{}{}
				}
				for key := range properties {
					if _, ok := requiredSet[key]; !ok {
						t.Fatalf("%s missing required key %q", path, key)
					}
				}
			}
		}
		for key, child := range value {
			assertStrictObjectSchema(t, path+"."+key, child)
		}
	case []interface{}:
		for idx, child := range value {
			assertStrictObjectSchema(t, path, child)
			_ = idx
		}
	}
}

func stringSlice(value interface{}) ([]string, bool) {
	switch typed := value.(type) {
	case []string:
		return typed, true
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, text)
		}
		return out, true
	default:
		return nil, false
	}
}

func mapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}
