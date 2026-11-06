package model_test

import (
	"github.com/mgnsk/splitgo/domain"
	"github.com/mgnsk/splitgo/model"
	"github.com/mgnsk/webkit"
	"github.com/mgnsk/webkit/snowflake"
	. "github.com/mgnsk/webkit/testutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("users", func() {
	It("normalizes and finds users by email", func(ctx SpecContext) {
		u := &domain.User{ID: snowflake.Generate(), Email: "  Alice@Example.COM ", Currency: "EUR"}
		Expect(model.InsertUser(ctx, db, u)).To(Succeed())

		found := Must(model.GetUserByEmail(ctx, db, "alice@example.com"))
		Expect(found.ID).To(Equal(u.ID))
		Expect(found.Email).To(Equal("alice@example.com"))
		Expect(found.DisplayName()).To(Equal("alice"))
	})

	It("rejects duplicate emails", func(ctx SpecContext) {
		mustCreateUser(ctx, "a@example.com")
		err := model.InsertUser(ctx, db, &domain.User{ID: snowflake.Generate(), Email: "A@example.com", Currency: "EUR"})
		Expect(err).To(MatchError(webkit.AlreadyExists))
	})

	It("updates the profile", func(ctx SpecContext) {
		u := mustCreateUser(ctx, "a@example.com")
		u.Name = "Alice"
		u.Currency = "USD"
		Expect(model.UpdateUser(ctx, db, u)).To(Succeed())

		found := Must(model.GetUser(ctx, db, u.ID))
		Expect(found.DisplayName()).To(Equal("Alice"))
		Expect(found.Currency).To(Equal("USD"))
	})

	It("returns NotFound for unknown users", func(ctx SpecContext) {
		_, err := model.GetUserByEmail(ctx, db, "nobody@example.com")
		Expect(err).To(MatchError(webkit.NotFound))
	})
})
