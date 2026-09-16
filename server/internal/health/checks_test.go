package health

import (
	"context"
	"testing"

	"github.com/aetheris-dev/aetheris/server/internal/storage"
)

func TestOptionalProvidersAreHealthyWhenDisabled(t *testing.T) {
	status := OptionalProvider(context.Background(), storage.DisabledBlobStore{}, storage.DisabledVectorIndex{})
	if status["object_storage"] != StatusOK || status["vector_index"] != StatusOK {
		t.Fatalf("status = %#v", status)
	}
}
