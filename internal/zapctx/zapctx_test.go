package zapctx_test

import (
	"bytes"
	"context"
	"io"

	"github.com/juju/testing"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jimm/internal/zapctx"
)

type zapctxSuite struct {
	testing.CleanupSuite
}

var _ = gc.Suite(&zapctxSuite{})

func (s *zapctxSuite) TestLogger(c *gc.C) {
	var buf bytes.Buffer
	logger := s.logger(&buf)
	ctx := zapctx.WithLogger(context.Background(), logger)
	zapctx.Logger(ctx).Info("hello")
	c.Assert(buf.String(), gc.Matches, `INFO\thello\n`)
}

func (s *zapctxSuite) TestDefaultLogger(c *gc.C) {
	var buf bytes.Buffer
	logger := s.logger(&buf)

	s.PatchValue(&zapctx.Default, logger)
	zapctx.Logger(context.Background()).Info("hello")
	c.Assert(buf.String(), gc.Matches, `INFO\thello\n`)
}

func (s *zapctxSuite) TestWithFields(c *gc.C) {
	var buf bytes.Buffer
	logger := s.logger(&buf)

	ctx := zapctx.WithLogger(context.Background(), logger)
	ctx = zapctx.WithFields(ctx, zap.Int("foo", 999), zap.String("bar", "whee"))
	zapctx.Logger(ctx).Info("hello")
	c.Assert(buf.String(), gc.Matches, `INFO\thello\t\{"foo": 999, "bar": "whee"\}\n`)
}

func (*zapctxSuite) logger(w io.Writer) *zap.Logger {
	config := zapcore.EncoderConfig{
		MessageKey:  "msg",
		LevelKey:    "level",
		EncodeLevel: zapcore.CapitalLevelEncoder,
	}
	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(config),
		zapcore.AddSync(w),
		zapcore.InfoLevel,
	)
	return zap.New(core)
}
