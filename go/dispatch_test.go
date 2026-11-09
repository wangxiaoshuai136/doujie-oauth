package main

import "testing"

func TestAuthIdentifiersDoNotExposeLegacyAliases(t *testing.T) {
	value, err := handleMethod("auth.identifier", nil)
	if err != nil {
		t.Fatal(err)
	}
	payload, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("unexpected auth identifier payload: %#v", value)
	}
	identifiers, ok := payload["identifiers"].([]string)
	if !ok {
		t.Fatalf("unexpected identifiers: %#v", payload["identifiers"])
	}
	for _, identifier := range identifiers {
		switch identifier {
		case browserEntryID, dolaBrowserEntryID, domesticQREntryID, internationalQREntryID, desktopEntryID:
			t.Fatalf("hidden compatibility provider exposed in entry picker: %s", identifier)
		}
	}
	entries, ok := payload["entries"].([]map[string]any)
	if !ok || len(entries) != 2 {
		t.Fatalf("unexpected entries: %#v", payload["entries"])
	}
}
