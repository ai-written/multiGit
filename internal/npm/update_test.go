package npm

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestPatchJSON_InsertNewPkgs(t *testing.T) {
	raw := []byte(`{
  "devDependencies": {
    "typescript": "~5.7.2",
    "vite": "^6.1.0",
    "vue-tsc": "^2.2.0"
  }
}`)

	updates := []depUpdate{
		{name: "epaas-command", version: "0.0.2", section: "devDependencies"},
		{name: "epaas-core", version: "0.0.3", section: "devDependencies"},
	}

	result, updated := patchJSON(raw, updates, "")
	fmt.Printf("Updated: %v\n", updated)
	fmt.Printf("Result:\n%s\n", string(result))

	var parsed interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("Invalid JSON: %v\nResult:\n%s", err, string(result))
	}

	m := parsed.(map[string]interface{})
	dd := m["devDependencies"].(map[string]interface{})
	if v := dd["epaas-command"]; v != "0.0.2" {
		t.Errorf("epaas-command = %v, want 0.0.2", v)
	}
	if v := dd["epaas-core"]; v != "0.0.3" {
		t.Errorf("epaas-core = %v, want 0.0.3", v)
	}
}

func TestPatchJSON_ReplaceExisting(t *testing.T) {
	raw := []byte(`{
  "devDependencies": {
    "epaas-command": "0.0.1",
    "epaas-core": "0.0.2",
    "typescript": "~5.7.2"
  }
}`)

	updates := []depUpdate{
		{name: "epaas-command", version: "0.0.2", section: "devDependencies"},
		{name: "epaas-core", version: "0.0.3", section: "devDependencies"},
	}

	result, updated := patchJSON(raw, updates, "")
	fmt.Printf("Updated: %v\n", updated)
	fmt.Printf("Result:\n%s\n", string(result))

	var parsed interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("Invalid JSON: %v\nResult:\n%s", err, string(result))
	}

	m := parsed.(map[string]interface{})
	dd := m["devDependencies"].(map[string]interface{})
	if v := dd["epaas-command"]; v != "0.0.2" {
		t.Errorf("epaas-command = %v, want 0.0.2", v)
	}
	if v := dd["epaas-core"]; v != "0.0.3" {
		t.Errorf("epaas-core = %v, want 0.0.3", v)
	}
}


func TestPatchJSON_ReplaceAndBump(t *testing.T) {
	raw := []byte(`{
  "version": "1.0.0",
  "devDependencies": {
    "epaas-command": "0.0.1",
    "typescript": "~5.7.2"
  }
}`)

	updates := []depUpdate{
		{name: "epaas-command", version: "0.0.2", section: "devDependencies"},
	}

	result, updated := patchJSON(raw, updates, "1.0.1")
	fmt.Printf("Updated: %v\n", updated)
	fmt.Printf("Result:\n%s\n", string(result))

	var parsed interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("Invalid JSON: %v\nResult:\n%s", err, string(result))
	}

	m := parsed.(map[string]interface{})
	if v := m["version"]; v != "1.0.1" {
		t.Errorf("version = %v, want 1.0.1", v)
	}
	dd := m["devDependencies"].(map[string]interface{})
	if v := dd["epaas-command"]; v != "0.0.2" {
		t.Errorf("epaas-command = %v, want 0.0.2", v)
	}
}

func TestPatchJSON_InsertAndBump(t *testing.T) {
	raw := []byte(`{
  "version": "1.0.0",
  "devDependencies": {
    "typescript": "~5.7.2"
  }
}`)

	updates := []depUpdate{
		{name: "epaas-command", version: "0.0.2", section: "devDependencies"},
	}

	result, updated := patchJSON(raw, updates, "1.0.1")
	fmt.Printf("Updated: %v\n", updated)
	fmt.Printf("Result:\n%s\n", string(result))

	var parsed interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("Invalid JSON: %v\nResult:\n%s", err, string(result))
	}

	m := parsed.(map[string]interface{})
	if v := m["version"]; v != "1.0.1" {
		t.Errorf("version = %v, want 1.0.1", v)
	}
	dd := m["devDependencies"].(map[string]interface{})
	if v := dd["epaas-command"]; v != "0.0.2" {
		t.Errorf("epaas-command = %v, want 0.0.2", v)
	}
}
