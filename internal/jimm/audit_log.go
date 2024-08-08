// Copyright 2024 Canonical.

package jimm

import (
	"context"
	"encoding/json"
	"time"

	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/servermon"
	"github.com/canonical/jimm/v3/internal/utils"
)

// AuditLoggerBackend defines the interface used by the DbAuditLogger to store
// audit events.
type AuditLoggerBackend interface {
	AddAuditLogEntry(*dbmodel.AuditLogEntry)
}

type DbAuditLogger struct {
	backend        AuditLoggerBackend
	conversationId string
	getUser        func() names.UserTag
}

// NewDbAuditLogger returns a new audit logger that logs to the database.
func NewDbAuditLogger(backend AuditLoggerBackend, getUserFunc func() names.UserTag) DbAuditLogger {
	logger := DbAuditLogger{
		backend:        backend,
		conversationId: utils.NewConversationID(),
		getUser:        getUserFunc,
	}
	return logger
}

func (r DbAuditLogger) newAuditLogEntry(header *rpc.Header) dbmodel.AuditLogEntry {
	ale := dbmodel.AuditLogEntry{
		Time:           time.Now().UTC().Round(time.Millisecond),
		MessageId:      header.RequestId,
		IdentityTag:    r.getUser().String(),
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
	r.backend.AddAuditLogEntry(&ale)
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
	o.backend.AddAuditLogEntry(&ale)
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

// AuditLogCleanupService is a service capable of cleaning up audit logs
// on a defined retention period. The retention period is in DAYS.
type auditLogCleanupService struct {
	auditLogRetentionPeriodInDays int
	db                            db.Database
}

// pollTimeOfDay holds the time hour, minutes and seconds to poll at.
type pollTimeOfDay struct {
	Hours   int
	Minutes int
	Seconds int
}

var pollDuration = pollTimeOfDay{
	Hours: 9,
}

// NewAuditLogCleanupService returns a service capable of cleaning up audit logs
// on a defined retention period. The retention period is in DAYS.
func NewAuditLogCleanupService(db db.Database, auditLogRetentionPeriodInDays int) *auditLogCleanupService {
	return &auditLogCleanupService{
		auditLogRetentionPeriodInDays: auditLogRetentionPeriodInDays,
		db:                            db,
	}
}

// Start starts a routine which checks daily for any logs
// needed to be cleaned up.
func (a *auditLogCleanupService) Start(ctx context.Context) {
	go a.poll(ctx)
}

// poll is designed to be run in a routine where it can be cancelled safely
// from the service's context. It calculates the poll duration at 9am each day
// UTC.
func (a *auditLogCleanupService) poll(ctx context.Context) {

	for {
		select {
		case <-time.After(calculateNextPollDuration(time.Now().UTC())):
			retentionDate := time.Now().AddDate(0, 0, -(a.auditLogRetentionPeriodInDays))
			deleted, err := a.db.DeleteAuditLogsBefore(ctx, retentionDate)
			if err != nil {
				zapctx.Error(ctx, "failed to cleanup audit logs", zap.Error(err))
				continue
			}
			zapctx.Debug(ctx, "audit log cleanup run successfully", zap.Int64("count", deleted))
		case <-ctx.Done():
			zapctx.Debug(ctx, "exiting audit log cleanup polling")
			return
		}
	}
}

// calculateNextPollDuration returns the next duration to poll on.
// We recalculate each time and not rely on running every 24 hours
// for absolute consistency within ns apart.
func calculateNextPollDuration(startingTime time.Time) time.Duration {
	now := startingTime
	nineAM := time.Date(now.Year(), now.Month(), now.Day(), pollDuration.Hours, 0, 0, 0, time.UTC)
	nineAMDuration := nineAM.Sub(now)
	var d time.Duration
	// If 9am is behind the current time, i.e., 1pm
	if nineAMDuration < 0 {
		// Add 24 hours, flip it to an absolute duration, i.e., -10h == 10h
		// and subtract it from 24 hours to calculate 9am tomorrow
		d = time.Hour*24 - nineAMDuration.Abs()
	} else {
		d = nineAMDuration.Abs()
	}
	return d
}
