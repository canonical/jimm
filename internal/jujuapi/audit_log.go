// Copyright 2023 Canonical Ltd.

package jujuapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/jimm"
	"github.com/CanonicalLtd/jimm/internal/servermon"
	"github.com/juju/juju/rpc"
	"github.com/juju/names/v4"
)

type AuditLogger interface {
	LogRequest(header *rpc.Header, body interface{}) error
	LogResponse(r rpc.Request, header *rpc.Header, body interface{}) error
}

type dbLogger struct {
	jimm           *jimm.JIMM
	conversationId string
	getUser        func() names.UserTag
}

// NewConversationID generates a unique ID that is used for the
// lifetime of a websocket connection.
func NewConversationID() string {
	buf := make([]byte, 8)
	rand.Read(buf) // Can't fail
	return hex.EncodeToString(buf)
}

// NewDbLogger returns a new audit logger that logs to the database.
func NewDbLogger(j *jimm.JIMM, getUserFunc func() names.UserTag) dbLogger {
	logger := dbLogger{
		jimm:           j,
		conversationId: NewConversationID(),
		getUser:        getUserFunc,
	}
	return logger
}

// AddRequest
func (r dbLogger) LogRequest(header *rpc.Header, body interface{}) error {
	ale := dbmodel.AuditLogEntry{
		Time:           time.Now().UTC().Round(time.Millisecond),
		MessageId:      header.RequestId,
		UserTag:        r.getUser().String(),
		ConversationId: r.conversationId,
		ObjectId:       header.Request.Id,
		FacadeName:     header.Request.Type,
		FacadeMethod:   header.Request.Action,
		FacadeVersion:  header.Request.Version,
	}
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return err
		}
		ale.Body = jsonBody
	}
	r.jimm.AddAuditLogEntry(&ale)
	return nil
}

type jujuError struct {
	Error     string         `json:"error"`
	ErrorCode string         `json:"error-code"`
	ErrorInfo map[string]any `json:"error-info"`
}

// AddResponse
func (o dbLogger) LogResponse(r rpc.Request, header *rpc.Header, body interface{}) error {
	errInfo := jujuError{
		Error:     header.Error,
		ErrorCode: header.ErrorCode,
		ErrorInfo: header.ErrorInfo,
	}
	jsonErr, err := json.Marshal(errInfo)
	if err != nil {
		return err
	}
	ale := dbmodel.AuditLogEntry{
		Time:           time.Now().UTC().Round(time.Millisecond),
		MessageId:      header.RequestId,
		ConversationId: o.conversationId,
		UserTag:        o.getUser().String(),
		ObjectId:       r.Id,
		FacadeName:     r.Type,
		FacadeMethod:   r.Action,
		FacadeVersion:  r.Version,
		Errors:         jsonErr,
		IsResponse:     true,
	}
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return err
		}
		ale.Body = jsonBody
	}
	o.jimm.AddAuditLogEntry(&ale)
	return nil
}

// recorder implements an rpc.Recorder.
type recorder struct {
	start          time.Time
	logger         AuditLogger
	conversationId string
}

// NewRecorder returns a new recorder struct useful for recording RPC events.
func NewRecorder(logger AuditLogger) recorder {
	return recorder{
		start:          time.Now(),
		conversationId: NewConversationID(),
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
