package k8s

import "context"

type ChangeType string

const (
	ChangeTypeUpsert ChangeType = "upsert"
	ChangeTypeDelete ChangeType = "delete"
)

type ChangeEvent struct {
	Kind      string
	Namespace string
	Name      string
	Change    ChangeType
}

type ChangeSink interface {
	Apply(ctx context.Context, event ChangeEvent) error
}
