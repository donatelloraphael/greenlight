package main

import (
	"context"
	"database/sql"
	"expvar"
	"flag"
	"fmt"
	"greenlight/internal/data"
	"greenlight/internal/jsonlog"
	"greenlight/internal/mailer"
	"greenlight/internal/vcs"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
)

var version = vcs.Version()

// config struct holds all the configuration settings for our application
type config struct {
	port int
	env  string
	db   struct {
		url          string
		maxOpenConns int
		maxIdleConns int
		maxIdleTime  string
	}
	limiter struct {
		rps     float64
		burst   int
		enabled bool
	}
	smtp struct {
		host     string
		port     int
		username string
		password string
		sender   string
	}
	cors struct {
		trustedOrigins []string
	}
}

// application struct holds the dependencies for our HTTP handlers, helpers, and middleware.
type application struct {
	config config
	logger *jsonlog.Logger
	models data.Models
	mailer mailer.Mailer
	wg     sync.WaitGroup
}

func main() {
	logger := jsonlog.New(os.Stdout, jsonlog.LevelInfo)

	err := godotenv.Load()
	if err != nil {
		logger.PrintFatal(err, nil)
	}

	var cfg config

	port, err := strconv.Atoi(os.Getenv("PORT"))
	if err != nil {
		logger.PrintFatal(fmt.Errorf("invalid port %s", err), nil)
	}
	flag.IntVar(&cfg.port, "PORT", port, "API server port")

	environment := os.Getenv("ENVIRONEMNT")
	if _, ok := map[string]bool{"development": true, "staging": true, "production": true}[environment]; !ok {
		logger.PrintFatal(fmt.Errorf("invalid environment %s", environment), nil)
	}
	flag.StringVar(&cfg.env, "ENVIRONEMNT", "development", "Environment (development|staging|production)")

	postgresUrl := os.Getenv("POSTGRESQL_URL")
	if postgresUrl == "" {
		logger.PrintFatal(fmt.Errorf("POSTGRESQL_URL is not set"), nil)
	}
	flag.StringVar(&cfg.db.url, "POSTGRESQL_URL", postgresUrl, "PostgreSQL DSN")

	postgresMaxOpenConns, err := strconv.Atoi(os.Getenv("POSTGRESQL_MAX_OPEN_CONNS"))
	if err != nil {
		logger.PrintFatal(fmt.Errorf("invalid POSTGRESQL_MAX_OPEN_CONNS %s", err), nil)
	}
	flag.IntVar(&cfg.db.maxOpenConns, "POSTGRESQL_MAX_OPEN_CONNS", postgresMaxOpenConns, "PostgreSQL max open connections")

	postgresMaxIdleConns, err := strconv.Atoi(os.Getenv("POSTGRESQL_MAX_IDLE_CONNS"))
	if err != nil {
		logger.PrintFatal(fmt.Errorf("invalid POSTGRESQL_MAX_IDLE_CONNS %s", err), nil)
	}
	flag.IntVar(&cfg.db.maxIdleConns, "POSTGRESQL_MAX_IDLE_CONNS", postgresMaxIdleConns, "PostgreSQL max idle connections")

	postgresMaxIdleTime := os.Getenv("POSTGRESQL_MAX_IDLE_TIME")
	if postgresMaxIdleTime == "" {
		logger.PrintFatal(fmt.Errorf("invalid POSTGRESQL_MAX_IDLE_TIME %s", err), nil)
	}
	flag.StringVar(&cfg.db.maxIdleTime, "POSTGRESQL_MAX_IDLE_TIME", postgresMaxIdleTime, "PostgreSQL max connection idle time")

	limiterRps, err := strconv.ParseFloat(os.Getenv("LIMITER_RPS"), 64)
	if err != nil {
		logger.PrintFatal(fmt.Errorf("invalid LIMITER_RPS %s", err), nil)
	}
	flag.Float64Var(&cfg.limiter.rps, "LIMITER_RPS", limiterRps, "Rate limiter maximum requests per second")

	limiterBurst, err := strconv.Atoi(os.Getenv("LIMITER_BURST"))
	if err != nil {
		logger.PrintFatal(fmt.Errorf("invalid LIMITER_BURST %s", err), nil)
	}
	flag.IntVar(&cfg.limiter.burst, "LIMITER_BURST", limiterBurst, "Rate limiter maximum burst")

	limiterEnabled, err := strconv.ParseBool(os.Getenv("LIMITER_ENABLED"))
	if err != nil {
		logger.PrintFatal(fmt.Errorf("invalid LIMITER_ENABLED %s", err), nil)
	}
	flag.BoolVar(&cfg.limiter.enabled, "LIMITER_ENABLED", limiterEnabled, "Enable rate limiter")

	smtpHost := os.Getenv("SMTP_HOST")
	if smtpHost == "" {
		logger.PrintFatal(fmt.Errorf("SMTP_HOST is not set"), nil)
	}
	flag.StringVar(&cfg.smtp.host, "SMTP_HOST", smtpHost, "SMTP server host")

	smtpPort, err := strconv.Atoi(os.Getenv("SMTP_PORT"))
	if err != nil {
		logger.PrintFatal(fmt.Errorf("invalid SMTP_PORT %s", err), nil)
	}
	flag.IntVar(&cfg.smtp.port, "SMTP_PORT", smtpPort, "SMTP server port")

	smtpUsername := os.Getenv("SMTP_USERNAME")
	if smtpUsername == "" {
		logger.PrintFatal(fmt.Errorf("SMTP_USERNAME is not set"), nil)
	}
	flag.StringVar(&cfg.smtp.username, "SMTP_USERNAME", smtpUsername, "SMTP server username")

	smtpPassword := os.Getenv("SMTP_PASSWORD")
	if smtpPassword == "" {
		logger.PrintFatal(fmt.Errorf("SMTP_PASSWORD is not set"), nil)
	}
	flag.StringVar(&cfg.smtp.password, "SMTP_PASSWORD", smtpPassword, "SMTP server password")

	smtpSender := os.Getenv("SMTP_SENDER")
	if smtpSender == "" {
		logger.PrintFatal(fmt.Errorf("SMTP_SENDER is not set"), nil)
	}
	flag.StringVar(&cfg.smtp.sender, "SMTP_SENDER", smtpSender, "SMTP sender")

	trustedOrigins := os.Getenv("CORS_TRUSTED_ORIGINS")
	flag.StringVar(&trustedOrigins, "CORS_TRUSTED_ORIGINS", trustedOrigins, "List of trusted CORS origins (space separated)")
	cfg.cors.trustedOrigins = strings.Fields(trustedOrigins)

	displayVersion := flag.Bool("version", false, "Display the version and exit")

	flag.Parse()

	if *displayVersion {
		fmt.Printf("Version:\t%s\n", version)
		os.Exit(0)
	}

	db, err := openDB(cfg)
	if err != nil {
		logger.PrintFatal(err, nil)
	}
	defer db.Close()

	logger.PrintInfo("database connection pool established", nil)

	expvar.NewString("version").Set(version)

	expvar.Publish("goroutines", expvar.Func(func() any {
		return runtime.NumGoroutine()
	}))

	expvar.Publish("database", expvar.Func(func() any {
		return db.Stats()
	}))

	expvar.Publish("timestamp", expvar.Func(func() any {
		return time.Now().Unix()
	}))

	app := &application{
		config: cfg,
		logger: logger,
		models: data.NewModels(db),
		mailer: mailer.New(cfg.smtp.host, cfg.smtp.port, cfg.smtp.username, cfg.smtp.password, cfg.smtp.sender),
	}

	err = app.serve()
	if err != nil {
		logger.PrintFatal(err, nil)
	}
}

func openDB(cfg config) (*sql.DB, error) {
	db, err := sql.Open("pgx", cfg.db.url)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(cfg.db.maxOpenConns)
	db.SetMaxIdleConns(cfg.db.maxIdleConns)

	duration, err := time.ParseDuration(cfg.db.maxIdleTime)
	if err != nil {
		return nil, err
	}

	db.SetConnMaxIdleTime(duration)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = db.PingContext(ctx)
	if err != nil {
		return nil, err
	}

	return db, nil
}
