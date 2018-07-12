package jujuapi

import (
	"context"
	"fmt"
	"strings"

	"github.com/CanonicalLtd/jem/internal/zapctx"
	"github.com/juju/bundlechanges"
	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/storage"
	charm "gopkg.in/juju/charm.v6"
)

// bundleAPI implements the Bundle facade.
type bundleAPI struct {
	root *controllerRoot
}

// GetChanges implements the GetChanges method on the Bundle facade.
//
// Note: This is copied from
// github.com/juju/juju/apiserver/facades/clientbundle/bundle.go and
// should be kept in sync with that.
func (b bundleAPI) GetChanges(args params.BundleChangesParams) (params.BundleChangesResults, error) {
	var results params.BundleChangesResults
	data, err := charm.ReadBundleData(strings.NewReader(args.BundleDataYAML))
	if err != nil {
		return results, errors.Annotate(err, "cannot read bundle YAML")
	}
	verifyConstraints := func(s string) error {
		_, err := constraints.Parse(s)
		return err
	}
	verifyStorage := func(s string) error {
		_, err := storage.ParseConstraints(s)
		return err
	}
	if err := data.Verify(verifyConstraints, verifyStorage); err != nil {
		if err, ok := err.(*charm.VerificationError); ok {
			results.Errors = make([]string, len(err.Errors))
			for i, e := range err.Errors {
				results.Errors[i] = e.Error()
			}
			return results, nil
		}
		// This should never happen as Verify only returns verification errors.
		return results, errors.Annotate(err, "cannot verify bundle")
	}
	changes, err := bundlechanges.FromData(
		bundlechanges.ChangesConfig{
			Bundle: data,
			Logger: zapLogger{b.root.context},
		})
	if err != nil {
		return results, err
	}
	results.Changes = make([]*params.BundleChange, len(changes))
	for i, c := range changes {
		results.Changes[i] = &params.BundleChange{
			Id:       c.Id(),
			Method:   c.Method(),
			Args:     c.GUIArgs(),
			Requires: c.Requires(),
		}
	}
	return results, nil
}

type zapLogger struct {
	ctx context.Context
}

func (l zapLogger) Tracef(s string, args ...interface{}) {
	zapctx.Debug(l.ctx, fmt.Sprintf(s, args...))
}
