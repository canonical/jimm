// Copyright 2023 Canonical Ltd.

package jimm

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/servermon"
)

type DbAuditLogger struct {
	jimm           *JIMM
	conversationId string
	getUser        func() names.UserTag
}

// newConversationID generates a unique ID that is used for the
// lifetime of a websocket connection.
func newConversationID() string {
	buf := make([]byte, 8)
	rand.Read(buf) // Can't fail
	return hex.EncodeToString(buf)
}

// NewDbAuditLogger returns a new audit logger that logs to the database.
func NewDbAuditLogger(j *JIMM, getUserFunc func() names.UserTag) DbAuditLogger {
	logger := DbAuditLogger{
		jimm:           j,
		conversationId: newConversationID(),
		getUser:        getUserFunc,
	}
	return logger
}

func (r DbAuditLogger) newAuditLogEntry(header *rpc.Header) dbmodel.AuditLogEntry {
	ale := dbmodel.AuditLogEntry{
		Time:           time.Now().UTC().Round(time.Millisecond),
		MessageId:      header.RequestId,
		UserTag:        r.getUser().String(),
		ConversationId: r.conversationId,
	}
	return ale
}

// LogRequest creates an audit log entry from a client request.
func (r DbAuditLogger) LogRequest(header *rpc.Header, body interface{}) error {
	ale := r.newAuditLogEntry(header)
	ale.ObjectId = header.Request.Id
	ale.FacadeName = header.Request.Type
	ale.FacadeMethod = header.Request.Action
	ale.FacadeVersion = header.Request.Version
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			zapctx.Error(context.Background(), "failed to marshal body", zap.Error(err))
			return err
		}
		ale.Params = jsonBody
	}
	r.jimm.AddAuditLogEntry(&ale)
	return nil
}

// LogResponse creates an audit log entry from a controller response.
func (o DbAuditLogger) LogResponse(r rpc.Request, header *rpc.Header, body interface{}) error {
	var allErrors params.ErrorResults
	bulkError, ok := body.(params.ErrorResults)
	if ok {
		allErrors.Results = append(allErrors.Results, bulkError.Results...)
	}
	singleError := params.Error{
		Message: header.Error,
		Code:    header.ErrorCode,
		Info:    header.ErrorInfo,
	}
	allErrors.Results = append(allErrors.Results, params.ErrorResult{Error: &singleError})
	jsonErr, err := json.Marshal(allErrors)
	if err != nil {
		return err
	}
	ale := o.newAuditLogEntry(header)
	ale.ObjectId = r.Id
	ale.FacadeName = r.Type
	ale.FacadeMethod = r.Action
	ale.FacadeVersion = r.Version
	ale.Errors = jsonErr
	ale.IsResponse = true
	o.jimm.AddAuditLogEntry(&ale)
	return nil
}

// recorder implements an rpc.Recorder.
type recorder struct {
	start          time.Time
	logger         DbAuditLogger
	conversationId string
}

// NewRecorder returns a new recorder struct useful for recording RPC events.
func NewRecorder(logger DbAuditLogger) recorder {
	return recorder{
		start:          time.Now(),
		conversationId: newConversationID(),
		logger:         logger,
	}
}

// HandleRequest implements rpc.Recorder.
func (r recorder) HandleRequest(header *rpc.Header, body interface{}) error {
	return r.logger.LogRequest(header, body)
}

// HandleReply implements rpc.Recorder.
func (o recorder) HandleReply(r rpc.Request, header *rpc.Header, body interface{}) error {
	d := time.Since(o.start)
	servermon.WebsocketRequestDuration.WithLabelValues(r.Type, r.Action).Observe(float64(d) / float64(time.Second))
	return o.logger.LogResponse(r, header, body)
}
