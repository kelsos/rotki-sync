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

// EvmTransactionAccount represents an account on an EVM chain
type EvmTransactionAccount struct {
	Address  string `json:"address" validate:"required"`
	EvmChain string `json:"evm_chain" validate:"required"`
}

// EvmTransactionsRequest represents a request to fetch EVM transactions
type EvmTransactionsRequest struct {
	Accounts []EvmTransactionAccount `json:"accounts" validate:"required"`
}

// EvmTransactionsResponse represents the API response for EVM transactions
type EvmTransactionsResponse struct {
	Result  bool   `json:"result" validate:"required"`
	Message string `json:"message,omitempty"`
}

// EvmTransactionDecodeRequest represents a request to decode EVM transactions
type EvmTransactionDecodeRequest struct {
	Chains []string `json:"chains" validate:"required"`
}

// DecodedTxNumber represents the number of decoded transactions per chain
type DecodedTxNumber map[string]int

// EvmTransactionDecodeResult represents the result of decoding EVM transactions
type EvmTransactionDecodeResult struct {
	DecodedTxNumber DecodedTxNumber `json:"decoded_tx_number" validate:"required"`
}

// EvmTransactionDecodeResponse represents the API response for decoding EVM transactions
type EvmTransactionDecodeResponse = APIResponse[EvmTransactionDecodeResult]

type QueryType string

const (
	EthWithdrawalsQuery   QueryType = "eth_withdrawals"
	BlockProductionsQuery QueryType = "block_productions"
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
