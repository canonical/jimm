// Copyright 2025 Canonical.
package mocks

import (
	"context"

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimmjwx"
)

type JWTService struct {
	NewJWT_ func(context.Context, jimmjwx.JWTParams) ([]byte, error)
}

func (j JWTService) NewJWT(ctx context.Context, params jimmjwx.JWTParams) ([]byte, error) {
	if j.NewJWT_ == nil {
		return nil, errors.New("not implemented")
	}
	return j.NewJWT_(ctx, params)
}
