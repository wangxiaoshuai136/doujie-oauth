package main

import (
	"testing"
)

func TestFlexNumberParsesGroupedAmounts(t *testing.T) {
	var value flexNumber
	if err := value.UnmarshalJSON([]byte(`"2,100"`)); err != nil {
		t.Fatal(err)
	}
	if float64(value) != 2100 {
		t.Fatalf("grouped amount parsed as %v", float64(value))
	}
	if err := value.UnmarshalJSON([]byte(`314`)); err != nil {
		t.Fatal(err)
	}
	if float64(value) != 314 {
		t.Fatalf("plain amount parsed as %v", float64(value))
	}
}
