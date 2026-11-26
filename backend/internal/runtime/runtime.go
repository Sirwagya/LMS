package runtime

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	clockadapter "vedam/lms/internal/adapters/clock"
	"vedam/lms/internal/adapters/crypto"
	"vedam/lms/internal/adapters/grader"
	httpadapter "vedam/lms/internal/adapters/http"
	"vedam/lms/internal/adapters/postgres"
	redisadapter "vedam/lms/internal/adapters/redis"
	"vedam/lms/internal/adapters/storage"
	contestapp "vedam/lms/internal/application/contest"
	"vedam/lms/internal/application/finance"
	"vedam/lms/internal/application/learning"
	"vedam/lms/internal/application/staff"
	"vedam/lms/internal/application/workspace"
	"vedam/lms/internal/domain/catalog"
	"vedam/lms/internal/domain/contest"
	"vedam/lms/migrations"
)

type Runtime struct {
	Config       Config
	Log          *slog.Logger
	DB           *pgxpool.Pool
	Redis        *redis.Client
	Tx           *postgres.TxManager
	Sync         *postgres.SyncRepo
	Cache        *redisadapter.Cache
	Queue        *redisadapter.Queue
	Coding       *coding
	contestJudge grader.ContestJudge
}

func Open(ctx context.Context, config Config, log *slog.Logger) (*Runtime, error) {
	opts, err := redis.ParseURL(config.RedisURL)
	if err != nil {
		return nil, err
	}
	db, err := pgxpool.New(ctx, config.DatabaseURL)
	if err != nil {
		return nil, err
	}
	if err := migrations.Up(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	client := redis.NewClient(opts)
	tx := postgres.NewTxManager(db)
	changes := postgres.NewSyncRepo(tx)
	cache := redisadapter.NewCache(client)
	var official grader.ContestJudge
	if config.ContestJudgeURL != "" {
		official = grader.NewRemoteContestJudge(config.ContestJudgeURL, config.ContestJudgeToken)
	}
	return &Runtime{contestJudge: official, Config: config, Log: log, DB: db, Redis: client, Tx: tx, Sync: changes, Cache: cache,
		Queue: redisadapter.NewQueue(client),
		Coding: &coding{tx: tx, changes: changes, cache: cache, client: client, bridge: grader.Binary{Path: config.Grader},
			assignments: postgres.NewAssignmentRepo(tx), attempts: postgres.NewCodingAttemptRepo(tx), submissions: postgres.NewSubmissionRepo(tx), slots: make(chan struct{}, 2)},
	}, nil
}
func (rt *Runtime) Close() {
	_ = rt.Redis.Close()
	rt.DB.Close()
}
func (rt *Runtime) Handler() (http.Handler, func(), error) {
	tx := rt.Tx
	users := postgres.NewUserRepo(tx)
	cat := postgres.NewCatalogRepo(tx)
	schedule := postgres.NewScheduleRepo(tx)
	notices := postgres.NewNoticeRepo(tx)
	assignments := rt.Coding.assignments
	attendance := postgres.NewAttendanceRepo(tx)
	progress := postgres.NewProgressRepo(tx)
	discussions := postgres.NewDiscussionRepo(tx)
	subs := rt.Coding.submissions
	invoices := postgres.NewFinanceRepo(tx)
	assets := postgres.NewAssetMetaRepo(tx)
	blob, err := storage.NewLocalAssets(rt.Config.Assets)
	if err != nil {
		return nil, nil, err
	}
	hasher := cryptoadapter.NewDualPasswordHasher()
	now := clockadapter.SystemClock{}
	id := uuid.NewString
	bootstrap := workspace.NewBootstrapService(cat, schedule, notices, attendance, progress, subs, assignments, assignments, postgres.NewContestRepo(tx), invoices)
	bootstrap.LoadCatalog = func(ctx context.Context) (workspace.CatalogSnapshot, error) {
		key := redisadapter.CatalogKey
		raw, err := rt.Cache.Load(ctx, key, []string{key}, workspace.CatalogTTL, func(ctx context.Context) (json.RawMessage, error) {
			var snapshot workspace.CatalogSnapshot
			err := tx.WithinReadSnapshot(ctx, func(ctx context.Context) error {
				var err error
				snapshot, err = bootstrap.ReadCatalog(ctx)
				return err
			})
			if err != nil {
				return nil, err
			}
			return json.Marshal(snapshot)
		})
		if err != nil {
			return workspace.CatalogSnapshot{}, err
		}
		var snapshot workspace.CatalogSnapshot
		err = json.Unmarshal(raw, &snapshot)
		return snapshot, err
	}
	contestRepo := postgres.NewContestAuthoringRepo(tx)
	contestRepo.EnableScoring = rt.Config.ContestGrading && rt.contestJudge != nil
	var attempts contest.AttemptRepository
	if rt.Config.ContestAttempts && rt.Config.ContestAuthoring {
		repo := postgres.NewContestAttemptRepo(rt.Tx)
		repo.JudgeAvailable = rt.contestJudge != nil
		attempts = repo
	}
	var contests *contestapp.Service
	if rt.Config.ContestAuthoring {
		contests = contestapp.New(contestRepo, tx, now, id)
	}
	return httpadapter.New(httpadapter.Dependencies{
		ContestRepo: contestRepo, Contests: contests, ContestAttempts: attempts,
		Log: rt.Log, PingDB: rt.DB.Ping, Redis: rt.Redis, Sessions: redisadapter.NewSessions(rt.Redis, rt.Config.SessionTTL),
		Cache: rt.Cache, CookieSecure: rt.Config.CookieSecure, PublicOrigin: rt.Config.PublicOrigin, TrustedProxies: rt.Config.TrustedProxies, Production: rt.Config.Production, SessionSliding: rt.Config.SessionSliding, SessionTTL: rt.Config.SessionTTL, Web: webFiles(rt.Config.Web),
		Tx: tx, Users: users, Hasher: hasher, Catalog: cat, Notices: notices, Assignments: assignments, Submissions: subs,
		Attendance: attendance, Discussions: discussions, Progress: progress, Invoices: invoices, Academic: postgres.NewAcademicRepo(tx), Sync: rt.Sync,
		AssetMeta: assets, Assets: blob, Bootstrap: bootstrap,
		Learning: learning.NewService(tx, cat, assignments, subs, progress, discussions, assets, blob, rt.Sync, now, id),
		Staff:    staff.NewService(tx, cat, cat, schedule, notices, attendance, assignments, subs, users, users, hasher, invoices, rt.Sync, now, id),
		Payments: finance.NewPayInvoiceService(tx, invoices, invoices, invoices, rt.Sync, rt.Sync, now, id),
		Coding:   rt.Coding, Queue: rt.Queue,
	}), func() { _ = blob.Close() }, nil
}

func webFiles(dir string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}
		root, err := os.OpenRoot(dir)
		if err != nil {
			buildRequired(w)
			return
		}
		defer root.Close()
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		f, err := root.Open(name)
		if err == nil {
			defer f.Close()
			info, statErr := f.Stat()
			if statErr == nil && info.Mode().IsRegular() {
				w.Header().Set("X-Content-Type-Options", "nosniff")
				http.ServeContent(w, r, name, info.ModTime(), f)
				return
			}
		}
		index, err := root.Open("index.html")
		if err != nil {
			buildRequired(w)
			return
		}
		defer index.Close()
		w.Header().Set("X-Content-Type-Options", "nosniff")
		http.ServeContent(w, r, "index.html", time.Time{}, index)
	})
}
func buildRequired(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = w.Write([]byte(`{"error":"Run npm run build first."}`))
}

var _ catalog.AssetBlobStore = (*storage.LocalAssets)(nil)
