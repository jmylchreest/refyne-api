package migrations

func init() {
	Register(Migration{
		Timestamp:   "20260118-105653",
		Description: "Add subscription period tracking to user_balances",
		Up: []string{
			// Add period_start and period_end columns to track the current billing period
			// These are updated when we receive Clerk subscription webhooks
			`ALTER TABLE user_balances ADD COLUMN period_start TEXT`,
			`ALTER TABLE user_balances ADD COLUMN period_end TEXT`,
		},
	})
}
