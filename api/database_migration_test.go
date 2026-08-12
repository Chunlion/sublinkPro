package api

import (
	"bytes"
	"strings"
	"testing"
)

func TestCopyDatabaseMigrationUploadEnforcesLimit(t *testing.T) {
	var destination bytes.Buffer
	err := copyDatabaseMigrationUpload(&destination, strings.NewReader("12345"), 4)
	if err == nil {
		t.Fatal("expected oversized migration upload to fail")
	}
	if destination.Len() != 5 {
		t.Fatalf("copied bytes = %d, want 5", destination.Len())
	}
}
