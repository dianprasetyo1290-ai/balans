package db_test

import (
	"os"
	"testing"

	"balans/internal/platform/db/dbtest"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}
