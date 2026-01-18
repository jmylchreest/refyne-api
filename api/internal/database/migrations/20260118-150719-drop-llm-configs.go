package migrations

func init() {
	Register(Migration{
		Timestamp:   "20260118-150719",
		Description: "Drop legacy llm_configs table (replaced by user_service_keys and user_fallback_chain)",
		Up: []string{
			`DROP TABLE IF EXISTS llm_configs`,
		},
	})
}
