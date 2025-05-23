package models

type UserStatus string

const (
	StatusLoggedIn  UserStatus = "loggedin"
	StatusLoggedOut UserStatus = "loggedout"
)

type UsersMap map[string]UserStatus

type UserResponse = APIResponse[UsersMap]

type UserActionResponse = APIResponse[bool]

type Settings struct {
	HavePremium                       bool     `json:"have_premium"`
	Version                           int      `json:"version"`
	LastWriteTS                       int64    `json:"last_write_ts"`
	PremiumShouldSync                 bool     `json:"premium_should_sync"`
	IncludeCrypto2Crypto              bool     `json:"include_crypto2crypto"`
	UIFloatingPrecision               int      `json:"ui_floating_precision"`
	TaxfreeAfterPeriod                int64    `json:"taxfree_after_period"`
	BalanceSaveFrequency              int      `json:"balance_save_frequency"`
	IncludeGasCosts                   bool     `json:"include_gas_costs"`
	KsmRPCEndpoint                    string   `json:"ksm_rpc_endpoint"`
	DotRPCEndpoint                    string   `json:"dot_rpc_endpoint"`
	BeaconRPCEndpoint                 string   `json:"beacon_rpc_endpoint"`
	MainCurrency                      string   `json:"main_currency"`
	DateDisplayFormat                 string   `json:"date_display_format"`
	SubmitUsageAnalytics              bool     `json:"submit_usage_analytics"`
	ActiveModules                     []string `json:"active_modules"`
	FrontendSettings                  string   `json:"frontend_settings"`
	BtcDerivationGapLimit             int      `json:"btc_derivation_gap_limit"`
	CalculatePastCostBasis            bool     `json:"calculate_past_cost_basis"`
	DisplayDateInLocaltime            bool     `json:"display_date_in_localtime"`
	CurrentPriceOracles               []string `json:"current_price_oracles"`
	HistoricalPriceOracles            []string `json:"historical_price_oracles"`
	PnlCsvWithFormulas                bool     `json:"pnl_csv_with_formulas"`
	PnlCsvHaveSummary                 bool     `json:"pnl_csv_have_summary"`
	SsfGraphMultiplier                int      `json:"ssf_graph_multiplier"`
	LastDataMigration                 int      `json:"last_data_migration"`
	NonSyncingExchanges               []string `json:"non_syncing_exchanges"`
	EvmchainsToSkipDetection          []string `json:"evmchains_to_skip_detection"`
	CostBasisMethod                   string   `json:"cost_basis_method"`
	TreatEth2AsEth                    bool     `json:"treat_eth2_as_eth"`
	Eth2TaxableAfterWithdrawalEnabled bool     `json:"eth_staking_taxable_after_withdrawal_enabled"`
	AddressNamePriority               []string `json:"address_name_priority"`
	IncludeFeesInCostBasis            bool     `json:"include_fees_in_cost_basis"`
	InferZeroTimedBalances            bool     `json:"infer_zero_timed_balances"`
	QueryRetryLimit                   int      `json:"query_retry_limit"`
	ConnectTimeout                    int      `json:"connect_timeout"`
	ReadTimeout                       int      `json:"read_timeout"`
	OraclePenaltyThresholdCount       int      `json:"oracle_penalty_threshold_count"`
	OraclePenaltyDuration             int      `json:"oracle_penalty_duration"`
	AutoDeleteCalendarEntries         bool     `json:"auto_delete_calendar_entries"`
	AutoCreateCalendarReminders       bool     `json:"auto_create_calendar_reminders"`
	AskUserUponSizeDiscrepancy        bool     `json:"ask_user_upon_size_discrepancy"`
	AutoDetectTokens                  bool     `json:"auto_detect_tokens"`
	CsvExportDelimiter                string   `json:"csv_export_delimiter"`
	LastBalanceSave                   int64    `json:"last_balance_save"`
	LastDataUploadTS                  int64    `json:"last_data_upload_ts"`
}

type Exchange struct {
	Name      string `json:"name"`
	Location  string `json:"location"`
	APIKey    string `json:"api_key,omitempty"`
	APISecret string `json:"api_secret,omitempty"`
	Enabled   bool   `json:"enabled"`
}

type UserLogin struct {
	Username    string     `json:"username"`
	Status      UserStatus `json:"status"`
	LastLoginTS int64      `json:"last_login_ts,omitempty"`
	Premium     bool       `json:"premium"`
	PremiumSync bool       `json:"premium_sync"`
	Settings    Settings   `json:"settings,omitempty"`
	Exchanges   []Exchange `json:"exchanges,omitempty"`
}

type UserLoginResponse = APIResponse[UserLogin]
