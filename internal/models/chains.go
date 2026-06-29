package models

// Chain type constants
const (
	ChainTypeEvm       = "evm"
	ChainTypeEvmLike   = "evmlike"
	ChainTypeBitcoin   = "bitcoin"
	ChainTypeSolana    = "solana"
	ChainTypeSubstrate = "substrate"
)

// Blockchain represents a blockchain supported by the API
type Blockchain struct {
	ID           string `json:"id" validate:"required"`
	Name         string `json:"name" validate:"required"`
	Type         string `json:"type" validate:"required"`
	NativeToken  string `json:"native_token" validate:"required"`
	Image        string `json:"image" validate:"required"`
	EvmChainName string `json:"evm_chain_name,omitempty"`
}

// BlockchainResponse represents the API response for supported blockchains
type BlockchainResponse = APIResponse[[]Blockchain]

// TransactionDecodeResult represents the result of decoding transactions via
// the unified /blockchains/transactions/decode endpoint. Each request decodes a
// single chain, so decoded_tx_number is a plain count (not the legacy
// per-chain map that the removed /blockchains/evm/transactions/decode returned).
type TransactionDecodeResult struct {
	DecodedTxNumber int `json:"decoded_tx_number"`
}

type QueryType string

const (
	EthWithdrawalsQuery   QueryType = "eth_withdrawals"
	BlockProductionsQuery QueryType = "block_productions"
	GnosisPayQuery        QueryType = "gnosis_pay"
	MoneriumQuery         QueryType = "monerium"
)

type EventsQueryPayload struct {
	QueryType QueryType `json:"query_type" validate:"required"`
}

type EventsQueryResponse = APIResponse[bool]

// TransactionAccount represents an account for generic transaction fetching
type TransactionAccount struct {
	Address    string `json:"address"`
	Blockchain string `json:"blockchain"`
}

// TransactionsRequest represents a request to fetch transactions via generic endpoint
type TransactionsRequest struct {
	Accounts []TransactionAccount `json:"accounts"`
}

// TransactionDecodeRequest represents a request to decode transactions via generic endpoint
type TransactionDecodeRequest struct {
	Chain string `json:"chain"`
}

// TokenDetectRequest represents a request to detect tokens on a chain
type TokenDetectRequest struct {
	Addresses []string `json:"addresses"`
	OnlyCache bool     `json:"only_cache,omitempty"`
}

// TokenDetectAddressInfo holds cached token detection info for a single address
type TokenDetectAddressInfo struct {
	Tokens              []string `json:"tokens"`
	LastUpdateTimestamp int64    `json:"last_update_timestamp"`
}

// TokenDetectResponse maps an address to its cached token detection info
type TokenDetectResponse map[string]TokenDetectAddressInfo
