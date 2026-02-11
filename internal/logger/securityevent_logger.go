// Copyright 2026 Canonical.

package logger

import (
	"context"
	"fmt"
	"strings"

	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type eventLevel string

const (
	warning  eventLevel = "WARN"
	critical eventLevel = "CRITICAL"
)

type securityEvent struct {
	Event       string
	Description string
	Severity    eventLevel
}

func (s securityEvent) toZapFields() []zapcore.Field {
	return []zapcore.Field{
		zap.String("event", s.Event),
		zap.String("description", s.Description),
		zap.String("severity", string(s.Severity)),
	}
}

func logSecurityEvent(ctx context.Context, event securityEvent) {
	switch event.Severity {
	case warning:
		zapctx.Warn(ctx, event.Event, event.toZapFields()...)
	case critical:
		zapctx.Error(ctx, event.Event, event.toZapFields()...)
	}
}

// LogFailedLogin logs a failed login attempt for the given identityId.
func LogFailedLogin(ctx context.Context, identityId string) {
	logSecurityEvent(ctx, securityEvent{
		Event:       "authn_login_fail:" + identityId,
		Description: "login failed",
		Severity:    warning,
	})
}

// LogSuccessfulLogin logs a successful login attempt for the given identityId.
func LogSuccessfulLogin(ctx context.Context, identityId string) {
	logSecurityEvent(ctx, securityEvent{
		Event:       "authn_login_success:" + identityId,
		Description: "login succeeded",
		Severity:    warning,
	})
}

// LogUnauthorizedAccess logs an unauthorized access attempt for the given identityId.
func LogUnauthorizedAccess(ctx context.Context, identityId string, description string) {
	logSecurityEvent(ctx, securityEvent{
		Event:       "authz_fail:" + identityId,
		Description: description,
		Severity:    critical,
	})
}

// LogGrantJimmAdmins logs the granting of JIMM admin role to the given identityIds.
func LogGrantJimmAdmins(ctx context.Context, identityIds []string) {
	identities := strings.Join(identityIds, ", ")
	logSecurityEvent(ctx, securityEvent{
		Event:       "authz_admin:" + identities,
		Description: "JIMM admin role was granted.",
		Severity:    warning,
	})
}

// LogUserCreated logs the creation of a user, implicitly an identity
func LogUserCreated(ctx context.Context, identityId string) {
	logSecurityEvent(ctx, securityEvent{
		Event:       fmt.Sprintf("user_created:%s", identityId),
		Description: fmt.Sprintf("User %s was created.", identityId),
		Severity:    warning,
	})
}

// LogUserUpdated logs updates to a user, implicitly adding or removing an openfga relation
func LogUserUpdated(ctx context.Context, adminID string, userID string, relation string, target string, isAddition bool) {
	action := "add"
	if !isAddition {
		action = "remove"
	}
	logSecurityEvent(ctx, securityEvent{
		Event:       fmt.Sprintf("user_updated:%s,%s,%s,%s:%s", adminID, userID, action, relation, target), // ?
		Description: fmt.Sprintf("User %s updated %s to %s relation %s to object %s", adminID, userID, action, relation, target),
		Severity:    warning,
	})
}

// LogJimmStartup logs that JIMM has started.
func LogJimmStartup(ctx context.Context) {
	logSecurityEvent(ctx, securityEvent{
		Event:       "sys_startup",
		Description: "JIMM has started.",
		Severity:    warning,
	})
}

// LogJimmShutdown logs that JIMM is shutting down.
func LogJimmShutdown(ctx context.Context) {
	logSecurityEvent(ctx, securityEvent{
		Event:       "sys_shutdown",
		Description: "JIMM is shutting down.",
		Severity:    warning,
	})
}

// SystemMonitoringWarning prints a warning to stdout about potential issues with logging security events
// if the logger level is lower than warn.
func SystemMonitoringWarning(ctx context.Context, loggerLevel zapcore.Level) {
	if loggerLevel < zapcore.WarnLevel {
		logSecurityEvent(ctx, securityEvent{
			Event: "sys_monitor_disabled",
			Description: "Security events are using the default logger.\n" +
				fmt.Sprintf("Logger level '%s' may hide security events below this severity.\n", loggerLevel.String()) +
				"Set your logger to at least \"WARN\" level to ensure visibility of security events.\n",
			Severity: critical,
		})
	}
}
