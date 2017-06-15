package zaputil

import (
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/errgo.v1"
)

// Error returns a field suitable for logging an error
// to a zap Logger along with its error trace.
// If err is nil, the field is a no-op.
func Error(err error) zapcore.Field {
	if err == nil {
		return zap.Skip()
	}
	return zap.Object("error", errObject{err})
}

// errObject is the type stored in the zap.Field. It implements
// MarshalJSON and also formats decently as an error (for example when
// used by zap.TextEncoder).
type errObject struct {
	error
}

func (e errObject) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if e.error == nil {
		return nil
	}
	enc.AddString("msg", e.error.Error())
	switch e.error.(type) {
	case errgo.Locationer:
	case errgo.Wrapper:
	default:
		// if this is not an errgo.Wrapper, or an
		// errgo.Locationer then there is no information to add.
		return nil
	}
	return errgo.Mask(enc.AddArray("trace", errorTrace{e.error}), errgo.Any)
}

type errorTrace struct {
	error error
}

func (t errorTrace) MarshalLogArray(enc zapcore.ArrayEncoder) error {
	err := t.error
	for err != nil {
		if eerr := enc.AppendObject(traceLevel{err}); eerr != nil {
			return errgo.Mask(eerr, errgo.Any)
		}
		if werr, ok := err.(errgo.Wrapper); ok {
			err = werr.Underlying()
		} else {
			break
		}
	}
	return nil
}

type traceLevel struct {
	error error
}

func (l traceLevel) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if loc := l.location(); loc != "" {
		enc.AddString("loc", loc)
	}
	if msg := l.message(); msg != "" {
		enc.AddString("msg", msg)
	}
	return nil
}

func (l traceLevel) message() string {
	if werr, ok := l.error.(errgo.Wrapper); ok {
		return werr.Message()
	}
	return l.error.Error()
}

func (l traceLevel) location() string {
	if lerr, ok := l.error.(errgo.Locationer); ok {
		if file, line := lerr.Location(); file != "" {
			return fmt.Sprintf("%s:%d", file, line)
		}
	}
	return ""
}
