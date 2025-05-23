package models

type AssetEntry struct {
	Amount               string `json:"amount" validate:"required"`
	PercentageOfNetValue string `json:"percentage_of_net_value" validate:"required"`
	UsdValue             string `json:"usd_value" validate:"required"`
}

type LocationEntry struct {
	PercentageOfNetValue string `json:"percentage_of_net_value" validate:"required"`
	UsdValue             string `json:"usd_value" validate:"required"`
}

type BalanceResult struct {
	Assets      map[string]AssetEntry    `json:"assets" validate:"required"`
	Liabilities map[string]AssetEntry    `json:"liabilities" validate:"required"`
	Location    map[string]LocationEntry `json:"location" validate:"required"`
}

type BalanceResponse = APIResponse[BalanceResult]

type PeriodicResult struct {
	LastBalanceSave  int64               `json:"last_balance_save" validate:"required"`
	ConnectedNodes   map[string][]string `json:"connected_nodes" validate:"required"`
	LastDataUploadTS int64               `json:"last_data_upload_ts" validate:"required"`
}

type PeriodicResponse = APIResponse[PeriodicResult]

type ExchangeRateResponse = APIResponse[map[string]string]
