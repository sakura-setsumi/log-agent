package main

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/redis/go-redis/v9"
)

// Credentials are stored beside the other local connection profiles and never
// returned by the listing endpoint. An omitted password preserves the old one.
type redisProfile struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	User        string `json:"user"`
	Password    string `json:"password,omitempty"`
	HasPassword bool   `json:"hasPassword,omitempty"`
	TLS         bool   `json:"tls"`
}

var redisProfilesMu sync.Mutex

func redisProfilesPath() string { return filepath.Join(configDir(), "redis.json") }
func readRedisProfiles() ([]redisProfile, error) {
	profiles := []redisProfile{}
	err := readJSONFile(redisProfilesPath(), &profiles)
	if errors.Is(err, errNoConfigFile) {
		err = nil
	}
	if profiles == nil {
		profiles = []redisProfile{}
	}
	return profiles, err
}
func redisConfig(p redisProfile) (redisProfile, error) {
	p.Name, p.Host = strings.TrimSpace(p.Name), strings.TrimSpace(p.Host)
	if p.Port == 0 {
		p.Port = 6379
	}
	if p.Name == "" || p.Host == "" || p.Port < 1 || p.Port > 65535 {
		return p, errors.New("name, host and a valid port are required")
	}
	return p, nil
}
func redisClient(p redisProfile, db int) *redis.Client {
	opts := &redis.Options{Addr: net.JoinHostPort(p.Host, strconv.Itoa(p.Port)), Username: p.User, Password: p.Password, DB: db, Protocol: 2, DialTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, ContextTimeoutEnabled: true, MaxRetries: -1, PoolSize: 2}
	if p.TLS {
		opts.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12, ServerName: p.Host}
	}
	return redis.NewClient(opts)
}
func redisAPIError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": "redis_error", "detail": err.Error()})
}

func handleRedisProfiles(w http.ResponseWriter, r *http.Request) {
	redisProfilesMu.Lock()
	defer redisProfilesMu.Unlock()
	profiles, err := readRedisProfiles()
	if err != nil {
		redisAPIError(w, 500, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		for i := range profiles {
			profiles[i].HasPassword = profiles[i].Password != ""
			profiles[i].Password = ""
		}
		writeJSON(w, 200, profiles)
		return
	case http.MethodPost:
		var req struct {
			redisProfile
			Password *string `json:"password"`
		}
		if err = decodeDatabaseRequest(w, r, &req); err != nil {
			redisAPIError(w, 400, err)
			return
		}
		p := req.redisProfile
		index := -1
		for i, old := range profiles {
			if old.ID == p.ID {
				index = i
				p.Password = old.Password
				break
			}
		}
		if p.ID != "" && index < 0 {
			redisAPIError(w, 404, errors.New("connection not found"))
			return
		}
		if req.Password != nil {
			p.Password = *req.Password
		}
		p, err = redisConfig(p)
		if err != nil {
			redisAPIError(w, 400, err)
			return
		}
		if p.ID == "" {
			p.ID = newTermID()
		}
		p.HasPassword = false
		if index < 0 {
			profiles = append(profiles, p)
		} else {
			profiles[index] = p
		}
		if err = os.MkdirAll(configDir(), 0700); err == nil {
			err = writeJSONFile(redisProfilesPath(), profiles)
		}
		if err != nil {
			redisAPIError(w, 500, err)
			return
		}
		p.HasPassword = p.Password != ""
		p.Password = ""
		writeJSON(w, 200, p)
	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		found := false
		kept := []redisProfile{}
		for _, p := range profiles {
			if p.ID == id {
				found = true
			} else {
				kept = append(kept, p)
			}
		}
		if !found {
			redisAPIError(w, 404, errors.New("connection not found"))
			return
		}
		if err = writeJSONFile(redisProfilesPath(), kept); err != nil {
			redisAPIError(w, 500, err)
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	default:
		w.WriteHeader(405)
	}
}

const redisPreviewItems = 500
const redisPreviewBytes = 2 << 20

type redisValue struct {
	Type      string `json:"type"`
	Value     any    `json:"value"`
	TTL       int64  `json:"ttl"`
	Size      int64  `json:"size"`
	Truncated bool   `json:"truncated"`
	Version   string `json:"version"`
}

// Reads are bounded; incomplete or non-UTF-8 previews can never be saved.
func readRedisValue(ctx context.Context, c redis.Cmdable, key string) (redisValue, error) {
	v := redisValue{}
	var err error
	v.Type, err = c.Type(ctx, key).Result()
	if err != nil {
		return v, err
	}
	switch v.Type {
	case "none":
		return v, redis.Nil
	case "string":
		v.Size, err = c.StrLen(ctx, key).Result()
		if err == nil {
			v.Value, err = c.GetRange(ctx, key, 0, redisPreviewBytes).Result()
		}
		v.Truncated = v.Size > redisPreviewBytes
	case "hash":
		v.Size, err = c.HLen(ctx, key).Result()
		if err == nil {
			var fields []string
			var cursor uint64
			fields, cursor, err = c.HScan(ctx, key, 0, "*", redisPreviewItems).Result()
			data := map[string]string{}
			for i := 0; i+1 < len(fields); i += 2 {
				data[fields[i]] = fields[i+1]
			}
			v.Value = data
			v.Truncated = cursor != 0 || int64(len(data)) < v.Size || v.Size > redisPreviewItems
		}
	case "list":
		v.Size, err = c.LLen(ctx, key).Result()
		if err == nil {
			v.Value, err = c.LRange(ctx, key, 0, redisPreviewItems-1).Result()
		}
		v.Truncated = v.Size > redisPreviewItems
	case "set":
		v.Size, err = c.SCard(ctx, key).Result()
		if err == nil {
			var values []string
			var cursor uint64
			values, cursor, err = c.SScan(ctx, key, 0, "*", redisPreviewItems).Result()
			sort.Strings(values)
			v.Value = values
			v.Truncated = cursor != 0 || int64(len(values)) < v.Size || v.Size > redisPreviewItems
		}
	case "zset":
		v.Size, err = c.ZCard(ctx, key).Result()
		if err == nil {
			v.Value, err = c.ZRangeWithScores(ctx, key, 0, redisPreviewItems-1).Result()
		}
		v.Truncated = v.Size > redisPreviewItems
	case "stream":
		v.Size, err = c.XLen(ctx, key).Result()
		if err == nil {
			v.Value, err = c.XRangeN(ctx, key, "-", "+", redisPreviewItems).Result()
		}
		v.Truncated = true
	default:
		return v, fmt.Errorf("type %s is not supported by this editor", v.Type)
	}
	if err != nil {
		return v, err
	}
	ttl, err := c.TTL(ctx, key).Result()
	if err != nil {
		return v, err
	}
	v.TTL = int64(ttl / time.Second)
	if ttl < 0 {
		v.TTL = int64(ttl)
	}
	data, err := json.Marshal(v.Value)
	if err != nil {
		return v, err
	}
	if len(data) > redisPreviewBytes+1024 {
		return v, errors.New("value exceeds the 2 MiB preview limit")
	}
	// JSON replaces invalid UTF-8; detect this before letting users overwrite bytes.
	valid := true
	switch data := v.Value.(type) {
	case string:
		valid = utf8.ValidString(data)
	case []string:
		for _, s := range data {
			valid = valid && utf8.ValidString(s)
		}
	case map[string]string:
		for k, s := range data {
			valid = valid && utf8.ValidString(k) && utf8.ValidString(s)
		}
	case []redis.Z:
		for _, z := range data {
			s, ok := z.Member.(string)
			valid = valid && ok && utf8.ValidString(s)
		}
	}
	v.Truncated = v.Truncated || !valid
	v.Version = fmt.Sprintf("%x", sha256.Sum256(append([]byte(v.Type+"\x00"), data...)))
	return v, nil
}

type redisWrite struct {
	Key       string          `json:"key"`
	Type      string          `json:"type"`
	Value     json.RawMessage `json:"value"`
	Version   string          `json:"version"`
	TTL       int64           `json:"ttl"` // -2 preserves expiry, -1 persists, positive values are seconds.
	Create    bool            `json:"create"`
	Operation string          `json:"operation"`
}

func writeRedisValue(ctx context.Context, c *redis.Client, req redisWrite) error {
	if len(req.Key) > 1<<20 || req.TTL < -2 || req.TTL == 0 || req.TTL > 315360000 {
		return errors.New("invalid key or TTL (use -2, -1 or 1..315360000 seconds)")
	}
	if req.Operation != "save" && req.Operation != "delete" && req.Operation != "ttl" {
		return errors.New("unknown operation")
	}
	var str string
	var fields map[string]string
	var items []string
	var scores []redis.Z
	if req.Operation == "save" {
		if len(req.Value) > redisPreviewBytes {
			return errors.New("value exceeds 2 MiB")
		}
		var err error
		switch req.Type {
		case "string":
			err = json.Unmarshal(req.Value, &str)
		case "hash":
			err = json.Unmarshal(req.Value, &fields)
			if len(fields) == 0 || len(fields) > redisPreviewItems {
				return errors.New("hash requires 1..500 fields")
			}
		case "list", "set":
			err = json.Unmarshal(req.Value, &items)
			if len(items) == 0 || len(items) > redisPreviewItems {
				return errors.New("collection requires 1..500 items")
			}
		case "zset":
			err = json.Unmarshal(req.Value, &scores)
			if len(scores) == 0 || len(scores) > redisPreviewItems {
				return errors.New("sorted set requires 1..500 members")
			}
			for _, z := range scores {
				if _, ok := z.Member.(string); !ok {
					return errors.New("sorted set members must be strings")
				}
			}
		default:
			return errors.New("unsupported editable type")
		}
		if err != nil {
			return err
		}
	}
	return c.Watch(ctx, func(tx *redis.Tx) error {
		var expires time.Time
		if req.Create {
			if req.Operation != "save" {
				return errors.New("create requires save")
			}
			n, err := tx.Exists(ctx, req.Key).Result()
			if err != nil {
				return err
			}
			if n != 0 {
				return errors.New("key already exists")
			}
		} else {
			old, err := readRedisValue(ctx, tx, req.Key)
			if err != nil {
				return err
			}
			if old.Version != req.Version || req.Version == "" {
				return errors.New("key changed; reload before applying changes")
			}
			if req.Operation == "save" && (old.Truncated || old.Type != req.Type) {
				return errors.New("this preview cannot be overwritten")
			}
			pttl, err := tx.PTTL(ctx, req.Key).Result()
			if err != nil {
				return err
			}
			if pttl == -2 {
				return redis.Nil
			}
			if pttl >= 0 {
				expires = time.Now().Add(pttl)
			}
		}
		_, err := tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			if req.Operation == "delete" {
				pipe.Del(ctx, req.Key)
				return nil
			}
			if req.Operation == "save" {
				pipe.Del(ctx, req.Key)
				switch req.Type {
				case "string":
					pipe.Set(ctx, req.Key, str, 0)
				case "hash":
					pipe.HSet(ctx, req.Key, fields)
				case "list":
					values := make([]any, len(items))
					for i, s := range items {
						values[i] = s
					}
					pipe.RPush(ctx, req.Key, values...)
				case "set":
					values := make([]any, len(items))
					for i, s := range items {
						values[i] = s
					}
					pipe.SAdd(ctx, req.Key, values...)
				case "zset":
					pipe.ZAdd(ctx, req.Key, scores...)
				}
			}
			if req.TTL == -1 {
				pipe.Persist(ctx, req.Key)
			} else if req.TTL > 0 {
				pipe.Expire(ctx, req.Key, time.Duration(req.TTL)*time.Second)
			} else if !expires.IsZero() {
				pipe.PExpireAt(ctx, req.Key, expires)
			}
			return nil
		})
		return err
	}, req.Key)
}

func (s *server) handleRedis(w http.ResponseWriter, r *http.Request) {
	if !requireFileBackend(w, r) {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	action := strings.TrimPrefix(r.URL.Path, "/api/redis/")
	if action == "profiles" {
		handleRedisProfiles(w, r)
		return
	}
	methods := map[string]string{"test": "POST", "catalog": "GET", "scan": "GET", "value": "GET", "write": "POST"}
	method, ok := methods[action]
	if !ok {
		w.WriteHeader(404)
		return
	}
	if r.Method != method {
		w.WriteHeader(405)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	db := 0
	var err error
	if raw := r.URL.Query().Get("db"); raw != "" {
		db, err = strconv.Atoi(raw)
	}
	if err != nil || db < 0 || db > 65535 {
		redisAPIError(w, 400, errors.New("invalid database index"))
		return
	}
	var p redisProfile
	if action == "test" {
		var req struct {
			redisProfile
			Password *string `json:"password"`
		}
		if err = decodeDatabaseRequest(w, r, &req); err != nil {
			redisAPIError(w, 400, err)
			return
		}
		p = req.redisProfile
		if req.Password != nil {
			p.Password = *req.Password
		} else if p.ID != "" {
			redisProfilesMu.Lock()
			profiles, e := readRedisProfiles()
			redisProfilesMu.Unlock()
			if e != nil {
				redisAPIError(w, 500, e)
				return
			}
			for _, old := range profiles {
				if old.ID == p.ID {
					p.Password = old.Password
				}
			}
		}
		p, err = redisConfig(p)
	} else {
		redisProfilesMu.Lock()
		profiles, e := readRedisProfiles()
		redisProfilesMu.Unlock()
		err = e
		for _, item := range profiles {
			if item.ID == r.URL.Query().Get("id") {
				p = item
				break
			}
		}
		if err == nil && p.ID == "" {
			redisAPIError(w, 404, errors.New("connection not found"))
			return
		}
	}
	if err != nil {
		redisAPIError(w, 400, err)
		return
	}
	c := redisClient(p, db)
	defer c.Close()
	var result any
	switch action {
	case "test":
		err = c.Ping(ctx).Err()
		result = map[string]bool{"ok": true}
	case "catalog":
		var info string
		info, err = c.Info(ctx, "keyspace").Result()
		dbs := map[string]int64{}
		if err == nil {
			for _, line := range strings.Split(info, "\n") {
				line = strings.TrimSpace(line)
				if !strings.HasPrefix(line, "db") {
					continue
				}
				parts := strings.SplitN(line, ":", 2)
				if len(parts) != 2 {
					continue
				}
				for _, part := range strings.Split(parts[1], ",") {
					if strings.HasPrefix(part, "keys=") {
						n, _ := strconv.ParseInt(strings.TrimPrefix(part, "keys="), 10, 64)
						dbs[parts[0]] = n
					}
				}
			}
		}
		result = map[string]any{"databases": dbs}
	case "scan":
		cursor, e := strconv.ParseUint(r.URL.Query().Get("cursor"), 10, 64)
		if e != nil {
			redisAPIError(w, 400, errors.New("invalid scan cursor"))
			return
		}
		pattern := r.URL.Query().Get("pattern")
		if pattern == "" {
			pattern = "*"
		}
		var keys []string
		var next uint64
		keys, next, err = c.Scan(ctx, cursor, pattern, 200).Result()
		sort.Strings(keys)
		if keys == nil {
			keys = []string{}
		}
		result = map[string]any{"keys": keys, "cursor": strconv.FormatUint(next, 10)}
	case "value":
		result, err = readRedisValue(ctx, c, r.URL.Query().Get("key"))
	case "write":
		var req redisWrite
		err = decodeDatabaseRequest(w, r, &req)
		if err == nil {
			err = writeRedisValue(ctx, c, req)
		}
		result = map[string]bool{"ok": true}
	}
	if err != nil {
		code := 400
		if errors.Is(err, redis.Nil) {
			code = 404
			err = errors.New("key no longer exists")
		}
		if errors.Is(err, redis.TxFailedErr) {
			code = 409
			err = errors.New("key changed; reload before applying changes")
		}
		redisAPIError(w, code, err)
		return
	}
	writeJSON(w, 200, result)
}
