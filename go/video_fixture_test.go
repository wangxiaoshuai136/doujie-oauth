package main

import (
	"os"
	"testing"
)

func TestParseRecordedAgentFixture(t *testing.T) {
	path := os.Getenv("DOUBAO_VIDEO_FIXTURE")
	if path == "" {
		t.Skip("DOUBAO_VIDEO_FIXTURE is not set")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	result, err := parseDoubaoAgentResponse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Videos) == 0 || result.TaskID == "" {
		t.Fatalf("unexpected parsed result: %#v", result)
	}
}
