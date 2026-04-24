package handler

import (
	"context"
	"encoding/json"
)

// siteSettingsJSON is the API representation of site-wide settings.
// サイト全体設定の API レスポンス表現。
type siteSettingsJSON struct {
	RobotsNoIndex bool `json:"robots_noindex"`
	RobotsNoAI    bool `json:"robots_noai"`
}

// rpcSiteGetSettings handles site.get_settings.
// サイト全体設定を返す。
func (h *Handler) rpcSiteGetSettings(ctx context.Context, _ json.RawMessage) (any, *rpcErr) {
	noIndex, _ := h.store.GetSetting(ctx, SettingRobotsNoIndex)
	noAI, _ := h.store.GetSetting(ctx, SettingRobotsNoAI)
	return &siteSettingsJSON{
		RobotsNoIndex: noIndex == "true",
		RobotsNoAI:    noAI == "true",
	}, nil
}

// rpcSiteUpdateSettings handles site.update_settings.
// サイト全体設定を更新する。
func (h *Handler) rpcSiteUpdateSettings(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	type updateParams struct {
		RobotsNoIndex *bool `json:"robots_noindex,omitempty"`
		RobotsNoAI    *bool `json:"robots_noai,omitempty"`
	}
	req, rErr := parseParams[updateParams](params)
	if rErr != nil {
		return nil, rErr
	}

	if req.RobotsNoIndex != nil {
		val := "false"
		if *req.RobotsNoIndex {
			val = "true"
		}
		h.store.SetSetting(ctx, SettingRobotsNoIndex, val)
	}
	if req.RobotsNoAI != nil {
		val := "false"
		if *req.RobotsNoAI {
			val = "true"
		}
		h.store.SetSetting(ctx, SettingRobotsNoAI, val)
	}

	return statusOK, nil
}
