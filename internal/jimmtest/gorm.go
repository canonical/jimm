// Copyright 2024 Canonical.

// Package jimmtest contains useful helpers for testing JIMM.
package jimmtest

import (
	"context"
	//nolint:gosec // We're only using sha1 in tests.
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/errors"
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
	db, _ := PostgresDBWithDbName(t, nowFunc)
	return db
}

// PostgresDBWithDbName creates a Postgres DB for tests, returning the DB name.
// Useful for GoCheck tests that don't support a cleanup function and require the DB
// name for manual cleanup.
func PostgresDBWithDbName(t Tester, nowFunc func() time.Time) (*gorm.DB, string) {
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

	suggestedName := "jimm_test_" + t.Name()
	t.Logf("suggested db name: %s", suggestedName)
	databaseName, dsn, err := createDatabaseFromTemplate(suggestedName, templateDatabaseName)
	if err != nil {
		t.Fatalf("error creating database (%s): %s", suggestedName, err)
	}

	gdb, err := gorm.Open(postgres.Open(dsn), &cfg)
	if err != nil {
		t.Fatalf("error opening database: %s", err)
	}

	t.Cleanup(func() {
		dbCleanup(t, gdb, databaseName)
	})
	return gdb, databaseName
}

func dbCleanup(t Tester, gdb *gorm.DB, databaseName string) {
	if gdb != nil {
		sqlDB, err := gdb.DB()
		if err != nil {
			t.Logf("failed to get the internal DB object: %s", err)
		}
		if err := sqlDB.Close(); err != nil {
			t.Logf("failed to close database connection: %s", err)
		}
	}
	// Only delete the DB after closing connections to it.
	_, skipCleanup := os.LookupEnv("NO_DB_CLEANUP")
	if !skipCleanup {
		err := DeleteDatabase(databaseName)
		if err != nil {
			t.Logf("failed to delete database (%s): %s", databaseName, err)
		}
	}
}

const unsafeCharsPattern = "[ .:;`'\"|<>~/\\?!@#$%^&*()[\\]{}=+-]"
const defaultDSN = "postgresql://jimm:jimm@127.0.0.1:5432/jimm"

// maxDatabaseNameLength Postgres's limit on database name length.
const maxDatabaseNameLength = 63

// computeSafeDatabaseName returns a database-safe name based on the given suggestion.
// Since there's a length limit of 63 chars for database names in Postgres, this
// method truncates longer names and also appends a hash at the end of it to make
// sure no name collisions occur and also future calls with the same suggested
// database name results in the same safe name.
func computeSafeDatabaseName(suggestedName string) string {
	re := regexp.MustCompile(unsafeCharsPattern)
	safeName := re.ReplaceAllString(suggestedName, "_")

	//nolint:gosec // We're only using sha1 in tests.
	hasher := sha1.New()
	// Provide some random chars for the hash. Useful where tests
	// have the same suite name and same test name.
	hasher.Write([]byte(suggestedName + randSeq(5)))
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

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randSeq(n int) string {
	b := make([]rune, n)
	for i := range b {
		//nolint:gosec // We're only using rand.Intn for tests.
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

// createDatabaseMutex to avoid issues at the time of creating databases, it's
// best to synchronize them to happen sequentially (specially, when creating a
// database from a template).
var createDatabaseMutex = sync.Mutex{}
var deleteDatabaseMutex = sync.Mutex{}

// createDatabaseFromTemplate creates a Postgres database from a given template
// and returns the created database name (which may be different than the
// requested name due to sanitization) and DSN.
// If the database was already exist, it'll be dropped and re-created.
func createDatabaseFromTemplate(suggestedName string, templateName string) (string, string, error) {
	databaseName := computeSafeDatabaseName(suggestedName)

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

	u.Path = databaseName
	return databaseName, u.String(), nil
}

func DeleteDatabase(databaseName string) (err error) {
	dsn := defaultDSN
	if envTestDSN, exists := os.LookupEnv("JIMM_TEST_PGXDSN"); exists {
		dsn = envTestDSN
	}

	deleteDatabaseMutex.Lock()
	defer deleteDatabaseMutex.Unlock()

	gdb, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return errors.E(fmt.Sprintf("error opening database: %s", err))
	}
	db, err := gdb.DB()
	if err != nil {
		return errors.E(fmt.Sprintf("error getting db: %s", err))
	}
	defer func() { err = db.Close() }()

	dropDatabaseCommand := fmt.Sprintf(`DROP DATABASE IF EXISTS "%s"`, databaseName)
	if err := gdb.Exec(dropDatabaseCommand).Error; err != nil {
		return errors.E(fmt.Sprintf("failed to delete database (%s): %s", databaseName, err))
	}
	return nil
}

// createDatabase creates an empty Postgres database and returns the created
// database name (which may be different than the requested name due to
// sanitization) and DSN.
// If the database was already exist, it'll be dropped and re-created.
func createEmptyDatabase(suggestedName string) (string, string, error) {
	databaseName := computeSafeDatabaseName(suggestedName)

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
	databaseName, dsn, err := createEmptyDatabase("jimm_test_" + t.Name())
	if err != nil {
		t.Fatalf("error creating empty database: %s", err)
	}
	t.Cleanup(func() {
		dbCleanup(t, nil, databaseName)
	})
	return dsn
}
