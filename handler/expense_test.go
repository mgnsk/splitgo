package handler_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"

	"github.com/mgnsk/splitgo/domain"
	"github.com/mgnsk/splitgo/model"
	"github.com/mgnsk/webkit/snowflake"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var expenseLinkRe = regexp.MustCompile(`/groups/\d+/expenses/(\d+)"`)

var _ = Describe("expenses", func() {
	var (
		env                 *testEnv
		alice, bob          *client
		aliceU, bobU, carol *domain.User
		group               string
		groupID             snowflake.ID
	)

	BeforeEach(func(ctx SpecContext) {
		env = newTestEnv(false)

		alice = env.newClient()
		aliceU = alice.login("alice@example.com")
		bobU = mustCreateUser(ctx, "bob@example.com")
		bob = env.newClient()
		bob.login("bob@example.com")

		group = alice.createGroup("Flat")
		Expect(groupID.UnmarshalText([]byte(group[len("/groups/"):]))).To(Succeed())
		Expect(model.AddGroupMember(ctx, db, groupID, bobU.ID)).To(Succeed())
	})

	addCarol := func(ctx context.Context) {
		carol = mustCreateUser(ctx, "carol@example.com")
		Expect(model.AddGroupMember(ctx, db, groupID, carol.ID)).To(Succeed())
	}

	balances := func(c *client) string {
		GinkgoHelper()
		resp := c.get(group)
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		return text(resp.body)
	}

	firstExpensePath := func(c *client) string {
		GinkgoHelper()
		m := expenseLinkRe.FindStringSubmatch(c.get(group).body)
		Expect(m).To(HaveLen(2))
		return group + "/expenses/" + m[1]
	}

	Describe("simple split presets", func() {
		DescribeTable("produce the expected balances for both people",
			func(preset, aliceSees, bobSees string) {
				alice.addExpense(group, url.Values{"amount": {"10"}, "mode": {"simple"}, "preset": {preset}})
				Expect(balances(alice)).To(ContainSubstring(aliceSees))
				Expect(balances(bob)).To(ContainSubstring(bobSees))
			},
			Entry("A: you paid, split equally", "you-paid-equal", "bob owes you €5.00", "You owe alice €5.00"),
			Entry("B: you are owed the full amount", "you-owed-full", "bob owes you €10.00", "You owe alice €10.00"),
			Entry("C: they paid, split equally", "they-paid-equal", "You owe bob €5.00", "alice owes you €5.00"),
			Entry("D: they are owed the full amount", "they-owed-full", "You owe bob €10.00", "alice owes you €10.00"),
		)

		It("renders the four options with live amounts", func() {
			resp := alice.post(group+"/expenses/split?part=preview", url.Values{
				"amount": {"10.01"}, "currency": {"EUR"}, "mode": {"simple"}, "preset": {"you-paid-equal"},
			}, true)
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			t := text(resp.body)
			Expect(t).To(ContainSubstring("You paid, split equally bob owes you €5.00"))
			Expect(t).To(ContainSubstring("You are owed the full amount bob owes you €10.01"))
			Expect(t).To(ContainSubstring("bob paid, split equally You owe bob €5.00"))
			Expect(t).To(ContainSubstring("bob is owed the full amount You owe bob €10.01"))
		})

		It("asks for an amount before showing sentences", func() {
			resp := alice.post(group+"/expenses/split?part=preview", url.Values{"amount": {""}, "currency": {"EUR"}}, true)
			Expect(text(resp.body)).To(ContainSubstring("Enter an amount"))
		})
	})

	Describe("larger groups", func() {
		BeforeEach(func(ctx SpecContext) {
			addCarol(ctx)
		})

		It("offers paid by and equal split in the simple view", func() {
			body := alice.get(group + "/expenses/new").body
			Expect(body).To(ContainSubstring(`name="paid_by"`))
			Expect(body).NotTo(ContainSubstring(`name="preset"`))
			Expect(text(body)).To(ContainSubstring("split equally among everyone"))
		})

		It("splits equally among everyone", func() {
			alice.addExpense(group, url.Values{"amount": {"30"}, "paid_by": {aliceU.ID.String()}})
			b := balances(alice)
			Expect(b).To(ContainSubstring("bob owes you €10.00"))
			Expect(b).To(ContainSubstring("carol owes you €10.00"))
		})

		It("simplifies debts when enabled", func() {
			// carol owes bob 10 and bob owes alice 10 => carol pays alice.
			alice.addExpense(group, url.Values{
				"amount": {"10"}, "mode": {"advanced"}, "split_type": {"shares"},
				"paid_by": {bobU.ID.String()}, "value_" + carol.ID.String(): {"1"},
			})
			alice.addExpense(group, url.Values{
				"amount": {"10"}, "mode": {"advanced"}, "split_type": {"shares"},
				"paid_by": {aliceU.ID.String()}, "value_" + bobU.ID.String(): {"1"},
			})
			b := balances(alice)
			Expect(b).To(ContainSubstring("carol owes bob €10.00"))
			Expect(b).To(ContainSubstring("bob owes you €10.00"))

			alice.post(group+"/settings", url.Values{"name": {"Flat"}, "currency": {"EUR"}, "simplify_debts": {"true"}}, false)
			b = balances(alice)
			Expect(b).To(ContainSubstring("carol owes you €10.00"))
			Expect(b).NotTo(ContainSubstring("bob owes"))
		})
	})

	Describe("advanced split", func() {
		It("switches from a preset keeping payer and participants", func() {
			resp := alice.post(group+"/expenses/split?part=section", url.Values{
				"amount": {"30"}, "currency": {"EUR"}, "mode": {"simple"}, "preset": {"they-owed-full"}, "switch": {"advanced"},
			}, true)
			Expect(resp.body).To(MatchRegexp(fmt.Sprintf(`<option value="%s" selected>bob</option>`, bobU.ID)))
			Expect(resp.body).To(ContainSubstring(fmt.Sprintf(`name="value_%s" value="100"`, aliceU.ID)))
			Expect(resp.body).To(ContainSubstring(fmt.Sprintf(`name="value_%s" value="0"`, bobU.ID)))
			Expect(resp.body).NotTo(ContainSubstring(`value="equal"`))
		})

		It("starts a two-person percentage split at 50/50 with a slider", func() {
			resp := alice.post(group+"/expenses/split?part=values", url.Values{
				"amount": {"30"}, "currency": {"EUR"}, "mode": {"advanced"}, "split_type": {"percentage"},
				"paid_by": {aliceU.ID.String()},
			}, true)
			Expect(resp.body).To(ContainSubstring(`data-pair="percentage"`))
			Expect(resp.body).To(ContainSubstring(fmt.Sprintf(`name="value_%s" value="50"`, bobU.ID)))
			Expect(resp.body).To(MatchRegexp(`type="range"[^>]*value="50"`))
			Expect(resp.body).To(ContainSubstring("€15.00"))
			// The selects are left in place, only the values are swapped.
			Expect(resp.body).NotTo(ContainSubstring(`<select`))
		})

		It("saves a percentage split", func() {
			alice.addExpense(group, url.Values{
				"amount": {"100"}, "mode": {"advanced"}, "split_type": {"percentage"}, "paid_by": {aliceU.ID.String()},
				"value_" + aliceU.ID.String(): {"25"}, "value_" + bobU.ID.String(): {"75"},
			})
			Expect(balances(alice)).To(ContainSubstring("bob owes you €75.00"))

			// Reopens in advanced mode with the original percentages.
			edit := alice.get(firstExpensePath(alice) + "/edit").body
			Expect(edit).To(ContainSubstring(fmt.Sprintf(`name="value_%s" value="75"`, bobU.ID)))
			Expect(edit).To(ContainSubstring(`<option value="percentage" selected>`))
		})

		It("previews errors live", func() {
			resp := alice.post(group+"/expenses/split?part=preview", url.Values{
				"amount": {"100"}, "currency": {"EUR"}, "mode": {"advanced"}, "split_type": {"percentage"},
				"paid_by": {aliceU.ID.String()}, "value_" + aliceU.ID.String(): {"50"},
			}, true)
			Expect(text(resp.body)).To(ContainSubstring("Percentages add up to 50%, not 100%"))
		})

		It("previews exact shares", func() {
			resp := alice.post(group+"/expenses/split?part=preview", url.Values{
				"amount": {"100"}, "currency": {"EUR"}, "mode": {"advanced"}, "split_type": {"exact"},
				"paid_by": {aliceU.ID.String()}, "value_" + aliceU.ID.String(): {"60"}, "value_" + bobU.ID.String(): {"40"},
			}, true)
			Expect(text(resp.body)).To(ContainSubstring("You €60.00 bob €40.00"))
		})

		It("re-renders the form with errors on an invalid split", func() {
			resp := alice.post(group+"/expenses", url.Values{
				"description": {"Rent"}, "amount": {"100"}, "currency": {"EUR"}, "date": {"2026-11-03"},
				"mode": {"advanced"}, "split_type": {"exact"}, "paid_by": {aliceU.ID.String()},
				"value_" + aliceU.ID.String(): {"60"},
			}, false)
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(text(resp.body)).To(ContainSubstring("Amounts add up to €60.00, €40.00 left to assign"))
			Expect(resp.body).To(ContainSubstring(`value="Rent"`))
			Expect(balances(alice)).To(ContainSubstring("Everyone is settled up"))
		})
	})

	It("validates the expense form", func() {
		resp := alice.post(group+"/expenses", url.Values{"description": {""}, "amount": {"abc"}, "currency": {"EUR"}, "date": {"2026-11-03"}}, false)
		t := text(resp.body)
		Expect(t).To(ContainSubstring("Description is required"))
		Expect(t).To(ContainSubstring("Invalid amount"))
	})

	It("rejects a payer outside the group", func() {
		resp := alice.post(group+"/expenses", url.Values{
			"description": {"x"}, "amount": {"10"}, "currency": {"EUR"}, "date": {"2026-11-03"},
			"mode": {"advanced"}, "split_type": {"shares"}, "paid_by": {"12345"}, "value_" + aliceU.ID.String(): {"1"},
		}, false)
		Expect(text(resp.body)).To(ContainSubstring("Choose who paid"))
	})

	It("shows, edits and deletes an expense", func() {
		alice.addExpense(group, url.Values{"description": {"Pizza"}, "amount": {"20"}, "preset": {"you-paid-equal"}})
		path := firstExpensePath(alice)

		detail := text(bob.get(path).body)
		Expect(detail).To(ContainSubstring("alice paid €20.00"))
		Expect(detail).To(ContainSubstring("You owe €10.00"))

		// Bob sees the mirrored preset when editing.
		edit := bob.get(path + "/edit").body
		Expect(edit).To(ContainSubstring(`value="they-paid-equal" checked`))

		resp := bob.post(path, url.Values{
			"description": {"Pizza"}, "amount": {"30"}, "currency": {"EUR"}, "date": {"2026-11-03"},
			"mode": {"simple"}, "preset": {"they-paid-equal"},
		}, false)
		Expect(resp.StatusCode).To(Equal(http.StatusSeeOther))
		Expect(balances(alice)).To(ContainSubstring("bob owes you €15.00"))

		resp = bob.post(path+"/delete", nil, true)
		Expect(resp.StatusCode).To(Equal(http.StatusNoContent))
		Expect(resp.Header.Get("HX-Redirect")).To(Equal(group))
		Expect(balances(alice)).To(ContainSubstring("Everyone is settled up"))
		Expect(alice.get(path).StatusCode).To(Equal(http.StatusNotFound))
	})

	It("labels the split type on the expense page", func() {
		alice.addExpense(group, url.Values{"description": {"Pizza"}, "amount": {"20"}, "preset": {"you-paid-equal"}})
		Expect(text(alice.get(firstExpensePath(alice)).body)).To(ContainSubstring("Equally"))

		alice.addExpense(group, url.Values{"description": {"Taxi"}, "amount": {"20"}, "preset": {"you-owed-full"}})
		detail := text(alice.get(firstExpensePath(alice)).body)
		Expect(detail).To(ContainSubstring("Taxi"))
		Expect(detail).To(ContainSubstring("Full amount"))
		Expect(detail).NotTo(ContainSubstring("Equally"))
	})

	It("records partial and full payments", func() {
		alice.addExpense(group, url.Values{"amount": {"24"}, "preset": {"you-paid-equal"}})

		form := alice.get(group + "/settle?from=" + bobU.ID.String() + "&to=" + aliceU.ID.String() + "&currency=EUR&amount=12.00").body
		Expect(form).To(ContainSubstring(`value="12.00"`))

		settle := func(amount string) {
			resp := bob.post(group+"/settle", url.Values{
				"from": {bobU.ID.String()}, "to": {aliceU.ID.String()},
				"amount": {amount}, "currency": {"EUR"}, "date": {"2026-11-04"},
			}, false)
			Expect(resp.StatusCode).To(Equal(http.StatusSeeOther))
		}

		settle("5")
		Expect(balances(alice)).To(ContainSubstring("bob owes you €7.00"))

		settle("7")
		b := balances(alice)
		Expect(b).To(ContainSubstring("Everyone is settled up"))
		Expect(b).To(ContainSubstring("Total spending: €24.00"))

		Expect(text(alice.get("/").body)).To(ContainSubstring("You're all settled up."))
		Expect(text(alice.get(group).body)).To(ContainSubstring("bob paid you €7.00"))
	})

	It("shows balances above the expense list on the group page", func() {
		alice.addExpense(group, url.Values{"description": {"Pizza"}, "amount": {"20"}, "preset": {"you-paid-equal"}})

		t := text(alice.get(group).body)
		Expect(t).To(MatchRegexp(`Balances .*bob owes you €10\.00 Settle up Expenses .*Pizza`))
		Expect(t).NotTo(ContainSubstring("Record a payment"))
		Expect(alice.get(group + "/balances").StatusCode).To(Equal(http.StatusNotFound))
	})

	It("validates payments", func() {
		resp := bob.post(group+"/settle", url.Values{
			"from": {bobU.ID.String()}, "to": {bobU.ID.String()},
			"amount": {"5"}, "currency": {"EUR"}, "date": {"2026-11-04"},
		}, false)
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(text(resp.body)).To(ContainSubstring("Payer and recipient must be different people"))
	})

	It("keeps currencies separate", func() {
		alice.addExpense(group, url.Values{"amount": {"10"}, "preset": {"you-paid-equal"}})
		alice.addExpense(group, url.Values{"amount": {"1000"}, "currency": {"JPY"}, "preset": {"they-paid-equal"}})

		home := text(alice.get("/").body)
		Expect(home).To(ContainSubstring("You are owed €5.00"))
		Expect(home).To(ContainSubstring("You owe ¥500"))
	})

	It("lists activity across groups", func() {
		alice.addExpense(group, url.Values{"description": {"Groceries"}, "amount": {"10"}, "preset": {"you-paid-equal"}})
		other := alice.createGroup("Trip")
		alice.addExpense(other, url.Values{"description": {"Solo"}, "amount": {"5"}, "mode": {"advanced"}, "split_type": {"shares"},
			"paid_by": {aliceU.ID.String()}, "value_" + aliceU.ID.String(): {"1"}})

		t := text(alice.get("/activity").body)
		Expect(t).To(ContainSubstring("Groceries Flat · You paid €10.00 you lent €5.00"))
		Expect(t).To(ContainSubstring("Solo Trip · You paid €5.00"))

		Expect(text(bob.get("/activity").body)).NotTo(ContainSubstring("Solo"))
	})

	It("paginates the expense list", func(ctx SpecContext) {
		for i := range 35 {
			alice.addExpense(group, url.Values{"description": {fmt.Sprintf("Item %02d", i)}, "amount": {"1"}, "preset": {"you-paid-equal"}})
		}

		page := alice.get(group).body
		Expect(regexp.MustCompile(`Item \d\d`).FindAllString(page, -1)).To(HaveLen(30))

		next := regexp.MustCompile(`hx-get="([^"]+)"`).FindStringSubmatch(page)
		Expect(next).To(HaveLen(2))

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, env.ts.URL+htmlUnescape(next[1]), nil)
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("HX-Request", "true")
		more := alice.do(req)
		Expect(regexp.MustCompile(`Item \d\d`).FindAllString(more.body, -1)).To(HaveLen(5))
		Expect(more.body).NotTo(ContainSubstring("<html"))
		Expect(more.body).NotTo(ContainSubstring("hx-get"))
	})
})

func htmlUnescape(s string) string {
	return regexp.MustCompile(`&amp;`).ReplaceAllString(s, "&")
}
