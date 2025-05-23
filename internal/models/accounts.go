package models

type Account struct {
	Address string   `json:"address" validate:"required"`
	Label   string   `json:"label"`
	Tags    []string `json:"tags,omitempty"`
}

type ChainAccount struct {
	ChainID  string
	EvmChain string
	Address  string
	Label    *string
	Tags     []string
}

// AccountsResponse represents the API response for accounts
type AccountsResponse = APIResponse[[]Account]
