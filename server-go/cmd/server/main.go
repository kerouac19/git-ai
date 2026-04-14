package main

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"strings"
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

//go:embed templates/*.html
var templateFS embed.FS

var templates *template.Template

func init() {
	funcMap := template.FuncMap{
		"printf": fmt.Sprintf,
	}
	templates = template.Must(template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/*.html"))
}

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

	// Handlers
	loginH := &handler.LoginHandler{UserSvc: userSvc, JWTSecret: cfg.JWTSecret, IsProduction: isProduction}
	healthH := &handler.HealthHandler{Pool: pool}
	compatH := &handler.CompatibilityHandler{
		DashboardSvc:  dashboardSvc,
		AuthorshipSvc: authorshipSvc,
		CasSvc:        casSvc,
		DeviceFlowSvc: deviceFlowSvc,
		MetricsSvc:    metricsSvc,
	}
	authorshipH := &handler.AuthorshipHandler{Svc: authorshipSvc}
	bundleH := &handler.BundleHandler{Svc: bundleSvc}
	casH := &handler.CasHandler{Svc: casSvc}
	dashboardH := &handler.DashboardHandler{Svc: dashboardSvc}
	releaseH := &handler.ReleaseHandler{}
	sysConfigH := &handler.SysConfigHandler{Svc: sysConfigSvc}

	r := gin.Default()

	// Global middleware
	r.Use(middleware.SecurityHeadersMiddleware())
	if cfg.HTTPSRedirect {
		r.Use(middleware.HTTPSRedirectMiddleware())
	}
	r.Use(middleware.AuditMiddleware(pool))

	// CORS
	r.Use(corsMiddleware(cfg.CORSOrigin))

	jwtMW := auth.JWTAuthMiddleware(cfg.JWTSecret)
	workerMW := auth.WorkerAuthMiddleware(cfg.JWTSecret, cfg.ValidAPIKeys(), defaultTokenSubject(cfg))

	// --- Direct routes (no /api prefix) ---
	r.GET("/health", healthH.GetHealth)

	// Login page
	r.GET("/login", handleLoginPage())

	// OAuth Device Flow HTML pages
	r.GET("/oauth/device", handleDeviceFlowPage(deviceFlowSvc))
	r.POST("/oauth/device/approve", handleDeviceApprove(deviceFlowSvc, cfg.JWTSecret, isProduction))
	r.POST("/oauth/device/deny", handleDeviceDeny(deviceFlowSvc, isProduction))

	// /me dashboard page (cookie-based session)
	r.GET("/me", handleMePage(deviceFlowSvc, dashboardSvc))

	// --- Worker compatibility endpoints ---
	workerRoutes := []string{"worker", "workers"}
	for _, prefix := range workerRoutes {
		r.POST("/"+prefix+"/oauth/device/code", compatH.StartDeviceFlow)
		r.POST("/"+prefix+"/oauth/token", compatH.ExchangeOAuthToken)
		r.POST("/"+prefix+"/metrics/upload", workerMW, compatH.UploadWorkerMetrics)
		r.POST("/"+prefix+"/cas/upload", workerMW, compatH.UploadWorkerCas)
		r.GET("/"+prefix+"/cas", workerMW, compatH.ReadWorkerCas)
		r.GET("/"+prefix+"/cas/", workerMW, compatH.ReadWorkerCas)
		r.GET("/"+prefix+"/cas/checkout", workerMW, compatH.CheckoutWorkerCas)
		r.GET("/"+prefix+"/releases", releaseH.GetReleases)
		r.GET("/"+prefix+"/releases/:channel/download/:name", releaseH.Download)
	}

	// --- /api/* routes ---
	api := r.Group("/api")
	{
		api.GET("/health", healthH.GetAPIHealth)
		api.GET("/health/database", healthH.GetDatabaseHealth)
		api.GET("/status", compatH.GetStatus)
		api.GET("/version", compatH.GetVersion)
		api.GET("/me", jwtMW, compatH.GetMe)

		// User auth
		api.POST("/user/login", loginH.Login)
		api.GET("/user/logout", loginH.Logout)
		api.POST("/user/logout", loginH.Logout)
		api.POST("/user/register", jwtMW, adminOnly(), loginH.Register)
		api.POST("/bundles", bundleH.Create)

		// Authorship
		authorship := api.Group("/authorship")
		{
			authorship.POST("/record", authorshipH.SaveRecord)
			authorship.POST("/commit", authorshipH.SaveCommitAttribution)
			authorship.GET("/commits/:userId", authorshipH.GetUserCommits)
			authorship.GET("/commits/:userId/:commitHash", authorshipH.GetUserCommitByHash)
			authorship.GET("/commit/:commitHash", authorshipH.GetCommitAttribution)
			authorship.PUT("/sync/:userId", authorshipH.SyncAuthorship)
		}

		// CAS
		cas := api.Group("/cas")
		{
			cas.POST("/upload", casH.Upload)
			cas.GET("/read/:hash", casH.Read)
		}

		// Dashboard
		dashboard := api.Group("/dashboard")
		{
			dashboard.GET("/public", dashboardH.GetPublicStats)
			dashboard.GET("/stats", dashboardH.GetStats)
			dashboard.POST("/generate-report", dashboardH.GenerateReport)
		}

		// Config (JWT protected)
		cfgGroup := api.Group("/config", jwtMW)
		{
			cfgGroup.GET("", sysConfigH.GetAll)
			cfgGroup.GET("/:key", sysConfigH.GetByKey)
			cfgGroup.POST("", sysConfigH.Create)
			cfgGroup.PATCH("/:key", sysConfigH.Update)
			cfgGroup.DELETE("/:key", sysConfigH.Delete)
		}
	}

	// Startup logging
	port := cfg.Port
	log.Printf("Application is running on: http://localhost:%d", port)
	log.Printf("Environment: %s", cfg.AppEnv)
	log.Printf("Database target: %s", cfg.DescribeDatabaseTarget())
	log.Printf("Trust proxy: %v", cfg.TrustProxy())

	if err := r.Run(fmt.Sprintf(":%d", port)); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
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
		c.Header("Access-Control-Allow-Credentials", "true")

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

// --- HTML page handlers ---

type deviceFlowPageData struct {
	UserCode     string
	ExpiresAt    string
	Status       string
	SubjectName  string
	SubjectEmail string
}

type deviceResultPageData struct {
	Title    string
	Message  string
	Status   string
	LinkURL  string
	LinkText string
}

type dashboardPageData struct {
	Name                string
	Email               string
	Role                string
	OrgName             string
	OrgSlug             string
	Initial             string
	UserID              string
	AICodePercentage    float64
	TotalAddedLines     int
	CommittedAILines    int
	GeneratedAILines    int
	EditedAILines       int
	TopAgentLabel       string
	TopAgentCount       int
	TopModelLabel       string
	TopModelCount       int
	ActivePromptCount   int
	CheckpointFileCount int
	EventCount7d        int
	RepoCount7d         int
	LastSyncAt          string
	ActivityCount       int
	PromptCount         int
	FileCount           int
	TodayLastUpdatedAt  string
}

func handleDeviceFlowPage(svc *auth.DeviceFlowService) gin.HandlerFunc {
	return func(c *gin.Context) {
		userCode := c.Query("user_code")
		if userCode == "" {
			renderResult(c, http.StatusBadRequest, "Missing User Code", "No user_code query parameter was provided.", "error", "", "")
			return
		}

		entry, err := svc.GetDeviceCodeByUserCode(c.Request.Context(), userCode)
		if err != nil || entry == nil {
			renderResult(c, http.StatusNotFound, "Device Request Not Found", "The device code is missing, expired, or has already been completed.", "error", "", "")
			return
		}

		// Use logged-in user's info if available, otherwise fall back to device code subject
		subjectName := ""
		subjectEmail := ""
		if accessToken := auth.ExtractAccessTokenFromCookie(c.GetHeader("Cookie")); accessToken != "" {
			if claims, err := svc.DecodeAccessToken(accessToken); err == nil && claims.Subject != "" {
				subjectName = claims.Name
				subjectEmail = claims.Email
			}
		}
		if subjectName == "" && entry.Subject != nil {
			subjectName = entry.Subject.Name
			subjectEmail = entry.Subject.Email
		}

		// Format expiry time
		expiresAtStr := ""
		if entry.ExpiresAt > 0 {
			expiresAtStr = time.UnixMilli(entry.ExpiresAt).Format("2006-01-02 15:04:05 MST")
		}

		data := deviceFlowPageData{
			UserCode:     entry.UserCode,
			ExpiresAt:    expiresAtStr,
			Status:       entry.Status,
			SubjectName:  subjectName,
			SubjectEmail: subjectEmail,
		}
		c.Header("Content-Type", "text/html; charset=utf-8")
		if err := templates.ExecuteTemplate(c.Writer, "device_flow.html", data); err != nil {
			c.String(http.StatusInternalServerError, "Template error: %v", err)
		}
	}
}

func handleDeviceApprove(svc *auth.DeviceFlowService, jwtSecret string, isProduction bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check if user has an active browser session
		accessToken := auth.ExtractAccessTokenFromCookie(c.GetHeader("Cookie"))
		if accessToken == "" {
			userCode := c.PostForm("user_code")
			if userCode == "" {
				userCode = c.Query("user_code")
			}
			redirect := url.QueryEscape("/oauth/device?user_code=" + userCode)
			renderResult(c, http.StatusUnauthorized, "Login Required", "Please log in first before approving device authorization.", "error", "/login?redirect="+redirect, "Go to Login")
			return
		}

		claims, err := svc.DecodeAccessToken(accessToken)
		if err != nil || claims.Subject == "" {
			userCode := c.PostForm("user_code")
			if userCode == "" {
				userCode = c.Query("user_code")
			}
			redirect := url.QueryEscape("/oauth/device?user_code=" + userCode)
			renderResult(c, http.StatusUnauthorized, "Session Expired", "Your session has expired. Please log in again.", "error", "/login?redirect="+redirect, "Go to Login")
			return
		}

		userCode := c.PostForm("user_code")
		if userCode == "" {
			userCode = c.Query("user_code")
		}

		// Update device code with the real user's subject before approving
		realSubject := auth.TokenSubject{
			Sub:           claims.Subject,
			Email:         claims.Email,
			Name:          claims.Name,
			PersonalOrgID: claims.PersonalOrgID,
			Orgs:          claims.Orgs,
			Role:          claims.Role,
		}
		log.Printf("[DEBUG] Updating device code subject: userCode=%s, sub=%s, email=%s, name=%s", userCode, realSubject.Sub, realSubject.Email, realSubject.Name)
		if err := svc.UpdateDeviceCodeSubject(c.Request.Context(), userCode, realSubject); err != nil {
			log.Printf("[ERROR] Failed to update device code subject: %v", err)
			renderResult(c, http.StatusInternalServerError, "Error", fmt.Sprintf("Failed to update device authorization: %v", err), "error", "", "")
			return
		}
		log.Printf("[DEBUG] Successfully updated device code subject for userCode=%s", userCode)

		entry, err := svc.ApproveDeviceCode(c.Request.Context(), userCode)
		if err != nil || entry == nil {
			renderResult(c, http.StatusNotFound, "Device Request Not Found", "The device code is missing, expired, or has already been completed.", "error", "", "")
			return
		}

		if entry.Status == "denied" {
			renderResult(c, http.StatusConflict, "Authorization Denied", "This device request was already denied and cannot be approved anymore.", "error", "", "")
			return
		}

		renderResult(c, http.StatusOK, "Device Approved", "CLI authorization has been approved. This browser session is now signed in.", "ok", "/me", "Open Dashboard")
	}
}

func handleDeviceDeny(svc *auth.DeviceFlowService, isProduction bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userCode := c.PostForm("user_code")
		if userCode == "" {
			userCode = c.Query("user_code")
		}

		entry, err := svc.DenyDeviceCode(c.Request.Context(), userCode)
		if err != nil || entry == nil {
			renderResult(c, http.StatusNotFound, "Device Request Not Found", "The device code is missing, expired, or has already been completed.", "error", "", "")
			return
		}

		c.Header("Set-Cookie", auth.ClearSessionCookie(isProduction))
		renderResult(c, http.StatusOK, "Device Denied", "CLI authorization was denied. You can close this tab and retry git-ai login later.", "error", "", "")
	}
}

func handleMePage(svc *auth.DeviceFlowService, dashSvc *service.DashboardService) gin.HandlerFunc {
	return func(c *gin.Context) {
		accessToken := auth.ExtractAccessTokenFromCookie(c.GetHeader("Cookie"))
		if accessToken == "" {
			renderLoginRequired(c)
			return
		}

		claims, err := svc.DecodeAccessToken(accessToken)
		if err != nil || claims.Subject == "" {
			renderLoginRequired(c)
			return
		}

		dashboard, _ := dashSvc.GetDashboardStats(c.Request.Context(), claims.Subject)

		data := buildDashboardPageData(claims, dashboard)
		c.Header("Content-Type", "text/html; charset=utf-8")
		if err := templates.ExecuteTemplate(c.Writer, "dashboard.html", data); err != nil {
			c.String(http.StatusInternalServerError, "Template error: %v", err)
		}
	}
}

func buildDashboardPageData(claims *auth.Claims, dashboard map[string]interface{}) dashboardPageData {
	name := claims.Name
	if name == "" {
		name = claims.Email
	}

	orgName := ""
	orgSlug := ""
	if len(claims.Orgs) > 0 {
		orgName = claims.Orgs[0].OrgName
		orgSlug = claims.Orgs[0].OrgSlug
	}

	initial := "?"
	if name != "" {
		initial = strings.ToUpper(name[:1])
	}

	data := dashboardPageData{
		Name:    name,
		Email:   claims.Email,
		Role:    claims.Role,
		OrgName: orgName,
		OrgSlug: orgSlug,
		Initial: initial,
		UserID:  claims.Subject,
	}

	if dashboard == nil {
		return data
	}

	if aiCode, ok := dashboard["aiCode"].(map[string]interface{}); ok {
		data.AICodePercentage = math.Round(toFloat(aiCode["percentage"])*10) / 10
		data.TotalAddedLines = toInt(aiCode["totalAddedLines"])
		data.CommittedAILines = toInt(aiCode["committedAiLines"])
	}
	if leaders, ok := dashboard["leaders"].(map[string]interface{}); ok {
		if ta, ok := leaders["topAgent"].(map[string]interface{}); ok {
			data.TopAgentLabel = toString(ta["label"])
			data.TopAgentCount = toInt(ta["promptCount"])
		}
		if tm, ok := leaders["topModel"].(map[string]interface{}); ok {
			data.TopModelLabel = toString(tm["label"])
			data.TopModelCount = toInt(tm["promptCount"])
		}
	}
	if aiOutput, ok := dashboard["aiOutput"].(map[string]interface{}); ok {
		data.GeneratedAILines = toInt(aiOutput["generated"])
		data.EditedAILines = toInt(aiOutput["edited"])
	}
	if activity, ok := dashboard["activity"].(map[string]interface{}); ok {
		data.ActivePromptCount = toInt(activity["activePromptCount"])
		data.CheckpointFileCount = toInt(activity["checkpointFileCount"])
	}
	if ms, ok := dashboard["metricsSummary"].(*model.MetricsSummary); ok && ms != nil {
		data.EventCount7d = ms.EventCount7d
		data.RepoCount7d = ms.RepoCount7d
		if ms.LastSyncAt != nil {
			data.LastSyncAt = *ms.LastSyncAt
		}
	}
	if today, ok := dashboard["today"].(map[string]interface{}); ok {
		data.ActivityCount = toInt(today["activityCount"])
		data.PromptCount = toInt(today["promptCount"])
		data.FileCount = toInt(today["fileCount"])
		data.TodayLastUpdatedAt = toString(today["lastUpdatedAt"])
	}

	return data
}

func renderResult(c *gin.Context, status int, title, message, resultStatus, linkURL, linkText string) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Status(status)
	data := deviceResultPageData{
		Title:    title,
		Message:  message,
		Status:   resultStatus,
		LinkURL:  linkURL,
		LinkText: linkText,
	}
	if err := templates.ExecuteTemplate(c.Writer, "device_result.html", data); err != nil {
		c.String(http.StatusInternalServerError, "Template error: %v", err)
	}
}

func renderLoginRequired(c *gin.Context) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Status(http.StatusUnauthorized)
	if err := templates.ExecuteTemplate(c.Writer, "login_required.html", nil); err != nil {
		c.String(http.StatusInternalServerError, "Template error: %v", err)
	}
}

func toFloat(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return 0
	}
}

func toInt(v interface{}) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	if s, ok := v.(*string); ok && s != nil {
		return *s
	}
	return fmt.Sprintf("%v", v)
}

func handleLoginPage() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Content-Type", "text/html; charset=utf-8")
		if err := templates.ExecuteTemplate(c.Writer, "login.html", nil); err != nil {
			c.String(http.StatusInternalServerError, "Template error: %v", err)
		}
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

// suppress unused import warnings
var _ = os.Getenv
