package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"git-ai-server/internal/auth"
	"git-ai-server/internal/config"
	gitcrypto "git-ai-server/internal/crypto"
	"git-ai-server/internal/database"
	"git-ai-server/internal/handler"
	"git-ai-server/internal/middleware"
	"git-ai-server/internal/model"
	"git-ai-server/internal/service"

	"github.com/gin-gonic/gin"
)

// commitHash is overridden at build time via:
//
//	go build -ldflags="-X main.commitHash=$(git rev-parse --short HEAD)"
//
// scripts/deploy.sh build does this automatically. Falls back to "dev" so
// `go run ./cmd/server` still works.
var commitHash = "dev"

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	isProduction := cfg.AppEnv == "production"
	if isProduction {
		gin.SetMode(gin.ReleaseMode)
	}

	masterKey, err := gitcrypto.ResolveEncryptionMasterKey(cfg.EncryptionMasterKey, isProduction)
	if err != nil {
		log.Fatalf("Failed to resolve encryption master key: %v", err)
	}

	casKey, err := gitcrypto.ResolveCASEncryptionKey(cfg.CASEncryptionKey, isProduction)
	if err != nil {
		log.Fatalf("Failed to resolve CAS encryption key: %v", err)
	}

	if cfg.JWTSecret == "" {
		log.Fatal("JWT_SECRET must be set before starting the server")
	}

	ctx := context.Background()
	dbURL := cfg.DatabaseURL()

	if err := database.RunMigrations(dbURL); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	pool, err := database.NewPool(ctx, dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()

	// Services
	auditSvc := &service.AuditService{Pool: pool}
	_ = auditSvc
	userSvc := &service.UserService{Pool: pool}
	metricsSvc := &service.MetricsService{Pool: pool}
	casSvc := &service.CasService{Pool: pool, CASKey: casKey}
	authorshipSvc := &service.AuthorshipService{Pool: pool}
	bundleSvc := &service.BundleService{Pool: pool}
	dashboardSvc := &service.DashboardService{Pool: pool, MetricsSvc: metricsSvc}
	sysConfigSvc := &service.SysConfigService{Pool: pool, MasterKey: masterKey}

	deviceFlowSvc := &auth.DeviceFlowService{Pool: pool, JWTSecret: cfg.JWTSecret}

	// Bootstrap initial admin user
	bootstrapAdminUser(ctx, userSvc, cfg)

	trustProxy := cfg.TrustProxyEnabled()
	jsonBodyLimit := cfg.ParsedJSONBodyLimit()
	casUploadLimit := cfg.ParsedCASUploadLimit()

	// Handlers
	loginH := &handler.LoginHandler{UserSvc: userSvc, JWTSecret: cfg.JWTSecret, IsProduction: isProduction}
	healthH := &handler.HealthHandler{Pool: pool}
	compatH := &handler.CompatibilityHandler{
		DashboardSvc:  dashboardSvc,
		AuthorshipSvc: authorshipSvc,
		CasSvc:        casSvc,
		DeviceFlowSvc: deviceFlowSvc,
		MetricsSvc:    metricsSvc,
		TrustProxy:    trustProxy,
		Commit:        commitHash,
	}
	authorshipH := &handler.AuthorshipHandler{Svc: authorshipSvc}
	bundleH := &handler.BundleHandler{Svc: bundleSvc, TrustProxy: trustProxy}
	casH := &handler.CasHandler{Svc: casSvc}
	dashboardH := &handler.DashboardHandler{Svc: dashboardSvc}
	releaseStore := &service.ReleaseStore{Root: cfg.ReleaseStoragePath}
	releaseH := &handler.ReleaseHandler{Store: releaseStore}
	releaseAdminH := &handler.ReleaseAdminHandler{Store: releaseStore}
	uploadAuth := middleware.UploadTokenAuth(cfg.ReleaseUploadToken)
	sysConfigH := &handler.SysConfigHandler{Svc: sysConfigSvc}
	deviceFlowH := &handler.DeviceFlowHandler{Svc: deviceFlowSvc}

	r := gin.Default()

	// Global middleware
	r.Use(middleware.SecurityHeadersMiddleware())
	if cfg.HTTPSRedirect {
		r.Use(middleware.HTTPSRedirectMiddleware())
	}
	r.Use(middleware.AuditMiddleware(pool, trustProxy))

	// CORS
	r.Use(corsMiddleware(cfg.CORSOrigin))

	csrfMW := middleware.CSRFProtect()

	jsonLimit := middleware.BodyLimit(jsonBodyLimit)
	casLimit := middleware.BodyLimit(casUploadLimit)

	jwtMW := auth.JWTAuthMiddleware(cfg.JWTSecret)
	workerMW := auth.WorkerAuthMiddleware(cfg.JWTSecret, cfg.ValidAPIKeys(), defaultTokenSubject(cfg))

	// --- Direct routes (no /api prefix) ---
	r.GET("/health", healthH.GetHealth)

	// --- Worker compatibility endpoints ---
	workerRoutes := []string{"worker", "workers"}
	for _, prefix := range workerRoutes {
		r.POST("/"+prefix+"/oauth/device/code", jsonLimit, compatH.StartDeviceFlow)
		r.POST("/"+prefix+"/oauth/token", jsonLimit, compatH.ExchangeOAuthToken)
		r.POST("/"+prefix+"/metrics/upload", jsonLimit, workerMW, compatH.UploadWorkerMetrics)
		r.POST("/"+prefix+"/cas/upload", casLimit, workerMW, compatH.UploadWorkerCas)
		r.GET("/"+prefix+"/cas", workerMW, compatH.ReadWorkerCas)
		r.GET("/"+prefix+"/cas/", workerMW, compatH.ReadWorkerCas)
		r.GET("/"+prefix+"/cas/checkout", workerMW, compatH.CheckoutWorkerCas)
		r.GET("/"+prefix+"/releases", releaseH.GetReleases)
		r.GET("/"+prefix+"/releases/:channel/download/:name", releaseH.Download)
	}

	// --- /api/* routes ---
	// Body limits are applied per subgroup because http.MaxBytesReader only
	// tightens — once a group installs jsonLimit, subgroups can't relax it.
	api := r.Group("/api")
	{
		api.GET("/health", healthH.GetAPIHealth)
		api.GET("/health/database", healthH.GetDatabaseHealth)
		api.GET("/status", compatH.GetStatus)
		api.GET("/version", compatH.GetVersion)
		api.GET("/me", jwtMW, compatH.GetMe)

		// Device flow (cookie-session). info is read-only; approve/deny are
		// CSRF-protected writes that also require a logged-in cookie.
		device := api.Group("/oauth/device", jsonLimit)
		{
			device.GET("/info", deviceFlowH.Info)
			device.POST("/approve", csrfMW, deviceFlowH.Approve)
			device.POST("/deny", csrfMW, deviceFlowH.Deny)
		}

		// User auth
		api.POST("/user/login", jsonLimit, loginH.Login)
		api.GET("/user/logout", loginH.Logout)
		api.POST("/user/logout", csrfMW, loginH.Logout)
		api.POST("/user/register", jsonLimit, jwtMW, csrfMW, adminOnly(), loginH.Register)
		api.POST("/bundles", jsonLimit, jwtMW, csrfMW, bundleH.Create)

		// Release admin (upload Bearer token auth). These routes set their
		// own limit via http.MaxBytesReader in the handler, so no jsonLimit
		// here.
		api.PUT("/releases/:channel/artifacts/:tag/:name", uploadAuth, releaseAdminH.PutArtifact)
		api.PUT("/releases/:channel/current.json", uploadAuth, releaseAdminH.PutCurrent)
		api.GET("/releases/:channel/current.json", uploadAuth, releaseAdminH.GetCurrent)

		// Authorship. Writes go through workerMW so CLI tokens and X-API-Key
		// credentials both work; reads go through jwtMW since the /me
		// browser session is the only expected reader.
		authorshipWrite := api.Group("/authorship", jsonLimit, workerMW)
		{
			authorshipWrite.POST("/record", authorshipH.SaveRecord)
			authorshipWrite.POST("/commit", authorshipH.SaveCommitAttribution)
			authorshipWrite.PUT("/sync/:userId", authorshipH.SyncAuthorship)
		}
		authorshipRead := api.Group("/authorship", jsonLimit, jwtMW)
		{
			authorshipRead.GET("/commits/:userId", authorshipH.GetUserCommits)
			authorshipRead.GET("/commits/:userId/:commitHash", authorshipH.GetUserCommitByHash)
			authorshipRead.GET("/commit/:commitHash", authorshipH.GetCommitAttribution)
		}

		// CAS — gets the larger casLimit since payloads can be bigger than
		// the generic 2MB JSON budget. Worker auth accepts either a CLI JWT
		// or an X-API-Key.
		cas := api.Group("/cas", casLimit, workerMW)
		{
			cas.POST("/upload", casH.Upload)
			cas.GET("/read/:hash", casH.Read)
		}

		// Dashboard. Public stats stay open; per-user stats require a
		// session and always map to the caller's own sub.
		dashboard := api.Group("/dashboard", jsonLimit)
		{
			dashboard.GET("/public", dashboardH.GetPublicStats)
			dashboard.GET("/stats", jwtMW, dashboardH.GetStats)
			dashboard.POST("/generate-report", jwtMW, csrfMW, dashboardH.GenerateReport)
		}

		// Config (JWT protected)
		cfgGroup := api.Group("/config", jsonLimit, jwtMW)
		{
			cfgGroup.GET("", sysConfigH.GetAll)
			cfgGroup.GET("/:key", sysConfigH.GetByKey)
			cfgGroup.POST("", csrfMW, sysConfigH.Create)
			cfgGroup.PATCH("/:key", csrfMW, sysConfigH.Update)
			cfgGroup.DELETE("/:key", csrfMW, sysConfigH.Delete)
		}
	}

	// Startup logging
	port := cfg.Port
	log.Printf("Application is running on: http://localhost:%d", port)
	log.Printf("Environment: %s", cfg.AppEnv)
	log.Printf("Database target: %s", cfg.DescribeDatabaseTarget())
	log.Printf("Trust proxy: %v", cfg.TrustProxy())

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: r,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log.Printf("Received signal %v, shutting down gracefully...", sig)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited gracefully")
}

func corsMiddleware(origin string) gin.HandlerFunc {
	return func(c *gin.Context) {
		allowOrigin := origin
		if allowOrigin == "" {
			allowOrigin = "*"
		}
		c.Header("Access-Control-Allow-Origin", allowOrigin)
		c.Header("Access-Control-Allow-Methods", "GET,HEAD,PUT,PATCH,POST,DELETE")
		c.Header("Access-Control-Allow-Headers", "Authorization,Content-Type,X-Distinct-Id,X-Distinct-ID,X-API-Key,X-Author-Identity")
		// Browsers reject Allow-Credentials: true combined with Allow-Origin: *,
		// so only set credentials when the origin is explicitly pinned.
		if allowOrigin != "*" {
			c.Header("Access-Control-Allow-Credentials", "true")
		}

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func defaultTokenSubject(cfg *config.Config) auth.TokenSubject {
	return auth.TokenSubject{
		Sub:           cfg.DefaultUserID,
		Email:         cfg.DefaultUserEmail,
		Name:          cfg.DefaultUserName,
		PersonalOrgID: cfg.DefaultPersonalOrgID,
		Role:          cfg.DefaultUserRole,
		Orgs: []auth.Org{
			{
				OrgID:   cfg.DefaultPersonalOrgID,
				OrgName: cfg.DefaultOrgName,
				OrgSlug: cfg.DefaultOrgSlug,
				Role:    cfg.DefaultUserRole,
			},
		},
	}
}

func adminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := c.Get("user")
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Authorization required"})
			return
		}
		userMap, ok := user.(gin.H)
		if !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Forbidden"})
			return
		}
		role, _ := userMap["role"].(string)
		if role != "admin" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
			return
		}
		c.Next()
	}
}

func bootstrapAdminUser(ctx context.Context, userSvc *service.UserService, cfg *config.Config) {
	count, err := userSvc.UserCount(ctx)
	if err != nil {
		log.Printf("Warning: could not check user count: %v", err)
		return
	}
	if count > 0 {
		return
	}

	password := cfg.InitialAdminPassword
	if password == "" {
		log.Println("No users exist and INITIAL_ADMIN_PASSWORD is not set. Skipping admin bootstrap.")
		return
	}

	username := cfg.InitialAdminUsername
	if username == "" {
		username = "admin"
	}

	hash, err := service.HashPassword(password)
	if err != nil {
		log.Printf("Warning: failed to hash admin password: %v", err)
		return
	}

	user := &model.User{
		Username:     username,
		DisplayName:  username,
		PasswordHash: hash,
		Role:         "admin",
		Status:       model.UserStatusEnabled,
	}
	if err := userSvc.Create(ctx, user); err != nil {
		log.Printf("Warning: failed to create initial admin user: %v", err)
		return
	}

	log.Printf("Initial admin user '%s' created successfully", username)
}
