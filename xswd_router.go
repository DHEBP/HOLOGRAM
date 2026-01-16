// Copyright 2025 HOLOGRAM Project. All rights reserved.
// XSWD Method Routing - Extracted from app.go for maintainability
// Session 87: Code restructuring

package main

import (
	"fmt"
	"log"
	"strings"
)

// XSWDRequest represents a parsed XSWD method call
type XSWDRequest struct {
	Method string                 `json:"method"`
	Params map[string]interface{} `json:"params"`
}

// XSWDResponse is a standardized response structure
type XSWDResponse map[string]interface{}

// Helper to create success response
func xswdSuccess(result interface{}) XSWDResponse {
	return XSWDResponse{
		"success": true,
		"result":  result,
	}
}

// Helper to create error response
func xswdError(msg string, technicalErr ...string) XSWDResponse {
	resp := XSWDResponse{
		"success": false,
		"error":   msg,
	}
	if len(technicalErr) > 0 {
		resp["technicalError"] = technicalErr[0]
	}
	return resp
}

// routeDaemonCall handles DERO.* daemon RPC methods
func (a *App) routeDaemonCall(method string, params map[string]interface{}) XSWDResponse {
	result, err := a.daemonClient.Call(method, params)
	if err != nil {
		log.Printf("[ERR] Daemon call failed: %v", err)
		return xswdError(FriendlyError(err), err.Error())
	}
	return xswdSuccess(result)
}

// routeEpochCall handles EPOCH-related methods
func (a *App) routeEpochCall(method string, params map[string]interface{}) XSWDResponse {
	switch method {
	case "AttemptEPOCH", "AttemptEPOCHWithAddr":
		hashes := 100 // default
		if h, ok := params["hashes"].(float64); ok && h > 0 {
			hashes = int(h)
		}

		// Handle developer address switching for AttemptEPOCHWithAddr
		// This allows app developers to receive EPOCH rewards when users interact with their apps
		if method == "AttemptEPOCHWithAddr" {
			if devAddress, ok := params["address"].(string); ok && devAddress != "" && len(devAddress) >= 60 {
				// Pause background worker (it hashes for default address)
				if a.devSupportWorker != nil {
					a.devSupportWorker.Pause(PauseReasonAppActive)
				}

				// Switch EPOCH to the app developer's address
				if a.epochHandler != nil {
					if err := a.epochHandler.SwitchToAddress(devAddress); err != nil {
						log.Printf("[EPOCH] Failed to switch to developer address: %v", err)
						// Continue anyway - hashes will go to current address
					}
				}
			}
		}

		// Compute hashes (rewards go to current EPOCH address - either app dev or default)
		epochResult := a.HandleEpochRequest(hashes, "")
		if epochResult["success"] == true {
			return xswdSuccess(map[string]interface{}{
				"epochHashes":        epochResult["hashes"],
				"epochSubmitted":     epochResult["submitted"],
				"epochDuration":      epochResult["duration_ms"],
				"epochHashPerSecond": epochResult["hash_per_sec"],
			})
		}
		return xswdError(fmt.Sprintf("%v", epochResult["error"]))

	case "GetMaxHashesEPOCH":
		stats := a.GetEpochStats()
		return xswdSuccess(map[string]interface{}{
			"maxHashes": stats["maxHashes"],
		})

	default:
		return xswdError(fmt.Sprintf("Unknown EPOCH method: %s", method))
	}
}

// routeTELACall handles TELA-specific methods
func (a *App) routeTELACall(method string, params map[string]interface{}) XSWDResponse {
	switch method {
	case "HandleTELALinks":
		telaLink := ""
		if link, ok := params["telaLink"].(string); ok {
			telaLink = link
		}
		log.Printf("[TELA] HandleTELALinks called with: %s", telaLink)
		return xswdSuccess(map[string]interface{}{
			"handled": true,
			"link":    telaLink,
		})

	default:
		return xswdError(fmt.Sprintf("Unknown TELA method: %s", method))
	}
}

// routeGnomonCall handles Gnomon.* indexer methods
func (a *App) routeGnomonCall(method string, params map[string]interface{}) XSWDResponse {
	if a.gnomonClient == nil || !a.gnomonClient.IsRunning() {
		return xswdError("Gnomon is not running. Start it in Settings.")
	}

	gnomonMethod := strings.TrimPrefix(method, "Gnomon.")
	log.Printf("[GNOMON] Handling method: %s", gnomonMethod)

	// Helper to get string param
	getStr := func(key string) string {
		if s, ok := params[key].(string); ok {
			return s
		}
		return ""
	}

	switch gnomonMethod {
	case "GetSCIDKeysByValue":
		keysStr, keysUint := a.gnomonClient.GetSCIDKeysByValue(getStr("scid"), getStr("value"))
		return xswdSuccess(map[string]interface{}{
			"keys_string": keysStr,
			"keys_uint64": keysUint,
		})

	case "GetSCIDValuesByKey":
		valsStr, valsUint := a.gnomonClient.GetSCIDValuesByKey(getStr("scid"), getStr("key"))
		return xswdSuccess(map[string]interface{}{
			"values_string": valsStr,
			"values_uint64": valsUint,
		})

	case "GetAllSCIDVariableDetails":
		vars := a.gnomonClient.GetAllSCIDVariableDetails(getStr("scid"))
		varMaps := make([]map[string]interface{}, 0, len(vars))
		for _, v := range vars {
			varMaps = append(varMaps, map[string]interface{}{
				"Key":   v.Key,
				"Value": v.Value,
			})
		}
		return xswdSuccess(map[string]interface{}{
			"allVariables": varMaps,
		})

	case "GetAllOwnersAndSCIDs":
		return xswdSuccess(a.gnomonClient.GetAllOwnersAndSCIDs())

	case "GetTxCount":
		status := a.gnomonClient.GetStatus()
		return xswdSuccess(map[string]interface{}{
			"indexed_height": status["indexed_height"],
			"chain_height":   status["chain_height"],
		})

	case "GetAllMiniblockDetails":
		return xswdSuccess(a.gnomonClient.GetStatus())

	case "GetStatus":
		return xswdSuccess(a.gnomonClient.GetStatus())

	case "GetLastIndexHeight":
		status := a.gnomonClient.GetStatus()
		return xswdSuccess(map[string]interface{}{
			"indexedheight": status["indexed_height"],
		})

	case "GetSCIDInteractionHeight":
		height := a.gnomonClient.LatestInteractionHeight(getStr("scid"))
		return xswdSuccess(map[string]interface{}{
			"height": height,
		})

	case "GetOwner":
		scid := getStr("scid")
		owners := a.gnomonClient.GetAllOwnersAndSCIDs()
		// Map is scid -> owner, so direct lookup
		owner := owners[scid]
		return xswdSuccess(map[string]interface{}{
			"owner": owner,
		})

	case "GetGetInfoDetails", "GetInfoDetails":
		return xswdSuccess(a.gnomonClient.GetStatus())

	case "SearchByKey":
		return xswdSuccess(a.gnomonClient.SearchByKey(getStr("key")))

	case "SearchByValue":
		return xswdSuccess(a.gnomonClient.SearchByValue(getStr("value")))

	case "ResolveName":
		scid, found := a.gnomonClient.ResolveName(getStr("name"))
		return xswdSuccess(map[string]interface{}{
			"scid":  scid,
			"found": found,
		})

	case "ResolveDURL":
		scid, found := a.gnomonClient.ResolveDURL(getStr("durl"))
		return xswdSuccess(map[string]interface{}{
			"scid":  scid,
			"found": found,
		})

	case "GetLiveSCIDKeysByValue":
		keysStr, keysUint := a.gnomonClient.GetSCIDKeysByValue(getStr("scid"), getStr("value"))
		return xswdSuccess(map[string]interface{}{
			"keys_string": keysStr,
			"keys_uint64": keysUint,
		})

	case "GetLiveSCIDValuesByKey":
		// Note: demo app uses "value" param for key in some cases
		key := getStr("key")
		if key == "" {
			key = getStr("value")
		}
		valsStr, valsUint := a.gnomonClient.GetSCIDValuesByKey(getStr("scid"), key)
		return xswdSuccess(map[string]interface{}{
			"values_string": valsStr,
			"values_uint64": valsUint,
		})

	// === Simple-Gnomon Feature: Tag System ===
	case "GetSCIDsByTag":
		store := InitSCIDTagStore()
		scids := store.GetSCIDsByTag(getStr("tag"))
		return xswdSuccess(map[string]interface{}{
			"scids": scids,
			"count": len(scids),
		})

	case "GetSCIDsByClass":
		store := InitSCIDTagStore()
		scids := store.GetSCIDsByClass(getStr("class"))
		return xswdSuccess(map[string]interface{}{
			"scids": scids,
			"count": len(scids),
		})

	case "GetAllTags":
		store := InitSCIDTagStore()
		return xswdSuccess(map[string]interface{}{
			"tags": store.GetAllTags(),
		})

	case "GetAllClasses":
		store := InitSCIDTagStore()
		return xswdSuccess(map[string]interface{}{
			"classes": store.GetAllClasses(),
		})

	case "GetSCIDMetadata":
		store := InitSCIDTagStore()
		meta := store.GetMetadata(getStr("scid"))
		if meta == nil {
			return xswdError("SCID not found in tag store")
		}
		return xswdSuccess(map[string]interface{}{
			"scid":    meta.SCID,
			"owner":   meta.Owner,
			"class":   meta.Class,
			"tags":    meta.Tags,
			"headers": meta.Headers,
		})

	case "GetTagStats":
		store := InitSCIDTagStore()
		return xswdSuccess(store.GetStats())

	default:
		log.Printf("[GNOMON] Unknown method: %s", gnomonMethod)
		return xswdError(fmt.Sprintf("Unknown Gnomon method: %s", gnomonMethod))
	}
}

// isEpochMethod checks if a method is an EPOCH method
func isEpochMethod(method string) bool {
	return method == "AttemptEPOCH" || method == "AttemptEPOCHWithAddr" || method == "GetMaxHashesEPOCH"
}

// isTELAMethod checks if a method is a TELA method
func isTELAMethod(method string) bool {
	return method == "HandleTELALinks"
}

