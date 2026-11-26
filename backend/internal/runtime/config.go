package runtime

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const defaultDBURL = "postgres://lms:lms_local_dev@127.0.0.1:5432/lms_dev?sslmode=disable"

type Config struct {
	Addr, DatabaseURL, RedisURL, Assets, Web, Grader                    string
	CookieSecure, SessionSliding                                        bool
	SessionTTL                                                          time.Duration
	Production                                                          bool
	ContestAuthoring, ContestAttempts, ContestGrading, ContestIntegrity bool
	ContestJudgeURL, ContestJudgeToken                                  string
	PublicOrigin                                                        string
	TrustedProxies                                                      []netip.Prefix
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
	sliding, err := strconv.ParseBool(get("SESSION_SLIDING", "false"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid SESSION_SLIDING")
	}
	addr := get("HTTP_ADDR", "")
	if addr == "" {
		addr = "127.0.0.1:" + get("PORT", "3001")
	}
	mode := get("APP_ENV", "development")
	if mode != "development" && mode != "production" {
		return Config{}, fmt.Errorf("invalid APP_ENV")
	}
	production := mode == "production"
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return Config{}, fmt.Errorf("invalid HTTP_ADDR")
	}
	if !production && (net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback()) {
		return Config{}, fmt.Errorf("development HTTP_ADDR must bind a loopback IP")
	}
	databaseURL, redisURL := get("DATABASE_URL", defaultDBURL), get("REDIS_URL", "redis://127.0.0.1:6379/0")
	publicOrigin := get("PUBLIC_ORIGIN", "")
	if production {
		database, dbErr := url.Parse(databaseURL)
		cache, cacheErr := url.Parse(redisURL)
		origin, originErr := url.Parse(publicOrigin)
		if !secure || originErr != nil || origin.Scheme != "https" || origin.Hostname() == "" || origin.User != nil ||
			origin.Path != "" || origin.RawQuery != "" || origin.ForceQuery || origin.Fragment != "" {
			return Config{}, fmt.Errorf("production requires COOKIE_SECURE=true and an HTTPS PUBLIC_ORIGIN")
		}
		if dbErr != nil || (database.Scheme != "postgres" && database.Scheme != "postgresql") ||
			database.Hostname() == "" || database.Query().Get("sslmode") != "verify-full" || database.User == nil {
			return Config{}, fmt.Errorf("production requires a PostgreSQL URL with credentials and sslmode=verify-full")
		}
		password, _ := database.User.Password()
		if password == "" || password == "lms_local_dev" {
			return Config{}, fmt.Errorf("production requires a non-demo PostgreSQL password")
		}
		if cacheErr != nil || cache.Scheme != "rediss" || cache.Hostname() == "" || cache.User == nil || cache.Query().Has("skip_verify") {
			return Config{}, fmt.Errorf("production requires a TLS Redis URL with credentials")
		}
		password, _ = cache.User.Password()
		if password == "" {
			return Config{}, fmt.Errorf("production requires Redis authentication")
		}
	}
	var proxies []netip.Prefix
	for _, raw := range strings.FieldsFunc(get("TRUSTED_PROXY_CIDRS", ""), func(r rune) bool { return r == ',' }) {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(raw))
		if err != nil || prefix.Bits() == 0 {
			return Config{}, fmt.Errorf("invalid TRUSTED_PROXY_CIDRS")
		}
		proxies = append(proxies, prefix.Masked())
	}
	contestAuthoring, err := strconv.ParseBool(get("LMS_CONTEST_AUTHORING", "false"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid LMS_CONTEST_AUTHORING")
	}
	attempts, err := strconv.ParseBool(get("LMS_CONTEST_ATTEMPTS", "false"))
	if err != nil || (attempts && !contestAuthoring) {
		return Config{}, fmt.Errorf("LMS_CONTEST_ATTEMPTS must be a boolean and requires LMS_CONTEST_AUTHORING=true when enabled")
	}
	grading, err := strconv.ParseBool(get("LMS_CONTEST_GRADING", "false"))
	if err != nil || (grading && !attempts) {
		return Config{}, fmt.Errorf("LMS_CONTEST_GRADING must be a boolean and requires LMS_CONTEST_ATTEMPTS=true when enabled")
	}
	integrity, err := strconv.ParseBool(get("LMS_CONTEST_INTEGRITY", "false"))
	if err != nil || (integrity && !attempts) {
		return Config{}, fmt.Errorf("LMS_CONTEST_INTEGRITY must be a boolean and requires LMS_CONTEST_ATTEMPTS=true when enabled")
	}
	judgeURL, judgeToken := get("LMS_CONTEST_JUDGE_URL", ""), get("LMS_CONTEST_JUDGE_TOKEN", "")
	if grading || judgeURL != "" || judgeToken != "" {
		if err := validateContestJudge(judgeURL, judgeToken, production); err != nil {
			return Config{}, err
		}
	}
	graderPath := path("GRADER_BINARY_PATH", "target/release/lms-grader")
	if os.Getenv("GRADER_BINARY_PATH") == "" {
		if _, err := os.Stat(graderPath); err != nil {
			debugPath := filepath.Join(root, "target/debug/lms-grader")
			if _, debugErr := os.Stat(debugPath); debugErr == nil {
				graderPath = debugPath
			}
		}
	}
	if production {
		graderPath = ""
	}
	return Config{
		Addr: addr, DatabaseURL: databaseURL, RedisURL: redisURL, Production: production, ContestAuthoring: contestAuthoring, ContestAttempts: attempts, ContestGrading: grading, ContestIntegrity: integrity,
		ContestJudgeURL: judgeURL, ContestJudgeToken: judgeToken, PublicOrigin: publicOrigin, TrustedProxies: proxies,
		Assets: path("ASSET_STORAGE_DIR", "backend/.data/assets"), Web: path("WEB_DIST_DIR", "dist"),
		Grader: graderPath, CookieSecure: secure, SessionSliding: sliding, SessionTTL: time.Duration(hours) * time.Hour,
	}, nil
}
func validateContestJudge(endpoint, token string, production bool) error {
	u, err := url.Parse(endpoint)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return fmt.Errorf("LMS_CONTEST_JUDGE_URL must be a dedicated HTTPS judge endpoint")
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme != "https" && (production || u.Scheme != "http" || ip == nil || !ip.IsLoopback()) {
		return fmt.Errorf("LMS_CONTEST_JUDGE_URL requires HTTPS outside loopback development")
	}
	if len(token) < 32 || strings.ContainsAny(token, " \t\r\n") {
		return fmt.Errorf("LMS_CONTEST_JUDGE_TOKEN must be a dedicated bearer secret of at least 32 characters")
	}
	return nil
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
