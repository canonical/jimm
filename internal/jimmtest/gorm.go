// Copyright 2020 Canonical Ltd.

// Package jimmtest contains useful helpers for testing JIMM.
package jimmtest

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/errors"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// A Tester is the test interface required by this package.
type Tester interface {
	Fatalf(format string, args ...interface{})
	Logf(format string, args ...interface{})
	Name() string
}

// A gormLogger is a gorm.Logger that is used in tests. It logs everything
// to the test.
type gormLogger struct {
	t     Tester
	level logger.LogLevel
}

// NewGormLogger returns a gorm logger.Interface that can be used in a test
// All output is logged to the test.
func NewGormLogger(t Tester, l logger.LogLevel) logger.Interface {
	return gormLogger{t: t, level: l}
}

func (l gormLogger) LogMode(_ logger.LogLevel) logger.Interface {
	return l
}

func (l gormLogger) Info(_ context.Context, fmt string, args ...interface{}) {
	if l.level >= logger.Info {
		l.t.Logf(fmt, args...)
	}
}

func (l gormLogger) Warn(_ context.Context, fmt string, args ...interface{}) {
	if l.level >= logger.Warn {
		l.t.Logf(fmt, args...)
	}
}

func (l gormLogger) Error(_ context.Context, fmt string, args ...interface{}) {
	if l.level >= logger.Error {
		l.t.Logf(fmt, args...)
	}
}

func (l gormLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, rows := fc()
	errS := "<nil>"
	if err != nil {
		errS = fmt.Sprintf("%q", err.Error())
	}
	l.Info(ctx, "sql:%q rows:%d, error:%s, duration:%0.3fms", sql, rows, errS, float64(time.Since(begin).Microseconds())/10e3)
}

var _ logger.Interface = gormLogger{}

// MemoryDB returns a PostgreSQL database instance for tests. To improve
// performance it creates a new database from a template (which has no data but
// is already-migrated).
// In cases where you need an entirely empty database, you should use
// `CreateEmptyDatabase` function in this package.
func MemoryDB(t Tester, nowFunc func() time.Time) *gorm.DB {
	_, present := os.LookupEnv("TERSE")
	logLevel := logger.Info
	if present {
		logLevel = logger.Warn
	}
	cfg := gorm.Config{
		Logger:  NewGormLogger(t, logLevel),
		NowFunc: nowFunc,
	}

	templateDatabaseName, _, err := getOrCreateTemplateDatabase()
	if err != nil {
		t.Fatalf("template database does not exist")
	}

	suggestedName := "jimm_test_" + t.Name()
	_, dsn, err := createDatabase(suggestedName, templateDatabaseName)
	if err != nil {
		t.Fatalf("error creating database (%s): %s", suggestedName, err)
	}

	gdb, err := gorm.Open(postgres.Open(dsn), &cfg)
	if err != nil {
		t.Fatalf("error opening database: %s", err)
	}

	return gdb
}

const unsafeCharsPattern = "[ .:;`'\"|<>~/\\?!@#$%^&*()[\\]{}=+-]"
const defaultDSN = "postgresql://jimm:jimm@127.0.0.1:5432/jimm"

// createDatabase creates a Postgres database and returns the created database
// name (which may be different than the requested name due to sanitization) and
// DSN. Note that:
//   - If `templateName` was empty, an empty database will be created.
//   - If the database was already exist, it'll be dropped and re-created.
func createDatabase(suggestedName string, templateName string) (string, string, error) {
	re, _ := regexp.Compile(unsafeCharsPattern)
	databaseName := strings.ToLower(re.ReplaceAllString(suggestedName, "_"))

	dsn := defaultDSN
	if envTestDSN, exists := os.LookupEnv("JIMM_TEST_PGXDSN"); exists {
		dsn = envTestDSN
	}

	u, err := url.Parse(dsn)
	if err != nil {
		return "", "", errors.E("error parsing DSN as a URI: %s", err)
	}

	gdb, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return "", "", errors.E(err, "error opening database")
	}

	dropDatabaseCommand := fmt.Sprintf(`DROP DATABASE IF EXISTS "%s"`, databaseName)
	if err := gdb.Exec(dropDatabaseCommand).Error; err != nil {
		return "", "", errors.E(err, fmt.Sprintf("error dropping existing database: %s", databaseName))
	}

	var createDatabaseCommand string
	if templateName != "" {
		createDatabaseCommand = fmt.Sprintf(`CREATE DATABASE "%s" TEMPLATE "%s"`, databaseName, templateName)
	} else {
		createDatabaseCommand = fmt.Sprintf(`CREATE DATABASE "%s"`, databaseName)
	}
	if err := gdb.Exec(createDatabaseCommand).Error; err != nil {
		return "", "", errors.E(err, fmt.Sprintf("error creating database: (%s)", databaseName))
	}

	u.Path = databaseName
	return databaseName, u.String(), nil
}

func createTemplateDatabase() (string, string, error) {
	templateName, templateDSN, err := createDatabase("jimm_template", "")
	if err != nil {
		return "", "", errors.E(err, "failed to create the template database")
	}

	gdb, err := gorm.Open(postgres.Open(templateDSN), &gorm.Config{})
	if err != nil {
		return "", "", errors.E(err, "error opening template database")
	}

	database := db.Database{
		DB: gdb,
	}
	if err := database.Migrate(context.Background(), true); err != nil {
		return "", "", errors.E(err, "error applying migrations on template database")
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		return "", "", errors.E(err, "failed to get the internal DB object")
	}
	if err := sqlDB.Close(); err != nil {
		return "", "", errors.E(err, "failed to close template database connection")
	}
	return templateName, templateDSN, nil
}

var createTemplateDBMutex = sync.Mutex{}
var templateDatabaseDSN string
var templateDatabaseName string

func getOrCreateTemplateDatabase() (string, string, error) {
	createTemplateDBMutex.Lock()
	defer createTemplateDBMutex.Unlock()
	if templateDatabaseDSN != "" {
		return templateDatabaseName, templateDatabaseDSN, nil
	}

	templateName, templateDSN, err := createTemplateDatabase()
	if err != nil {
		return "", "", errors.E(err, "error creating template database")
	}

	templateDatabaseDSN = templateDSN
	templateDatabaseName = templateName

	return templateDatabaseName, templateDatabaseDSN, nil
}

// CreateNewTestDatabase creates an empty Postgres database and returns the DSN.
func CreateEmptyDatabase(t Tester) string {
	_, dsn, err := createDatabase(t.Name(), "")
	if err != nil {
		t.Fatalf("error creating empty database: %s", err)
	}
	return dsn
}
