package storage

import (
	"context"
	"fmt"
	"io"
)

type BlobRef struct {
	Provider, ObjectKey, ContentHash string
	SizeBytes                        int64
}

type BlobStore interface {
	Put(context.Context, BlobRef, io.Reader) error
	Delete(context.Context, BlobRef) error
	Health(context.Context) error
}

type DisabledBlobStore struct{}

func (DisabledBlobStore) Put(context.Context, BlobRef, io.Reader) error {
	return fmt.Errorf("object storage disabled")
}
func (DisabledBlobStore) Delete(context.Context, BlobRef) error { return nil }
func (DisabledBlobStore) Health(context.Context) error          { return nil }
