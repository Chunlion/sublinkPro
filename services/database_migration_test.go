package services

import (
	"archive/zip"
	"testing"
)

func TestValidateMigrationArchiveRejectsOversizedContent(t *testing.T) {
	files := []*zip.File{{FileHeader: zip.FileHeader{UncompressedSize64: uint64(databaseMigrationExtractSizeLimit) + 1}}}
	if err := validateMigrationArchive(files); err == nil {
		t.Fatal("expected oversized migration archive to fail")
	}
}
