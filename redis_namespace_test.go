package main

import (
	"context"
	"net/url"
	"testing"
)

func TestRedisNamespaceActions(t *testing.T) {
	_, c := redisTestSetup(t)
	ctx := context.Background()
	for _, key := range []string{"agent:a", "agent:child:b", "agent", "agent2:a", "a[1]:x", "a1:x"} {
		c.Set(ctx, key, "value", 0)
	}
	preview := databaseDecode[struct {
		Count int    `json:"count"`
		Token string `json:"token"`
	}](t, redisRequest(t, "POST", "namespace", nil, map[string]any{"prefix": "agent:", "operation": "preview"}))
	if preview.Count != 2 || preview.Token == "" {
		t.Fatalf("invalid preview: %+v", preview)
	}
	c.Set(ctx, "agent:new", "value", 0)
	response := redisRequest(t, "POST", "namespace", nil, map[string]any{"prefix": "agent:", "operation": "delete", "token": preview.Token})
	if response.Code == 200 || c.Exists(ctx, "agent:a", "agent:child:b", "agent:new").Val() != 3 {
		t.Fatal("stale preview deleted keys")
	}
	preview = databaseDecode[struct {
		Count int    `json:"count"`
		Token string `json:"token"`
	}](t, redisRequest(t, "POST", "namespace", nil, map[string]any{"prefix": "agent:", "operation": "preview"}))
	result := databaseDecode[struct {
		Deleted int `json:"deleted"`
	}](t, redisRequest(t, "POST", "namespace", nil, map[string]any{"prefix": "agent:", "operation": "delete", "token": preview.Token}))
	if result.Deleted != 3 || c.Exists(ctx, "agent", "agent2:a").Val() != 2 {
		t.Fatal("namespace deletion crossed prefix boundary")
	}
	for _, prefix := range []string{"", "agent", "*"} {
		if w := redisRequest(t, "POST", "namespace", nil, map[string]any{"prefix": prefix, "operation": "preview"}); w.Code == 200 {
			t.Fatalf("accepted unsafe prefix %q", prefix)
		}
	}
	page := databaseDecode[struct {
		Keys []string `json:"keys"`
	}](t, redisRequest(t, "GET", "scan", url.Values{"prefix": {"a[1]:"}, "cursor": {"0"}}, nil))
	if len(page.Keys) != 1 || page.Keys[0] != "a[1]:x" {
		t.Fatalf("glob metacharacters were not escaped: %+v", page)
	}
	if w := redisRequest(t, "POST", "namespace", nil, map[string]any{"prefix": "agent2:", "operation": "delete"}); w.Code == 200 || c.Exists(ctx, "agent2:a").Val() != 1 {
		t.Fatal("delete without preview accepted")
	}
	if w := redisRequest(t, "GET", "namespace", nil, nil); w.Code != 405 {
		t.Fatal("namespace write accepted GET")
	}
}
