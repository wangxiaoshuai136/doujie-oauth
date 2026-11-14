package main

import (
	"encoding/json"
	"net/http"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

var pluginVersion = "0.2.4"

type rpcAuthStart struct {
	pluginapi.AuthLoginStartRequest
	CallbackID string `json:"host_callback_id"`
}

type rpcAuthPoll struct {
	pluginapi.AuthLoginPollRequest
	CallbackID string `json:"host_callback_id"`
}

type rpcModel struct {
	pluginapi.AuthModelRequest
	CallbackID string `json:"host_callback_id"`
}

type rpcQuotaFetch struct {
	pluginapi.QuotaFetchRequest
	CallbackID string `json:"host_callback_id"`
}

type rpcQuotaReset struct {
	pluginapi.QuotaResetRequest
	CallbackID string `json:"host_callback_id"`
}

type rpcExecution struct {
	pluginapi.ExecutorRequest
	CallbackID string `json:"host_callback_id"`
	StreamID   string `json:"stream_id"`
}

func unmarshal(raw []byte, out any) error {
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	if json.Unmarshal(raw, out) != nil {
		return pluginabi.NewError("invalid_request", "invalid plugin request", 400)
	}
	return nil
}

func registration() any {
	return map[string]any{
		"schema_version": pluginabi.SchemaVersion,
		"metadata": pluginapi.Metadata{
			Name:             "Doujie OAuth",
			Version:          pluginVersion,
			Author:           "wangxiaoshuai136",
			GitHubRepository: "https://github.com/wangxiaoshuai136/doujie-oauth",
			Logo:             resourceAssetURL("logo.png"),
			ConfigFields: []pluginapi.ConfigField{
				{Name: "max_concurrency", Type: pluginapi.ConfigFieldTypeInteger, Description: "Maximum simultaneous requests per credential (default 3)."},
				{Name: "bot_id", Type: pluginapi.ConfigFieldTypeString, Description: "Bot id used for every chat request. Leave the default to use the per-site bot automatically."},
				{Name: "deep_think", Type: pluginapi.ConfigFieldTypeBoolean, Description: "Request Doubao deep thinking for every call (default false; thinking models enable it automatically)."},
				{Name: "signer", Type: pluginapi.ConfigFieldTypeEnum, EnumValues: []string{"none", "external"}, Description: "Request signing mode: direct HTTP (none) or an external signing service (external)."},
				{Name: "signer_url", Type: pluginapi.ConfigFieldTypeString, Description: "Absolute HTTP(S) URL of the external signer; required when signer is external."},
				{Name: "public_base_url", Type: pluginapi.ConfigFieldTypeString, Description: "Optional absolute CPA base URL used for the static login result pages."},
				{Name: "endpoint", Type: pluginapi.ConfigFieldTypeEnum, EnumValues: []string{"samantha", "chat"}, Description: "Upstream endpoint family: samantha (default, direct HTTP) or chat (desktop completion, usually needs an external signer)."},
				{Name: "app_version", Type: pluginapi.ConfigFieldTypeString, Description: "Doubao desktop client version sent in request parameters (default 2.1.7)."},
			},
		},
		"capabilities": map[string]any{
			"auth_provider":           true,
			"quota_provider":          true,
			"model_provider":          true,
			"executor":                true,
			"executor_model_scope":    "oauth",
			"executor_input_formats":  []string{"claude", "openai", "openai-response"},
			"executor_output_formats": []string{"claude", "openai", "openai-response"},
			"management_api":          true,
		},
	}
}

func handleMethod(method string, raw []byte) (any, error) {
	switch method {
	case "plugin.register", "plugin.reconfigure":
		var req struct {
			ConfigYAML []byte `json:"config_yaml"`
		}
		if err := unmarshal(raw, &req); err != nil {
			return nil, err
		}
		if err := configure(req.ConfigYAML); err != nil {
			return nil, pluginabi.NewError("invalid_config", err.Error(), 400)
		}
		return registration(), nil
	case "plugin.quiesce", "plugin.shutdown":
		closeAllFlows()
		return map[string]any{}, nil
	case "auth.identifier":
		return map[string]any{
			"identifier":           entryID,
			"identifiers":          []string{entryID, internationalEntryID},
			"credential_providers": []string{credentialProvider},
			"entries": []map[string]any{
				{"identifier": entryID, "name": "豆姐（国内版）", "logo": resourceAssetURL("doubao.png")},
				{"identifier": internationalEntryID, "name": "豆姐国际版（Dola）", "logo": resourceAssetURL("dola.png")},
			},
		}, nil
	case "executor.identifier", "model.identifier":
		return map[string]string{"identifier": credentialProvider}, nil
	case "auth.parse":
		var req pluginapi.AuthParseRequest
		if err := unmarshal(raw, &req); err != nil {
			return nil, err
		}
		return parseAuth(req), nil
	case "auth.login.start":
		var req rpcAuthStart
		if err := unmarshal(raw, &req); err != nil {
			return nil, err
		}
		value, err := startLogin(req.AuthLoginStartRequest, cpaSender{callbackID: req.CallbackID})
		if err != nil {
			return nil, loginError(err)
		}
		return value, nil
	case "auth.login.poll":
		var req rpcAuthPoll
		if err := unmarshal(raw, &req); err != nil {
			return nil, err
		}
		value, err := pollLogin(req.AuthLoginPollRequest, cpaSender{callbackID: req.CallbackID})
		if err != nil {
			return nil, pluginabi.NewError("auth_error", err.Error(), 400)
		}
		return value, nil
	case "auth.refresh":
		return nil, pluginabi.NewError("reauth_required", "Sessions cannot be refreshed; sign in again", 400)
	case "management.register":
		return map[string]any{
			"routes": []map[string]any{
				{"Method": "GET", "Path": "/v0/management/doujie-oauth/video/status", "Description": "Video task leases and quota cooldowns."},
				{"Method": "POST", "Path": "/v0/management/doujie-oauth/video/cooldowns/clear", "Description": "Clear one or all video quota cooldowns."},
			},
			"resources": []map[string]any{
				{"path": "/logo.png", "menu": "", "description": "Plugin logo."},
				{"path": "/imported.html", "menu": "", "description": "Desktop import result page."},
				{"path": "/account.html", "menu": "", "description": "Browser-based account sign-in page."},
				{"path": "/doubao.png", "menu": "", "description": "Domestic sign-in logo."},
				{"path": "/dola.png", "menu": "", "description": "International sign-in logo."},
				{"path": "/doujie.json", "menu": "", "description": "Public plugin declaration."},
			},
		}, nil
	case "management.handle":
		var req pluginapi.ManagementRequest
		if err := unmarshal(raw, &req); err != nil {
			return nil, err
		}
		return handleManagementRequest(req)
	case "model.static":
		return pluginapi.ModelResponse{Provider: credentialProvider, Models: staticModels()}, nil
	case "model.for_auth":
		var req rpcModel
		if err := unmarshal(raw, &req); err != nil {
			return nil, err
		}
		var persisted struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(req.StorageJSON, &persisted) != nil || persisted.Type != credentialProvider {
			return nil, pluginabi.NewError("invalid_auth", "invalid credential", 401)
		}
		return pluginapi.ModelResponse{Provider: credentialProvider, Models: staticModels()}, nil
	case "executor.execute":
		var req rpcExecution
		if err := unmarshal(raw, &req); err != nil {
			return nil, err
		}
		return execute(req.ExecutorRequest, cpaSender{callbackID: req.CallbackID})
	case "executor.execute_stream":
		var req rpcExecution
		if err := unmarshal(raw, &req); err != nil {
			return nil, err
		}
		return executeStream(req.ExecutorRequest, req.CallbackID, req.StreamID, cpaSender{callbackID: req.CallbackID})
	case "quota.identifier":
		return map[string]string{"identifier": credentialProvider}, nil
	case "quota.describe":
		return quotaDescription(), nil
	case "quota.fetch":
		var req rpcQuotaFetch
		if err := unmarshal(raw, &req); err != nil {
			return nil, err
		}
		return quotaFetch(req.QuotaFetchRequest, cpaSender{callbackID: req.CallbackID})
	case "quota.reset":
		var req rpcQuotaReset
		if err := unmarshal(raw, &req); err != nil {
			return nil, err
		}
		resp, err := quotaReset(req.QuotaResetRequest, cpaSender{callbackID: req.CallbackID})
		if err != nil {
			return nil, pluginabi.NewError("reset_failed", err.Error(), 400)
		}
		return resp, nil
	case "executor.count_tokens":
		return nil, pluginabi.NewError("not_supported", "Doubao does not provide a verified token-count endpoint", http.StatusNotImplemented)
	case "executor.http_request":
		return nil, pluginabi.NewError("not_supported", "unsupported plugin method", http.StatusNotImplemented)
	default:
		return nil, pluginabi.NewError("unknown_method", "unknown plugin method", http.StatusNotImplemented)
	}
}
