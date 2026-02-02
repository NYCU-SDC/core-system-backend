package main

import (
	"NYCU-SDC/core-system-backend/internal"
	"NYCU-SDC/core-system-backend/internal/auth"
	"NYCU-SDC/core-system-backend/internal/auth/casbin"
	"NYCU-SDC/core-system-backend/internal/config"
	"NYCU-SDC/core-system-backend/internal/cors"
	"NYCU-SDC/core-system-backend/internal/distribute"
	"NYCU-SDC/core-system-backend/internal/form"
	"NYCU-SDC/core-system-backend/internal/form/question"
	"NYCU-SDC/core-system-backend/internal/form/response"
	"NYCU-SDC/core-system-backend/internal/form/submit"
	"NYCU-SDC/core-system-backend/internal/form/workflow"
	"NYCU-SDC/core-system-backend/internal/inbox"
	"NYCU-SDC/core-system-backend/internal/jwt"
	"NYCU-SDC/core-system-backend/internal/publish"
	"NYCU-SDC/core-system-backend/internal/tenant"
	"NYCU-SDC/core-system-backend/internal/unit"

	"NYCU-SDC/core-system-backend/internal/trace"
	"NYCU-SDC/core-system-backend/internal/user"
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	databaseutil "github.com/NYCU-SDC/summer/pkg/database"
	logutil "github.com/NYCU-SDC/summer/pkg/log"
	"github.com/NYCU-SDC/summer/pkg/middleware"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.6.1"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var AppName = "no-app-name"

var Version = "no-version"

var BuildTime = "no-build-time"

var CommitHash = "no-commit-hash"

var Environment = "no-env"

func main() {
	AppName = os.Getenv("APP_NAME")
	if AppName == "" {
		AppName = "core-system-backend"
	}

	if BuildTime == "no-build-time" {
		now := time.Now()
		BuildTime = "not provided (now: " + now.Format(time.RFC3339) + ")"
	}

	Environment = os.Getenv("ENV")
	if Environment == "" {
		Environment = "no-env"
	}

	appMetadata := []zap.Field{
		zap.String("app_name", AppName),
		zap.String("version", Version),
		zap.String("build_time", BuildTime),
		zap.String("commit_hash", CommitHash),
		zap.String("environment", Environment),
	}

	cfg, cfgLog := config.Load()
	err := cfg.Validate()
	if err != nil {
		if errors.Is(err, config.ErrDatabaseURLRequired) {
			title := "Database URL is required"
			message := "Please set the DATABASE_URL environment variable or provide a config file with the database_url key."
			message = EarlyApplicationFailed(title, message)
			log.Fatal(message)
		} else {
			log.Fatalf("Failed to validate config: %v, exiting...", err)
		}
	}

	logger, err := initLogger(&cfg, appMetadata)
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v, exiting...", err)
	}

	cfgLog.FlushToZap(logger)

	if cfg.Dev {
		logger.Warn("Running in development mode, make sure to disable it in production")
	}

	if cfg.Secret == config.DefaultSecret && !cfg.Debug {
		logger.Warn("Default secret detected in production environment, replace it with a secure random string")
		cfg.Secret = uuid.New().String()
	}

	logger.Info("Starting application...")

	logger.Info("Starting database migration...")

	err = databaseutil.MigrationUp(cfg.MigrationSource, cfg.DatabaseURL, logger)
	if err != nil {
		logger.Fatal("Failed to run database migration", zap.Error(err))
	}

	dbPool, err := initDatabasePool(cfg.DatabaseURL)
	if err != nil {
		logger.Fatal("Failed to initialize database pool", zap.Error(err))
	}
	defer dbPool.Close()

	shutdown, err := initOpenTelemetry(AppName, Version, BuildTime, CommitHash, Environment, cfg.OtelCollectorUrl)
	if err != nil {
		logger.Fatal("Failed to initialize OpenTelemetry", zap.Error(err))
	}

	validator := internal.NewValidator()
	problemWriter := internal.NewProblemWriter()

	// Service
	userService := user.NewService(logger, dbPool)
	jwtService := jwt.NewService(logger, dbPool, cfg.Secret, cfg.OauthProxySecret, cfg.AccessTokenExpiration, cfg.RefreshTokenExpiration)
	tenantService := tenant.NewService(logger, dbPool)
	unitService := unit.NewService(logger, dbPool, tenantService)
	distributeService := distribute.NewService(logger, unitService)
	formService := form.NewService(logger, dbPool)
	questionService := question.NewService(logger, dbPool)
	inboxService := inbox.NewService(logger, dbPool)
	responseService := response.NewService(logger, dbPool)
	submitService := submit.NewService(logger, formService, questionService, responseService)
	publishService := publish.NewService(logger, distributeService, formService, inboxService)
	workflowService := workflow.NewService(logger, dbPool, questionService)

	// Handler
	authHandler := auth.NewHandler(logger, validator, problemWriter, userService, jwtService, jwtService, cfg.BaseURL, cfg.OauthProxyBaseURL, Environment, cfg.Dev, cfg.AccessTokenExpiration, cfg.RefreshTokenExpiration, cfg.GoogleOauth)
	userHandler := user.NewHandler(logger, validator, problemWriter, userService)
	formHandler := form.NewHandler(logger, validator, problemWriter, formService, tenantService)
	questionHandler := question.NewHandler(logger, validator, problemWriter, questionService)
	unitHandler := unit.NewHandler(logger, validator, problemWriter, unitService, formService, tenantService, userService)
	responseHandler := response.NewHandler(logger, validator, problemWriter, responseService, questionService)
	submitHandler := submit.NewHandler(logger, validator, problemWriter, submitService)
	inboxHandler := inbox.NewHandler(logger, validator, problemWriter, inboxService, formService, unitService)
	publishHandler := publish.NewHandler(logger, validator, problemWriter, publishService)
	tenantHandler := tenant.NewHandler(logger, validator, problemWriter, tenantService)
	workflowHandler := workflow.NewHandler(logger, validator, problemWriter, workflowService)

	// Middleware
	traceMiddleware := trace.NewMiddleware(logger, cfg.Debug)
	corsMiddleware := cors.NewMiddleware(logger, cfg.AllowOrigins)
	jwtMiddleware := jwt.NewMiddleware(logger, validator, problemWriter, jwtService)
	tenantMiddleware := tenant.NewMiddleware(logger, dbPool, tenantService)

	// Basic Middleware (Tracing and Recovery)
	basicMiddleware := middleware.NewSet(traceMiddleware.RecoverMiddleware)
	basicMiddleware = basicMiddleware.Append(traceMiddleware.TraceMiddleware)

	// Auth Middleware
	authMiddleware := middleware.NewSet(traceMiddleware.RecoverMiddleware)
	authMiddleware = authMiddleware.Append(traceMiddleware.TraceMiddleware)
	authMiddleware = authMiddleware.Append(jwtMiddleware.AuthenticateMiddleware)

	// Tenant-aware Middleware
	tenantBasicMiddleware := basicMiddleware.Append(tenantMiddleware.Middleware)
	tenantAuthMiddleware := authMiddleware.Append(tenantMiddleware.Middleware)

	casbin.Init()

	if casbin.Enforcer == nil {
		logger.Fatal("casbin enforcer init failed")
	}
	//Casbin Middleware
	casbinAuthMiddleware := auth.NewMiddleware(logger, casbin.Enforcer, unitService, tenantService)
	tenantCasbinAuthMiddleware := tenantAuthMiddleware.Append(casbinAuthMiddleware.Middleware)

	// HTTP Server
	mux := http.NewServeMux()

	// Health check route
	mux.Handle("GET /api/healthz", basicMiddleware.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("OK"))
		if err != nil {
			logger.Error("Failed to write response", zap.Error(err))
		}
	}))

	// Internal Debug route
	mux.Handle("POST /api/auth/login/internal", basicMiddleware.HandlerFunc(authHandler.InternalAPITokenLogin))

	// OAuth2 Authentication routes
	mux.Handle("GET /api/auth/login/oauth/{provider}", basicMiddleware.HandlerFunc(authHandler.Oauth2Start))
	mux.Handle("GET /api/auth/login/oauth/{provider}/callback", basicMiddleware.HandlerFunc(authHandler.Callback))

	// JWT refresh route
	mux.Handle("POST /api/auth/refresh", basicMiddleware.HandlerFunc(authHandler.RefreshToken))

	mux.Handle("GET /api/auth/logout", basicMiddleware.HandlerFunc(authHandler.Logout))
	mux.Handle("POST /api/auth/logout", basicMiddleware.HandlerFunc(authHandler.Logout))

	// User authenticated routes
	mux.Handle("GET /api/users/me", authMiddleware.HandlerFunc(userHandler.GetMe))

	// Unit routes
	mux.Handle("POST /api/orgs", authMiddleware.HandlerFunc(unitHandler.CreateOrg))
	mux.Handle("POST /api/orgs/{slug}/units", tenantCasbinAuthMiddleware.HandlerFunc(unitHandler.CreateUnit))
	mux.Handle("GET /api/orgs/{slug}", tenantBasicMiddleware.HandlerFunc(unitHandler.GetOrgByID))
	mux.Handle("GET /api/orgs", basicMiddleware.HandlerFunc(unitHandler.GetAllOrganizations))
	mux.Handle("GET /api/orgs/me", authMiddleware.HandlerFunc(unitHandler.ListOrganizationsOfCurrentUser))
	mux.Handle("GET /api/orgs/{slug}/units/{id}", tenantBasicMiddleware.HandlerFunc(unitHandler.GetUnitByID))
	mux.Handle("POST /api/orgs/relations", authMiddleware.HandlerFunc(unitHandler.AddParentChild))
	mux.Handle("PUT /api/orgs/{slug}", tenantCasbinAuthMiddleware.HandlerFunc(unitHandler.UpdateOrg))
	mux.Handle("PUT /api/orgs/{slug}/units/{id}", tenantCasbinAuthMiddleware.HandlerFunc(unitHandler.UpdateUnit))
	mux.Handle("DELETE /api/orgs/{slug}", tenantCasbinAuthMiddleware.HandlerFunc(unitHandler.DeleteOrg))
	mux.Handle("DELETE /api/orgs/{slug}/units/{id}", tenantCasbinAuthMiddleware.HandlerFunc(unitHandler.DeleteUnit))
	mux.Handle("POST /api/orgs/{slug}/members", tenantAuthMiddleware.HandlerFunc(unitHandler.AddOrgMember))
	mux.Handle("GET /api/orgs/{slug}/members", tenantBasicMiddleware.HandlerFunc(unitHandler.ListOrgMembers))
	mux.Handle("DELETE /api/orgs/{slug}/members/{member_id}", tenantCasbinAuthMiddleware.HandlerFunc(unitHandler.RemoveOrgMember))
	mux.Handle("POST /api/orgs/{slug}/units/{id}/members", tenantAuthMiddleware.HandlerFunc(unitHandler.AddUnitMember))
	mux.Handle("GET /api/orgs/{slug}/units/{id}/members", tenantBasicMiddleware.HandlerFunc(unitHandler.ListUnitMembers))
	mux.Handle("DELETE /api/orgs/{slug}/units/{id}/members/{member_id}", tenantCasbinAuthMiddleware.HandlerFunc(unitHandler.RemoveUnitMember))
	//TODO
	//mux.Handle("PATCH /api/orgs/{slug}/members/{member_id}", tenantAuthMiddleware.HandlerFunc(unitHandler.))
	//mux.Handle("PATCH /api/orgs/{slug}/units/{id}/members/{member_id}", tenantAuthMiddleware.HandlerFunc(unitHandler.))

	// Slug availability and history
	mux.Handle("GET /api/orgs/{slug}/status", basicMiddleware.HandlerFunc(tenantHandler.GetStatus))
	mux.Handle("GET /api/orgs/{slug}/history", basicMiddleware.HandlerFunc(tenantHandler.GetStatusWithHistory))

	// List sub-units
	mux.Handle("GET /api/orgs/{slug}/units", tenantBasicMiddleware.HandlerFunc(unitHandler.ListOrgSubUnits))
	mux.Handle("GET /api/orgs/{slug}/units/{id}/subunits", tenantBasicMiddleware.HandlerFunc(unitHandler.ListUnitSubUnits))
	mux.Handle("GET /api/orgs/{slug}/unit-ids", tenantBasicMiddleware.HandlerFunc(unitHandler.ListOrgSubUnitIDs))
	mux.Handle("GET /api/orgs/{slug}/units/{id}/subunit-ids", tenantBasicMiddleware.HandlerFunc(unitHandler.ListUnitSubUnitIDs))

	// Form routes
	mux.Handle("GET /api/forms", authMiddleware.HandlerFunc(formHandler.ListHandler))
	mux.Handle("GET /api/forms/{id}", authMiddleware.HandlerFunc(formHandler.GetHandler))
	mux.Handle("PUT /api/forms/{id}", authMiddleware.HandlerFunc(formHandler.UpdateHandler))
	mux.Handle("DELETE /api/forms/{id}", authMiddleware.HandlerFunc(formHandler.DeleteHandler))
	mux.Handle("POST /api/forms/recipients/preview", authMiddleware.HandlerFunc(publishHandler.PreviewForm))
	mux.Handle("POST /api/forms/{id}/publish", authMiddleware.HandlerFunc(publishHandler.PublishForm))
	mux.Handle("POST /api/orgs/{slug}/units/{unitId}/forms", tenantCasbinAuthMiddleware.HandlerFunc(formHandler.CreateUnderUnitHandler))
	mux.Handle("GET /api/orgs/{slug}/units/{unitId}/forms", tenantBasicMiddleware.HandlerFunc(formHandler.ListByUnitHandler))
	mux.Handle("POST /api/orgs/{slug}/forms", tenantAuthMiddleware.HandlerFunc(formHandler.CreateUnderOrgHandler))
	mux.Handle("GET /api/orgs/{slug}/forms", tenantBasicMiddleware.HandlerFunc(formHandler.ListByOrgHandler))

	// Question routes
	mux.Handle("GET /api/forms/{formId}/questions", authMiddleware.HandlerFunc(questionHandler.ListHandler))
	mux.Handle("POST /api/forms/{formId}/questions", authMiddleware.HandlerFunc(questionHandler.AddHandler))
	mux.Handle("PUT /api/forms/{formId}/questions/{questionId}", authMiddleware.HandlerFunc(questionHandler.UpdateHandler))
	mux.Handle("DELETE /api/forms/{formId}/questions/{questionId}", authMiddleware.HandlerFunc(questionHandler.DeleteHandler))

	// Response routes
	mux.Handle("GET /api/forms/{formId}/responses", authMiddleware.HandlerFunc(responseHandler.ListHandler))
	mux.Handle("POST /api/forms/{formId}/responses", authMiddleware.HandlerFunc(submitHandler.SubmitHandler))
	mux.Handle("GET /api/forms/{formId}/responses/{responseId}", authMiddleware.HandlerFunc(responseHandler.GetHandler))
	mux.Handle("DELETE /api/forms/{formId}/responses/{responseId}", authMiddleware.HandlerFunc(responseHandler.DeleteHandler))
	mux.Handle("GET /api/forms/{formId}/questions/{questionId}", authMiddleware.HandlerFunc(responseHandler.GetAnswersByQuestionIDHandler))

	// Workflow routes
	mux.Handle("GET /api/forms/{id}/workflow", authMiddleware.HandlerFunc(workflowHandler.GetWorkflow))
	mux.Handle("PUT /api/forms/{id}/workflow", authMiddleware.HandlerFunc(workflowHandler.UpdateWorkflow))
	mux.Handle("POST /api/forms/{id}/workflow/activate", authMiddleware.HandlerFunc(workflowHandler.ActivateWorkflow))
	mux.Handle("POST /api/forms/{formId}/workflow/nodes", authMiddleware.HandlerFunc(workflowHandler.CreateNode))
	mux.Handle("DELETE /api/forms/{formId}/workflow/nodes/{nodeId}", authMiddleware.HandlerFunc(workflowHandler.DeleteNode))

	// User Inbox message route
	mux.Handle("GET /api/inbox", authMiddleware.HandlerFunc(inboxHandler.ListHandler))
	mux.Handle("GET /api/inbox/{id}", authMiddleware.HandlerFunc(inboxHandler.GetHandler))
	mux.Handle("PUT /api/inbox/{id}", authMiddleware.HandlerFunc(inboxHandler.UpdateHandler))

	// handle interrupt signal
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// CORS and Entry Point
	entrypoint := corsMiddleware.HandlerFunc(mux.ServeHTTP)

	srv := &http.Server{
		Addr:    cfg.Host + ":" + cfg.Port,
		Handler: entrypoint,
	}

	go func() {
		logger.Info("Starting listening request", zap.String("host", cfg.Host), zap.String("port", cfg.Port))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal("Fail to start server with error", zap.Error(err))
		}
	}()

	// wait for context close
	<-ctx.Done()
	logger.Info("Shutting down gracefully...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("Server forced to shutdown", zap.Error(err))
	}

	otelCtx, otelCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer otelCancel()
	if err := shutdown(otelCtx); err != nil {
		logger.Error("Forced to shutdown OpenTelemetry", zap.Error(err))
	}

	logger.Info("Successfully shutdown")
}

func initLogger(cfg *config.Config, appMetadata []zap.Field) (*zap.Logger, error) {
	var err error
	var logger *zap.Logger
	if cfg.Debug {
		logger, err = logutil.ZapDevelopmentConfig().Build()
		if err != nil {
			return nil, err
		}
		logger.Info("Running in debug mode", appMetadata...)
	} else {
		logger, err = logutil.ZapProductionConfig().Build()
		if err != nil {
			return nil, err
		}

		logger = logger.With(appMetadata...)
	}
	defer func() {
		err := logger.Sync()
		if err != nil {
			zap.S().Errorw("Failed to sync logger", zap.Error(err))
		}
	}()

	return logger, nil
}

func initDatabasePool(databaseURL string) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}

	dbPool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return nil, err
	}
	return dbPool, nil
}

func initOpenTelemetry(appName, version, buildTime, commitHash, environment, otelCollectorUrl string) (func(context.Context) error, error) {
	ctx := context.Background()

	serviceName := semconv.ServiceNameKey.String(appName)
	serviceVersion := semconv.ServiceVersionKey.String(version)
	serviceNamespace := semconv.ServiceNamespaceKey.String("example")
	serviceCommitHash := attribute.String("service.commit_hash", commitHash)
	serviceEnvironment := semconv.DeploymentEnvironmentKey.String(environment)

	res, err := resource.New(ctx,
		resource.WithAttributes(
			serviceName,
			serviceVersion,
			serviceNamespace,
			serviceCommitHash,
			serviceEnvironment,
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	options := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	}

	if otelCollectorUrl != "" {
		conn, err := initGrpcConn(otelCollectorUrl)
		if err != nil {
			return nil, fmt.Errorf("failed to create gRPC connection: %w", err)
		}

		traceExporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
		if err != nil {
			return nil, fmt.Errorf("failed to create trace exporter: %w", err)
		}

		bsp := sdktrace.NewBatchSpanProcessor(traceExporter)
		options = append(options, sdktrace.WithSpanProcessor(bsp))
	}

	tracerProvider := sdktrace.NewTracerProvider(options...)

	otel.SetTracerProvider(tracerProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tracerProvider.Shutdown, nil
}

func initGrpcConn(target string) (*grpc.ClientConn, error) {
	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC connection: %w", err)
	}

	return conn, nil
}

func EarlyApplicationFailed(title, action string) string {
	result := `
-----------------------------------------
Application Failed to Start
-----------------------------------------

# What's wrong?
%s

# How to fix it?
%s

`

	result = fmt.Sprintf(result, title, action)
	return result
}
