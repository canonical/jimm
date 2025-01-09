// Copyright 2025 Canonical.

package jujuapi

import (
	"context"
	"encoding/json"
	"time"

	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/servermon"
	"github.com/canonical/jimm/v3/internal/utils"
)

// LogBackend defines the interface used by the Logger to store
// audit events.
type LogBackend interface {
	AddAuditLogEntry(ale *dbmodel.AuditLogEntry)
}

// auditLogger determines how to convert Juju RPC messages to the desired
// format and then sends logs to the backend for persistence.
type auditLogger struct {
	backend        LogBackend
	conversationId string
	getUser        func() names.UserTag
}

// newAuditLogger returns a new audit logger that logs to the provided backend.
func newAuditLogger(backend LogBackend, getUserFunc func() names.UserTag) auditLogger {
	logger := auditLogger{
		backend:        backend,
		conversationId: utils.NewConversationID(),
		getUser:        getUserFunc,
	}
	return logger
}

func (r auditLogger) newEntry(header *rpc.Header) dbmodel.AuditLogEntry {
	ale := dbmodel.AuditLogEntry{
		Time:           time.Now().UTC().Round(time.Millisecond),
		MessageId:      header.RequestId,
		IdentityTag:    r.getUser().String(),
		ConversationId: r.conversationId,
	}
	return ale
}

// LogRequest creates an audit log entry from a client request.
func (r auditLogger) LogRequest(header *rpc.Header, body interface{}) error {
	ale := r.newEntry(header)
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
	r.backend.AddAuditLogEntry(&ale)
	return nil
}

// LogResponse creates an audit log entry from a controller response.
func (o auditLogger) LogResponse(r rpc.Request, header *rpc.Header, body interface{}) error {
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
	ale := o.newEntry(header)
	ale.ObjectId = r.Id
	ale.FacadeName = r.Type
	ale.FacadeMethod = r.Action
	ale.FacadeVersion = r.Version
	ale.Errors = jsonErr
	ale.IsResponse = true
	o.backend.AddAuditLogEntry(&ale)
	return nil
}

// recorder implements an rpc.Recorder.
type recorder struct {
	start          time.Time
	logger         auditLogger
	conversationId string
}

// NewRecorder returns a new recorder struct useful for recording RPC events.
func NewRecorder(logger auditLogger) recorder {
	return recorder{
		start:          time.Now(),
		conversationId: utils.NewConversationID(),
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
