package health

import (
	"context"
	"fmt"
	"os"

	"github.com/aetheris-dev/aetheris/server/internal/storage"
)

type Status string

const (
	StatusOK          Status = "ok"
	StatusDisabled    Status = "disabled"
	StatusUnavailable Status = "unavailable"
)

func OptionalProvider(ctx context.Context, blob storage.BlobStore, vector storage.VectorIndex) map[string]Status {
	result := map[string]Status{}
	if err := blob.Health(ctx); err != nil {
		result["object_storage"] = StatusDisabled
	} else {
		result["object_storage"] = StatusOK
	}
	if err := vector.Health(ctx); err != nil {
		result["vector_index"] = StatusDisabled
	} else {
		result["vector_index"] = StatusOK
	}
	return result
}

func DiskUsage(path string) (float64, error) {
	if !PathExists(path) {
		return 0, fmt.Errorf("path does not exist: %s", path)
	}
	return 0, fmt.Errorf("disk usage provider is platform-specific")
}

func PathExists(path string) bool { _, err := os.Stat(path); return err == nil }
