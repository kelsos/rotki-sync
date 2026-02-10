package models

type Account struct {
	Address string   `json:"address" validate:"required"`
	Label   string   `json:"label"`
	Tags    []string `json:"tags,omitempty"`
}

type ChainAccount struct {
	ChainID    string
	EvmChain   string
	Blockchain string // Chain ID used in generic /blockchains/transactions
	ChainType  string // "evm", "evmlike", "bitcoin", "solana", "substrate"
	Address    string
	Label      *string
	Tags       []string
}

// AccountsResponse represents the API response for accounts
type AccountsResponse = APIResponse[[]Account]
