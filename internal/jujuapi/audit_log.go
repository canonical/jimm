// Copyright 2023 Canonical Ltd.

package jujuapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/juju/juju/rpc"
	"github.com/juju/names/v4"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/jimm"
	"github.com/CanonicalLtd/jimm/internal/servermon"
)

type dbAuditLogger struct {
	jimm           *jimm.JIMM
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

// newDbAuditLogger returns a new audit logger that logs to the database.
func newDbAuditLogger(j *jimm.JIMM, getUserFunc func() names.UserTag) dbAuditLogger {
	logger := dbAuditLogger{
		jimm:           j,
		conversationId: newConversationID(),
		getUser:        getUserFunc,
	}
	return logger
}

func (r dbAuditLogger) newAuditMessage(header *rpc.Header) dbmodel.AuditLogEntry {
	ale := dbmodel.AuditLogEntry{
		Time:           time.Now().UTC().Round(time.Millisecond),
		MessageId:      header.RequestId,
		UserTag:        r.getUser().String(),
		ConversationId: r.conversationId,
	}
	return ale
}

// LogRequest creates an audit log entry from a client request.
func (r dbAuditLogger) LogRequest(header *rpc.Header, body interface{}) error {
	ale := r.newAuditMessage(header)
	ale.ObjectId = header.Request.Id
	ale.FacadeName = header.Request.Type
	ale.FacadeMethod = header.Request.Action
	ale.FacadeVersion = header.Request.Version
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

// LogResponse creates an audit log entry from a controller response.
func (o dbAuditLogger) LogResponse(r rpc.Request, header *rpc.Header, body interface{}) error {
	errInfo := jujuError{
		Error:     header.Error,
		ErrorCode: header.ErrorCode,
		ErrorInfo: header.ErrorInfo,
	}
	jsonErr, err := json.Marshal(errInfo)
	if err != nil {
		return err
	}
	ale := o.newAuditMessage(header)
	ale.ObjectId = r.Id
	ale.FacadeName = r.Type
	ale.FacadeMethod = r.Action
	ale.FacadeVersion = r.Version
	ale.Errors = jsonErr
	ale.IsResponse = true
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
	logger         dbAuditLogger
	conversationId string
}

// newRecorder returns a new recorder struct useful for recording RPC events.
func newRecorder(logger dbAuditLogger) recorder {
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
