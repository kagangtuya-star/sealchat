package api

import (
	"testing"

	"sealchat/model"
)

func TestBuildAdminBotOneBotConfigNormalizesTransportType(t *testing.T) {
	cfg := buildAdminBotOneBotConfig("bot-1", &adminBotOneBotConfigInput{
		Enabled:             true,
		TransportType:       " http ",
		HTTPPathSuffix:      "OlivOSMsgApi/qq/onebot/default/",
		HTTPPostPathSuffix:  " http://127.0.0.1:55001/OlivOSMsgApi/qq/onebot/default/ ",
		ReconnectIntervalMs: 0,
	})
	if cfg == nil {
		t.Fatal("config should not be nil")
	}
	if cfg.TransportType != model.OneBotTransportHTTP {
		t.Fatalf("transport type = %q, want %q", cfg.TransportType, model.OneBotTransportHTTP)
	}
	if cfg.ReconnectIntervalMs != model.DefaultOneBotReconnectIntervalMs {
		t.Fatalf("reconnect interval = %d, want %d", cfg.ReconnectIntervalMs, model.DefaultOneBotReconnectIntervalMs)
	}
	if cfg.HTTPPathSuffix != "/OlivOSMsgApi/qq/onebot/default" {
		t.Fatalf("http path suffix = %q", cfg.HTTPPathSuffix)
	}
	if cfg.HTTPPostPathSuffix != "http://127.0.0.1:55001/OlivOSMsgApi/qq/onebot/default" {
		t.Fatalf("http post path suffix = %q", cfg.HTTPPostPathSuffix)
	}

	defaultCfg := buildAdminBotOneBotConfig("bot-2", &adminBotOneBotConfigInput{
		Enabled:       true,
		TransportType: "not-real",
	})
	if defaultCfg.TransportType != model.OneBotTransportForwardWS {
		t.Fatalf("default transport type = %q, want %q", defaultCfg.TransportType, model.OneBotTransportForwardWS)
	}
	if defaultCfg.HTTPPathSuffix != model.DefaultOneBotHTTPPathSuffix {
		t.Fatalf("default http path suffix = %q, want %q", defaultCfg.HTTPPathSuffix, model.DefaultOneBotHTTPPathSuffix)
	}
	if defaultCfg.HTTPPostPathSuffix != "" {
		t.Fatalf("default http post path suffix = %q, want empty", defaultCfg.HTTPPostPathSuffix)
	}
}
