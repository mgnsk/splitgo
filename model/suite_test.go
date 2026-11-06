package model_test

import (
	"testing"

	"github.com/mgnsk/splitgo"
	"github.com/mgnsk/webkit/sqlite"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/uptrace/bun"
)

var db *bun.DB

var _ = BeforeEach(func() {
	db = sqlite.NewDB(":memory:").WithForeignKeys().Connect()
	DeferCleanup(db.Close)

	Expect(sqlite.MigrateUp(db.DB, splitgo.Migrations, "migrations")).To(Succeed())
	DeferCleanup(func() error {
		return sqlite.MigrateDown(db.DB, splitgo.Migrations, "migrations")
	})
})

func TestSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "model")
}
