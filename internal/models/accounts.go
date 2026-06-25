package models

import (
	"bytes"
	"encoding/json"
	"fmt"
)

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

// AccountList is the parsed account collection for a single chain. Most chains
// return a plain JSON array of accounts, but bitcoin-like chains (btc, bch)
// return a {standalone, xpubs} object instead. UnmarshalJSON accepts either and
// flattens both into one slice, so callers can treat every chain uniformly.
type AccountList []Account

// bitcoinAccounts mirrors the {standalone, xpubs} shape the /blockchains/btc and
// /blockchains/bch accounts endpoints return. Each xpub entry carries its own
// derived addresses (or null when none are derived yet).
type bitcoinAccounts struct {
	Standalone []Account `json:"standalone"`
	Xpubs      []struct {
		Addresses []Account `json:"addresses"`
	} `json:"xpubs"`
}

// UnmarshalJSON decodes either the array form (EVM and most chains) or the
// bitcoin {standalone, xpubs} object form into a flat []Account.
func (a *AccountList) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		*a = nil
		return nil
	}

	switch trimmed[0] {
	case '[':
		var arr []Account
		if err := json.Unmarshal(data, &arr); err != nil {
			return err
		}
		*a = arr
	case '{':
		var btc bitcoinAccounts
		if err := json.Unmarshal(data, &btc); err != nil {
			return err
		}
		out := append([]Account{}, btc.Standalone...)
		for _, x := range btc.Xpubs {
			out = append(out, x.Addresses...)
		}
		*a = out
	default:
		return fmt.Errorf("unexpected accounts payload: %s", trimmed)
	}
	return nil
}

// AccountsResponse represents the API response for accounts
type AccountsResponse = APIResponse[AccountList]
