// Copyright 2020 Canonical Ltd.

// Package jimmtest contains useful helpers for testing JIMM.
package jimmtest

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/errors"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// A Tester is the test interface required by this package.
type Tester interface {
	Fatalf(format string, args ...interface{})
	Logf(format string, args ...interface{})
	Name() string
	Cleanup(f func())
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

// PostgresDB returns a PostgreSQL database instance for tests. To improve
// performance it creates a new database from a template (which has no data but
// is already-migrated).
// In cases where you need an entirely empty database, you should use
// `CreateEmptyDatabase` function in this package.
func PostgresDB(t Tester, nowFunc func() time.Time) *gorm.DB {
	_, present := os.LookupEnv("TERSE")
	logLevel := logger.Info
	if present {
		logLevel = logger.Warn
	}

	wrappedNowFunc := func() time.Time {
		var now time.Time
		if nowFunc != nil {
			now = nowFunc()
		} else {
			now = time.Now()
		}
		return now.Truncate(time.Microsecond)
	}
	cfg := gorm.Config{
		Logger:  NewGormLogger(t, logLevel),
		NowFunc: wrappedNowFunc,
	}

	templateDatabaseName, _, err := getOrCreateTemplateDatabase()
	if err != nil {
		t.Fatalf("template database does not exist")
	}

	_, dsn, err := createDatabaseFromTemplate(t.Name(), templateDatabaseName)
	if err != nil {
		t.Fatalf("error creating database (test %s): %s", t.Name(), err)
	}

	gdb, err := gorm.Open(postgres.Open(dsn), &cfg)
	if err != nil {
		t.Fatalf("error opening database: %s", err)
	}

	t.Cleanup(func() {
		sqlDB, err := gdb.DB()
		if err != nil {
			t.Logf("failed to get the internal DB object: %s", err)
		}
		if err := sqlDB.Close(); err != nil {
			t.Logf("failed to close database connection: %s", err)
		}
	})

	return gdb
}

const unsafeCharsPattern = "[ .:;`'\"|<>~/\\?!@#$%^&*()[\\]{}=+-]"
const defaultDSN = "postgresql://jimm:jimm@127.0.0.1:5432/jimm"

func GetDSN() string {
	dsn := defaultDSN
	if envTestDSN, exists := os.LookupEnv("JIMM_TEST_PGXDSN"); exists {
		dsn = envTestDSN
	}
	return dsn
}

// maxDatabaseNameLength Postgres's limit on database name length.
const maxDatabaseNameLength = 63

// computeSafeDatabaseName returns a database-safe name based on the given suggestion.
// Since there's a length limit of 63 chars for database names in Postgres, this
// method truncates longer names and also appends a hash at the end of it to make
// sure no name collisions occur and also future calls with the same suggested
// database name results in the same safe name.
func computeSafeDatabaseName(testName string) string {
	testName = "jimm_test_" + testName
	re, _ := regexp.Compile(unsafeCharsPattern)
	safeName := re.ReplaceAllString(testName, "_")

	hasher := sha1.New()
	hasher.Write([]byte(testName))
	sha := base64.URLEncoding.EncodeToString(hasher.Sum(nil))

	// Note that when using `base64.URLEncoding` the result may include a hyphen (-)
	// which is not a safe character for a database name, so we have to replace it.
	// See this for the table of chars when using `base64.URLEncoding`:
	//   - https://www.rfc-editor.org/rfc/rfc4648.html#section-5
	shaSafe := strings.ReplaceAll(strings.ReplaceAll(sha, "-", "_"), "=", "")
	shaSuffix := "_" + shaSafe[0:8]

	safeNameWithHash := strings.ToLower(safeName + shaSuffix)
	if len(safeNameWithHash) <= maxDatabaseNameLength {
		return safeNameWithHash
	}
	return strings.ToLower(safeName[:maxDatabaseNameLength-len(shaSuffix)] + shaSuffix)
}

// createDatabaseMutex to avoid issues at the time of creating databases, it's
// best to synchronize them to happen sequentially (specially, when creating a
// database from a template).
var createDatabaseMutex = sync.Mutex{}

// createDatabaseFromTemplate creates a Postgres database from a given template
// and returns the created database name (which may be different than the
// requested name due to sanitization) and DSN.
// If the database was already exist, it'll be dropped and re-created.
func createDatabaseFromTemplate(testName string, templateName string) (string, string, error) {
	databaseName := computeSafeDatabaseName(testName)

	dsn := GetDSN()

	createDatabaseMutex.Lock()
	defer createDatabaseMutex.Unlock()

	gdb, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return "", "", errors.E(err, "error opening database")
	}

	dropDatabaseCommand := fmt.Sprintf(`DROP DATABASE IF EXISTS "%s"`, databaseName)
	if err := gdb.Exec(dropDatabaseCommand).Error; err != nil {
		return "", "", errors.E(err, fmt.Sprintf("error dropping existing database (maybe there's an active connection like psql client): %s", databaseName))
	}

	createDatabaseCommand := fmt.Sprintf(`CREATE DATABASE "%s" TEMPLATE "%s"`, databaseName, templateName)
	if err := gdb.Exec(createDatabaseCommand).Error; err != nil {
		return "", "", errors.E(err, fmt.Sprintf("error creating database: (%s)", databaseName))
	}

	sqlDB, err := gdb.DB()
	if err != nil {
		return "", "", errors.E(err, "failed to get the internal DB object")
	}
	if err := sqlDB.Close(); err != nil {
		return "", "", errors.E(err, "failed to close database connection")
	}

	newDSN, err := createDSNToTemplateDatabase(databaseName)
	if err != nil {
		return "", "", errors.E(err, "failed to create new DSN")
	}
	return databaseName, newDSN, nil
}

func createDSNToTemplateDatabase(databaseName string) (string, error) {
	dsn := GetDSN()
	u, err := url.Parse(dsn)
	if err != nil {
		return "", errors.E("error parsing DSN as a URI: %s", err)
	}
	u.Path = databaseName
	return u.String(), nil
}

func getTestDBName(t Tester) string {
	dbName := computeSafeDatabaseName(t.Name())
	dsn, err := createDSNToTemplateDatabase(dbName)
	if err != nil {
		t.Fatalf("failed to get DSN to test DB")
	}
	return dsn
}

// createDatabase creates an empty Postgres database and returns the created
// database name (which may be different than the requested name due to
// sanitization) and DSN.
// If the database was already exist, it'll be dropped and re-created.
func createEmptyDatabase(testName string) (string, string, error) {
	databaseName := computeSafeDatabaseName(testName)

	dsn := defaultDSN
	if envTestDSN, exists := os.LookupEnv("JIMM_TEST_PGXDSN"); exists {
		dsn = envTestDSN
	}

	u, err := url.Parse(dsn)
	if err != nil {
		return "", "", errors.E("error parsing DSN as a URI: %s", err)
	}

	createDatabaseMutex.Lock()
	defer createDatabaseMutex.Unlock()

	gdb, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return "", "", errors.E(err, "error opening database")
	}

	dropDatabaseCommand := fmt.Sprintf(`DROP DATABASE IF EXISTS "%s"`, databaseName)
	if err := gdb.Exec(dropDatabaseCommand).Error; err != nil {
		return "", "", errors.E(err, fmt.Sprintf("error dropping existing database (maybe there's an active connection like psql client): %s", databaseName))
	}

	createDatabaseCommand := fmt.Sprintf(`CREATE DATABASE "%s"`, databaseName)
	if err := gdb.Exec(createDatabaseCommand).Error; err != nil {
		return "", "", errors.E(err, fmt.Sprintf("error creating database: (%s)", databaseName))
	}

	sqlDB, err := gdb.DB()
	if err != nil {
		return "", "", errors.E(err, "failed to get the internal DB object")
	}
	if err := sqlDB.Close(); err != nil {
		return "", "", errors.E(err, "failed to close database connection")
	}

	u.Path = databaseName
	return databaseName, u.String(), nil
}

func createTemplateDatabase() (string, string, error) {
	// Template databases should use unique names, in case multiple tests run at
	// the same time.
	suggestedName := fmt.Sprintf("jimm_template_%s", uuid.New().String()[0:8])
	templateName, templateDSN, err := createEmptyDatabase(suggestedName)
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

// CreateEmptyDatabase creates an empty Postgres database and returns the DSN.
func CreateEmptyDatabase(t Tester) string {
	_, dsn, err := createEmptyDatabase(t.Name())
	if err != nil {
		t.Fatalf("error creating empty database: %s", err)
	}
	return dsn
}
