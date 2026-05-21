// Copyright 2025 Canonical.

package juju

import "context"

type bootstrapJobIDKey struct{}

// WithBootstrapJobID returns a context tagged with the bootstrap job responsible for controller activation.
func WithBootstrapJobID(ctx context.Context, jobID int64) context.Context {
	return context.WithValue(ctx, bootstrapJobIDKey{}, jobID)
}

func bootstrapJobIDFromContext(ctx context.Context) (int64, bool) {
	jobID, ok := ctx.Value(bootstrapJobIDKey{}).(int64)
	return jobID, ok
}
