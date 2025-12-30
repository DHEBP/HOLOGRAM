package main

import (
	"strings"
)

// UserFriendlyErrors maps technical error patterns to user-friendly messages
var UserFriendlyErrors = map[string]string{
	// Network/Connection errors
	"connection refused":           "Cannot connect to the node. Make sure derod is running.",
	"connection reset":             "Connection was reset. The node may have restarted.",
	"no such host":                 "Cannot find the node. Check your network settings.",
	"network is unreachable":       "Network is unreachable. Check your internet connection.",
	"i/o timeout":                  "Connection timed out. The node may be busy or unreachable.",
	"context deadline exceeded":    "Request timed out. Try again or check your connection.",
	"context canceled":             "Operation was cancelled.",
	"dial tcp":                     "Cannot connect to the node. Is derod running?",
	"EOF":                          "Connection closed unexpectedly. Try reconnecting.",
	
	// Simulator-specific errors
	"daemon connection lost":       "Simulator daemon connection lost. Please restart simulator mode.",
	"daemon crashed":               "Simulator daemon crashed. Please restart simulator mode.",
	"daemon endpoint is invalid":   "Simulator not properly configured. Please restart simulator mode.",
	"could not be built":           "Transaction build failed. Retrying with fresh nonce...",
	"simulator daemon not responding": "Simulator daemon not responding. Please restart simulator mode.",
	"websocket: close":             "Connection closed unexpectedly. Retrying...",
	"abnormal closure":             "Connection interrupted. The operation will be retried.",
	
	// Wallet errors
	"wallet not open":              "Please open a wallet first.",
	"wallet is not open":           "Please open a wallet first.",
	"wallet already open":          "A wallet is already open. Close it first.",
	"incorrect password":           "Incorrect wallet password.",
	"invalid password":             "Invalid wallet password.",
	"wallet file not found":        "Wallet file not found. Check the path.",
	"insufficient balance":         "Not enough DERO for this transaction.",
	"insufficient funds":           "Not enough DERO for this transaction.",
	
	// Transaction errors
	"tx rejected":                  "Transaction was rejected by the network.",
	"invalid transaction":          "Invalid transaction format.",
	"double spend":                 "Transaction rejected: possible double spend.",
	"mempool full":                 "Network mempool is full. Try again later.",
	"fee too low":                  "Transaction fee is too low.",
	
	// Smart contract errors
	"scid not found":               "Smart contract not found on the blockchain.",
	"invalid scid":                 "Invalid smart contract ID format.",
	"sc execution failed":          "Smart contract execution failed.",
	"gas limit exceeded":           "Transaction ran out of gas.",
	"panic":                        "Smart contract error occurred.",
	
	// XSWD errors
	"xswd not connected":           "Wallet not connected. Please connect first.",
	"xswd connection failed":       "Failed to connect to wallet service.",
	"permission denied":            "Permission denied by wallet.",
	"user rejected":                "Action was rejected by the user.",
	"request timeout":              "Wallet request timed out.",
	
	// Gnomon errors
	"gnomon not running":           "Gnomon indexer is not running. Start it in Settings.",
	"gnomon already running":       "Gnomon indexer is already running.",
	"index not found":              "Content not found in index. Try refreshing.",
	"durl not found":               "dURL not found. Check the address.",
	
	// File/Content errors
	"no html content":              "No displayable content found.",
	"no doc contracts":             "This INDEX has no associated documents.",
	"failed to fetch":              "Failed to load content. Check your connection.",
	"decode failed":                "Failed to decode content data.",
	"assembly failed":              "Failed to assemble content for display.",
	
	// Generic errors
	"not found":                    "The requested item was not found.",
	"unauthorized":                 "You don't have permission for this action.",
	"bad request":                  "Invalid request. Please check your input.",
	"internal error":               "An internal error occurred. Please try again.",
}

// FriendlyError converts a technical error message to a user-friendly one
func FriendlyError(err error) string {
	if err == nil {
		return ""
	}
	return FriendlyErrorString(err.Error())
}

// FriendlyErrorString converts a technical error string to a user-friendly one
func FriendlyErrorString(errMsg string) string {
	if errMsg == "" {
		return ""
	}
	
	lowerMsg := strings.ToLower(errMsg)
	
	// Check each pattern
	for pattern, friendly := range UserFriendlyErrors {
		if strings.Contains(lowerMsg, strings.ToLower(pattern)) {
			return friendly
		}
	}
	
	// Return original if no match found
	return errMsg
}

// ErrorResponse creates a standardized error response with both technical and friendly messages
func ErrorResponse(err error) map[string]interface{} {
	if err == nil {
		return map[string]interface{}{
			"success": true,
		}
	}
	
	return map[string]interface{}{
		"success":       false,
		"error":         FriendlyError(err),
		"technicalError": err.Error(),
	}
}

// ErrorResponseString creates an error response from a string
func ErrorResponseString(errMsg string) map[string]interface{} {
	return map[string]interface{}{
		"success":        false,
		"error":          FriendlyErrorString(errMsg),
		"technicalError": errMsg,
	}
}

// ErrorResponseWithData creates an error response with additional data fields
func ErrorResponseWithData(err error, data map[string]interface{}) map[string]interface{} {
	resp := ErrorResponse(err)
	for k, v := range data {
		resp[k] = v
	}
	return resp
}

