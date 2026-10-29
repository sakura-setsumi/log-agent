package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/redis/go-redis/v9"
)

// A namespace is an exact, colon-terminated prefix, never a Redis glob.
func redisNamespacePattern(prefix string) (string, error) {
	if prefix == "" || !strings.HasSuffix(prefix, ":") || len(prefix) > 4096 {
		return "", errors.New("a non-empty colon-terminated namespace is required")
	}
	var pattern strings.Builder
	for _, r := range prefix {
		if strings.ContainsRune(`\*?[]`, r) {
			pattern.WriteByte('\\')
		}
		pattern.WriteRune(r)
	}
	pattern.WriteByte('*')
	return pattern.String(), nil
}
func redisNamespaceKeys(ctx context.Context, c *redis.Client, prefix string) ([]string, error) {
	pattern, err := redisNamespacePattern(prefix)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var cursor uint64
	for {
		batch, next, err := c.Scan(ctx, cursor, pattern, 200).Result()
		if err != nil {
			return nil, err
		}
		for _, key := range batch {
			if strings.HasPrefix(key, prefix) {
				seen[key] = true
				if len(seen) > 10000 {
					return nil, errors.New("namespace exceeds 10,000 keys; choose a smaller namespace")
				}
			}
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, nil
}

type redisNamespaceRequest struct {
	Prefix    string `json:"prefix"`
	Operation string `json:"operation"`
	Token     string `json:"token"`
}

func redisNamespaceAction(ctx context.Context, c *redis.Client, id string, db int, req redisNamespaceRequest) (any, error) {
	if req.Operation != "preview" && req.Operation != "delete" {
		return nil, errors.New("unknown namespace operation")
	}
	keys, err := redisNamespaceKeys(ctx, c, req.Prefix)
	if err != nil {
		return nil, err
	}
	data, _ := json.Marshal([]any{id, db, req.Prefix, keys})
	token := fmt.Sprintf("%x", sha256.Sum256(data))
	if req.Operation == "preview" {
		return map[string]any{"prefix": req.Prefix, "count": len(keys), "token": token}, nil
	}
	if req.Token == "" || req.Token != token {
		return nil, errors.New("namespace keys changed after preview; reload and confirm again")
	}
	var deleted int64
	// Delete only the confirmed key names, in bounded batches. Keys appearing
	// after this scan are not swept up by a second wildcard scan.
	for start := 0; start < len(keys); start += 100 {
		end := start + 100
		if end > len(keys) {
			end = len(keys)
		}
		count, err := c.Del(ctx, keys[start:end]...).Result()
		if err != nil {
			return nil, fmt.Errorf("namespace deletion stopped after %d keys; reload to verify remaining keys: %w", deleted, err)
		}
		deleted += count
	}
	return map[string]any{"deleted": deleted, "prefix": req.Prefix}, nil
}
