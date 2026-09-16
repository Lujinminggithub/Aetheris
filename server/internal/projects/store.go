package projects

import (
	"context"

	"github.com/aetheris-dev/aetheris/server/internal/authorization"
)

type Store interface {
	Register(context.Context, authorization.Principal, RegistrationBatch) (BatchResult, error)
	List(context.Context, string) ([]ProjectView, error)
}
