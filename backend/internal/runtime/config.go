package runtime

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const defaultDBURL = "postgres://lms:lms_local_dev@127.0.0.1:5432/lms_dev?sslmode=disable"

type Config struct {
	Addr, DatabaseURL, RedisURL, Assets, Web, Grader string
	CookieSecure                                     bool
	SessionTTL                                       time.Duration
}

func LoadConfig() (Config, error) {
	root, err := os.Getwd()
	if err != nil {
		return Config{}, err
	}
	if filepath.Base(root) == "backend" {
		root = filepath.Dir(root)
	}
	local, err := readEnv(filepath.Join(root, ".env"))
	if err != nil {
		return Config{}, err
	}
	get := func(key, fallback string) string {
		if value := os.Getenv(key); value != "" {
			return value
		}
		if value := local[key]; value != "" {
			return value
		}
		return fallback
	}
	path := func(key, fallback string) string {
		value := get(key, fallback)
		if filepath.IsAbs(value) {
			return value
		}
		return filepath.Join(root, value)
	}
	hours, err := strconv.Atoi(get("SESSION_TTL_HOURS", "24"))
	if err != nil || hours < 1 || hours > 720 {
		return Config{}, fmt.Errorf("invalid SESSION_TTL_HOURS")
	}
	secure, err := strconv.ParseBool(get("COOKIE_SECURE", "false"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid COOKIE_SECURE")
	}
	addr := get("HTTP_ADDR", "")
	if addr == "" {
		addr = "127.0.0.1:" + get("PORT", "3001")
	}
	return Config{
		Addr: addr, DatabaseURL: get("DATABASE_URL", defaultDBURL), RedisURL: get("REDIS_URL", "redis://127.0.0.1:6379/0"),
		Assets: path("ASSET_STORAGE_DIR", "backend/.data/assets"), Web: path("WEB_DIST_DIR", "dist"),
		Grader: path("GRADER_BRIDGE_PATH", "grader/bridge.mjs"), CookieSecure: secure, SessionTTL: time.Duration(hours) * time.Hour,
	}, nil
}
func readEnv(path string) (map[string]string, error) {
	values := map[string]string{}
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return values, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if ok {
			values[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), "\"'")
		}
	}
	return values, scanner.Err()
}
