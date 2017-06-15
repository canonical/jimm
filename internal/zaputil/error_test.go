package zaputil_test

import (
	"bytes"
	"io"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	gc "gopkg.in/check.v1"
	errgo "gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jem/internal/zaputil"
)

type zaputilSuite struct{}

var _ = gc.Suite(&zaputilSuite{})

func (s *zaputilSuite) TestErrorJSONEncoder(c *gc.C) {
	var buf bytes.Buffer
	logger := s.logger(&buf)

	err := errgo.New("something")
	err = errgo.Mask(err)
	err = errgo.Notef(err, "an error")
	logger.Info("a message", zaputil.Error(err))
	c.Assert(buf.String(), gc.Matches, `\{"level":"info","ts":[0-9.]+,"msg":"a message","error":\{"msg":"an error: something","trace":\[\{"loc":".*zaputil/error_test.go:[0-9]+","msg":"an error"\},\{"loc":".*zaputil/error_test.go:[0-9]+"\},\{"loc":".*zaputil/error_test.go:[0-9]+","msg":"something"\}\]\}\}\n`)
}

func (s *zaputilSuite) TestTextEncoder(c *gc.C) {
	var buf bytes.Buffer
	logger := s.consoleLogger(&buf)

	err := errgo.New("something")
	err = errgo.Mask(err)
	err = errgo.Notef(err, "an error")
	logger.Info("a message", zaputil.Error(err))
	c.Assert(buf.String(), gc.Matches, `INFO	a message	\{"error": \{"msg": "an error: something", "trace": \[\{"loc": ".*zaputil/error_test.go:[0-9]+", "msg": "an error"\}, \{"loc": ".*zaputil/error_test.go:[0-9]+"\}, \{"loc": ".*zaputil/error_test.go:[0-9]+", "msg": "something"\}\]\}\}\n`)
}

func (s *zaputilSuite) TestNilError(c *gc.C) {
	var buf bytes.Buffer
	logger := s.consoleLogger(&buf)

	logger.Info("a message", zaputil.Error(nil))
	c.Assert(buf.String(), gc.Matches, `INFO	a message\n`)
}

func (s *zaputilSuite) TestSimpleError(c *gc.C) {
	var buf bytes.Buffer
	logger := s.logger(&buf)

	logger.Info("a message", zaputil.Error(io.EOF))
	c.Assert(buf.String(), gc.Matches, `\{"level":"info","ts":[0-9.]+,"msg":"a message","error":\{"msg":"EOF"\}\}\n`)
}

func (*zaputilSuite) logger(w io.Writer) *zap.Logger {
	config := zapcore.EncoderConfig{
		MessageKey:  "msg",
		LevelKey:    "level",
		TimeKey:     "ts",
		EncodeLevel: zapcore.LowercaseLevelEncoder,
		EncodeTime:  zapcore.EpochTimeEncoder,
	}
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(config),
		zapcore.AddSync(w),
		zapcore.InfoLevel,
	)
	return zap.New(core)
}

func (*zaputilSuite) consoleLogger(w io.Writer) *zap.Logger {
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
