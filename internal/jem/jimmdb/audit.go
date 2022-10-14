package jimmdb

import (
	"context"
	"time"

	"go.uber.org/zap"

	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v2/bakery/identchecker"
	"gopkg.in/mgo.v2/bson"

	"github.com/CanonicalLtd/jimm/internal/zapctx"
	"github.com/CanonicalLtd/jimm/internal/zaputil"
	"github.com/CanonicalLtd/jimm/params"
)

// AppendAudit appends the given entry to the audit log.
func (db *Database) AppendAudit(ctx context.Context, id identchecker.ACLIdentity, e params.AuditEntry) {
	common := e.Common()
	common.Created_ = time.Now()
	common.Originator = id.Id()
	common.Type_ = params.AuditLogType(e)

	if err := db.Audits().Insert(&params.AuditLogEntry{
		Content: e,
	}); err != nil {
		zapctx.Error(ctx, "cannot insert audit entry", zap.String("type", common.Type_), zaputil.Error(err))
		db.checkError(ctx, &err)
	}
}

// GetAuditEntries returns audit log entries based on the parameters passed in.
func (db *Database) GetAuditEntries(ctx context.Context, start time.Time, end time.Time, logType string) (entries params.AuditLogEntries, err error) {
	defer db.checkError(ctx, &err)
	query := make(bson.D, 0)
	if !start.IsZero() {
		query = append(query, bson.DocElem{"created", bson.D{{"$gte", start}}})
	}
	if !end.IsZero() {
		query = append(query, bson.DocElem{"created", bson.D{{"$lte", end}}})
	}
	if len(logType) > 0 {
		query = append(query, bson.DocElem{"type", logType})
	}
	if err = db.Audits().Find(query).Sort("created", "type").All(&entries); err != nil {
		return nil, errgo.Mask(err)
	}
	return entries, nil
}
