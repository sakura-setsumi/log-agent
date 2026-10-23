package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func redisTestSetup(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	t.Setenv("LOG_AGENT_CONFIG_DIR", t.TempDir())
	*configDirCache() = configDirState{}
	t.Cleanup(func() { *configDirCache() = configDirState{} })
	m := miniredis.RunT(t)
	port, _ := strconv.Atoi(m.Port())
	p := redisProfile{ID: "test", Name: "测试 Redis", Host: m.Host(), Port: port}
	if err := writeJSONFile(redisProfilesPath(), []redisProfile{p}); err != nil {
		t.Fatal(err)
	}
	c := redisClient(p, 0)
	t.Cleanup(func() { c.Close() })
	return m, c
}
func redisRequest(t *testing.T, method, action string, query url.Values, body any) *httptest.ResponseRecorder {
	t.Helper()
	if query == nil {
		query = url.Values{}
	}
	if !query.Has("id") {
		query.Set("id", "test")
	}
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	r := httptest.NewRequest(method, "/api/redis/"+action+"?"+query.Encode(), bytes.NewReader(data))
	r.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	(&server{}).handleRedis(w, r)
	return w
}
func TestRedisTypesAndExpiry(t *testing.T) {
	m, c := redisTestSetup(t)
	ctx := context.Background()
	c.Set(ctx, "biz:wx:token", "你好", time.Minute)
	c.HSet(ctx, "hash", "field", "value")
	c.RPush(ctx, "list", "a", "b")
	c.SAdd(ctx, "set", "a", "b")
	c.ZAdd(ctx, "zset", redis.Z{Score: 1.5, Member: "a"})
	c.XAdd(ctx, &redis.XAddArgs{Stream: "stream", Values: map[string]any{"message": "hello"}})
	for _, key := range []string{"biz:wx:token", "hash", "list", "set", "zset", "stream"} {
		t.Run(key, func(t *testing.T) {
			v := databaseDecode[redisValue](t, redisRequest(t, "GET", "value", url.Values{"key": {key}}, nil))
			if v.Version == "" {
				t.Fatal("missing conflict token")
			}
			if key == "stream" {
				if !v.Truncated {
					t.Fatal("stream should be read-only")
				}
				return
			}
			payload, _ := json.Marshal(v.Value)
			if err := writeRedisValue(ctx, c, redisWrite{Key: key, Type: v.Type, Value: payload, Version: v.Version, TTL: -2, Operation: "save"}); err != nil {
				t.Fatal(err)
			}
		})
	}
	if ttl := m.TTL("biz:wx:token"); ttl <= 0 || ttl > time.Minute {
		t.Fatalf("expiry not preserved: %s", ttl)
	}
	v, _ := readRedisValue(ctx, c, "biz:wx:token")
	resp := redisRequest(t, "POST", "write", nil, map[string]any{"key": "biz:wx:token", "version": v.Version, "operation": "ttl", "ttl": 10})
	if resp.Code != 200 {
		t.Fatal(resp.Body.String())
	}
	m.FastForward(11 * time.Second)
	if m.Exists("biz:wx:token") {
		t.Fatal("expiry was not applied")
	}
}
func TestRedisRejectsConflictsAndIncompleteWrites(t *testing.T) {
	_, c := redisTestSetup(t)
	ctx := context.Background()
	c.Set(ctx, "key", "first", 0)
	v, _ := readRedisValue(ctx, c, "key")
	c.Set(ctx, "key", "someone else", 0)
	for _, operation := range []string{"save", "delete", "ttl"} {
		err := writeRedisValue(ctx, c, redisWrite{Key: "key", Type: "string", Value: json.RawMessage(`"replacement"`), Version: v.Version, TTL: -1, Operation: operation})
		if err == nil {
			t.Fatalf("stale %s accepted", operation)
		}
	}
	if got := c.Get(ctx, "key").Val(); got != "someone else" {
		t.Fatal(got)
	}
	if err := writeRedisValue(ctx, c, redisWrite{Key: "key", Type: "string", Value: json.RawMessage(`"new"`), Create: true, TTL: -1, Operation: "save"}); err == nil {
		t.Fatal("overwrote existing key")
	}
	for i := 0; i < 501; i++ {
		c.RPush(ctx, "large", strconv.Itoa(i))
	}
	v, _ = readRedisValue(ctx, c, "large")
	if !v.Truncated {
		t.Fatal("large collection not marked")
	}
	if err := writeRedisValue(ctx, c, redisWrite{Key: "large", Type: "list", Value: json.RawMessage(`["new"]`), Version: v.Version, TTL: -2, Operation: "save"}); err == nil {
		t.Fatal("overwrote incomplete preview")
	}
	c.Set(ctx, "binary", string([]byte{0xff, 0xfe}), 0)
	v, _ = readRedisValue(ctx, c, "binary")
	if !v.Truncated {
		t.Fatal("binary value should be read-only")
	}
	for _, value := range []json.RawMessage{json.RawMessage(`[]`), json.RawMessage(`[1]`), json.RawMessage(`null`)} {
		if err := writeRedisValue(ctx, c, redisWrite{Key: "invalid", Type: "list", Value: value, Create: true, TTL: -1, Operation: "save"}); err == nil {
			t.Fatalf("accepted invalid collection: %s", value)
		}
	}
}
func TestRedisProfilesScanAndAccess(t *testing.T) {
	m, c := redisTestSetup(t)
	m.RequireAuth("secret")
	profiles, _ := readRedisProfiles()
	profiles[0].Password = "secret"
	writeJSONFile(redisProfilesPath(), profiles)
	c.Close()
	c = redisClient(profiles[0], 1)
	defer c.Close()
	c.Set(context.Background(), "biz:a", "one", 0)
	c.Set(context.Background(), "other", "two", 0)
	w := redisRequest(t, "GET", "profiles", nil, nil)
	if w.Code != 200 || strings.Contains(w.Body.String(), "secret") || !strings.Contains(w.Body.String(), `"hasPassword":true`) {
		t.Fatal(w.Body.String())
	}
	w = redisRequest(t, "POST", "profiles", nil, map[string]any{"id": "test", "name": "Renamed", "host": m.Host(), "port": profiles[0].Port})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	profiles, _ = readRedisProfiles()
	if profiles[0].Password != "secret" {
		t.Fatal("lost password")
	}
	w = redisRequest(t, "POST", "test", nil, map[string]any{"id": "test", "name": "Renamed", "host": m.Host(), "port": profiles[0].Port})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	data := databaseDecode[struct {
		Keys   []string `json:"keys"`
		Cursor string   `json:"cursor"`
	}](t, redisRequest(t, "GET", "scan", url.Values{"db": {"1"}, "pattern": {"biz:*"}, "cursor": {"0"}}, nil))
	if len(data.Keys) != 1 || data.Keys[0] != "biz:a" {
		t.Fatalf("wrong db/pattern: %+v", data)
	}
	for _, test := range []struct {
		method, action string
		query          url.Values
		code           int
	}{{"GET", "write", nil, 405}, {"GET", "scan", url.Values{"db": {"-1"}}, 400}, {"GET", "scan", url.Values{"cursor": {"bad"}}, 400}, {"GET", "value", url.Values{"key": {"missing"}}, 404}} {
		if w := redisRequest(t, test.method, test.action, test.query, nil); w.Code != test.code {
			t.Fatalf("%s status %d: %s", test.action, w.Code, w.Body.String())
		}
	}
	r := httptest.NewRequest(http.MethodGet, "/api/redis/profiles", nil)
	r.RemoteAddr = "192.0.2.1:1234"
	w = httptest.NewRecorder()
	(&server{}).handleRedis(w, r)
	if w.Code != 403 {
		t.Fatalf("remote caller status %d", w.Code)
	}
}
