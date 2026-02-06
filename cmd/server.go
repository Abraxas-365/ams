package main

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	appcontext "github.com/Abraxas-365/ams/context"
	"github.com/Abraxas-365/ams/manifest"
	"github.com/Abraxas-365/ams/orchestator"
	"github.com/Abraxas-365/ams/pkg/ai/llm"
	"github.com/Abraxas-365/ams/pkg/ai/llm/memoryx/memoryinfra"
	"github.com/Abraxas-365/ams/pkg/ai/llm/memoryx/memorysrv"
	aiopenai "github.com/Abraxas-365/ams/pkg/ai/providers/openai"
	"github.com/Abraxas-365/ams/pkg/config"
	"github.com/Abraxas-365/ams/pkg/errx"
	"github.com/Abraxas-365/ams/pkg/logx"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

func main() {
	// 1. Load Configuration
	cfg, err := config.Load()
	if err != nil {
		logx.Fatalf("Failed to load configuration: %v", err)
	}

	// 2. Initialize Logger
	initLogger(cfg)

	logx.Info("🚀 Starting Manifesto AI Orchestrator...")
	logx.Infof("Environment: %s", cfg.Server.Environment)

	// 3. Initialize Core Dependencies

	// --- A. Database (Optional) ---
	db, err := initDatabase()
	if err != nil {
		logx.Warnf("⚠️ Database not available: %v", err)
		logx.Info("ℹ️ Running without session persistence (buffer memory only)")
		db = nil
	} else {
		logx.Info("✅ Database connected successfully")
	}

	// --- B. AI Client (OpenAI) ---
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		logx.Warn("⚠️ OPENAI_API_KEY not set. AI features may fail.")
	}
	openaiProvider := aiopenai.NewOpenAIProvider(apiKey)
	llmClient := llm.NewClient(openaiProvider)

	// --- C. Manifest Registry ---
	manifestReg := manifest.NewRegistry()
	manifestPath := os.Getenv("MANIFEST_PATH")
	if manifestPath == "" {
		manifestPath = "manifest.yaml"
	}

	if err := manifestReg.LoadFromFile(manifestPath); err != nil {
		logx.Fatalf("❌ Failed to load manifest file: %v", err)
	}
	logx.Infof("✅ Manifest loaded from %s (Routes: %d)", manifestPath, len(manifestReg.ListRoutes()))

	// --- D. Session Service (if DB available) ---
	var sessionService *memorysrv.SessionService
	if db != nil {
		sessionRepo := memoryinfra.NewPostgresSessionRepository(db)
		sessionService = memorysrv.NewSessionService(sessionRepo)
		logx.Info("✅ Session service initialized (database-backed memory)")
	} else {
		logx.Info("ℹ️ Session service disabled (using buffer memory only)")
	}

	// --- E. Context & Orchestrator ---
	providerLoader := appcontext.NewProviderLoader()
	contextBuilder := appcontext.NewBuilder(providerLoader)

	orchConfig := orchestator.Config{
		LLMClient:      *llmClient,
		ContextBuilder: contextBuilder,
		ManifestReg:    manifestReg,
		MemoryFactory:  orchestator.NewBufferMemoryFactory(),
		SessionService: sessionService,
	}

	orch := orchestator.NewOrchestrator(orchConfig)

	// 4. Create Fiber App
	app := fiber.New(fiber.Config{
		AppName:               "Manifesto Orchestrator",
		DisableStartupMessage: true,
		ErrorHandler:          globalErrorHandler(cfg),
		BodyLimit:             10 * 1024 * 1024,
		IdleTimeout:           120 * time.Second,
	})

	// 5. Middleware
	setupMiddleware(app, cfg)

	// 6. Routes
	registerRoutes(app, orch)

	// 7. Start Server
	startServer(app, cfg)
}

// ============================================================================
// Database Initialization
// ============================================================================

func initDatabase() (*sqlx.DB, error) {
	host := os.Getenv("DB_HOST")
	if host == "" {
		return nil, fmt.Errorf("DB_HOST environment variable not set")
	}

	port := os.Getenv("DB_PORT")
	if port == "" {
		port = "5432"
	}

	user := os.Getenv("DB_USER")
	if user == "" {
		return nil, fmt.Errorf("DB_USER environment variable not set")
	}

	password := os.Getenv("DB_PASSWORD")
	if password == "" {
		return nil, fmt.Errorf("DB_PASSWORD environment variable not set")
	}

	dbname := os.Getenv("DB_NAME")
	if dbname == "" {
		return nil, fmt.Errorf("DB_NAME environment variable not set")
	}

	sslMode := os.Getenv("DB_SSL_MODE")
	if sslMode == "" {
		sslMode = "disable"
	}

	connStr := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, password, dbname, sslMode,
	)

	logx.WithFields(logx.Fields{
		"host": host,
		"port": port,
		"user": user,
		"db":   dbname,
	}).Debug("Connecting to database")

	db, err := sqlx.Connect("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	maxOpenConns := 25
	maxIdleConns := 5
	connMaxLifetime := 5 * time.Minute

	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxIdleConns)
	db.SetConnMaxLifetime(connMaxLifetime)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	logx.WithFields(logx.Fields{
		"max_open_conns": maxOpenConns,
		"max_idle_conns": maxIdleConns,
		"conn_lifetime":  connMaxLifetime,
	}).Info("Database connection pool configured")

	return db, nil
}

// ============================================================================
// Helper Functions
// ============================================================================

// generateAnonymousID creates a unique identifier for anonymous users
func generateAnonymousID() string {
	return fmt.Sprintf("anon_%s", uuid.New().String())
}

// setupAnonymousUser sets up user context for anonymous/unauthenticated requests
func setupAnonymousUser(req *orchestator.ChatRequest) {
	req.BearerToken = ""

	// Initialize Frontend context if nil
	if req.Frontend == nil {
		req.Frontend = &appcontext.FrontendContext{}
	}

	// Get or generate anonymous ID from frontend context
	anonymousID := req.Frontend.AnonymousID
	if anonymousID == "" {
		anonymousID = generateAnonymousID()
		req.Frontend.AnonymousID = anonymousID
		logx.WithField("generated_id", anonymousID).Debug("Generated new anonymous ID")
	}

	// Create anonymous user with unique ID
	if req.User == nil {
		req.User = &appcontext.User{
			ID:    anonymousID,
			Email: fmt.Sprintf("%s@anonymous.local", anonymousID),
			Name:  "Anonymous User",
		}
	}
}

// ============================================================================
// Routes
// ============================================================================

func registerRoutes(app *fiber.App, orch *orchestator.Orchestrator) {
	// Health check
	app.Get("/health", func(c *fiber.Ctx) error {
		health := fiber.Map{
			"status": "healthy",
			"mode":   "orchestrator",
		}

		if err := orch.Health(c.Context()); err != nil {
			health["status"] = "degraded"
			health["error"] = err.Error()
		}

		return c.JSON(health)
	})

	// List available routes
	app.Get("/routes", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"routes": orch.ListRoutes(),
			"stats":  orch.Stats(),
		})
	})

	// ========================================================================
	// Chat Endpoints
	// ========================================================================

	// 1. Standard Chat
	app.Post("/api/v1/chat", func(c *fiber.Ctx) error {
		var req orchestator.ChatRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid request body",
			})
		}

		// Setup anonymous user with unique ID
		setupAnonymousUser(&req)

		response, err := orch.HandleChat(c.Context(), req)
		if err != nil {
			return err
		}

		// Return anonymous_id back to client
		return c.JSON(fiber.Map{
			"response":     response,
			"anonymous_id": req.Frontend.AnonymousID,
			"session_id":   response.SessionID,
		})
	})

	// 2. Streaming Chat
	app.Post("/api/v1/chat/stream", func(c *fiber.Ctx) error {
		var req orchestator.ChatRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid request body",
			})
		}

		// Setup anonymous user with unique ID
		setupAnonymousUser(&req)
		anonymousID := req.Frontend.AnonymousID

		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")
		c.Set("Transfer-Encoding", "chunked")

		c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
			// Send anonymous_id as first event
			fmt.Fprintf(w, "event: init\ndata: {\"anonymous_id\":\"%s\"}\n\n", anonymousID)
			_ = w.Flush()

			_ = orch.HandleChatStream(c.Context(), req, func(chunk orchestator.StreamChunk) {
				if chunk.Error != "" {
					fmt.Fprintf(w, "event: error\ndata: {\"error\":\"%s\"}\n\n", chunk.Error)
				} else if chunk.Done {
					if chunk.SessionID != "" {
						fmt.Fprintf(w, "event: done\ndata: {\"session_id\":\"%s\",\"anonymous_id\":\"%s\"}\n\n",
							chunk.SessionID, anonymousID)
					} else {
						fmt.Fprintf(w, "event: done\ndata: {\"anonymous_id\":\"%s\"}\n\n", anonymousID)
					}
				} else {
					fmt.Fprintf(w, "event: message\ndata: %s\n\n", chunk.Content)
				}
				_ = w.Flush()
			})
		})
		return nil
	})

	// ========================================================================
	// Session Management Endpoints
	// ========================================================================

	sessionAPI := app.Group("/api/v1/sessions")

	// Create a new session
	sessionAPI.Post("/", func(c *fiber.Ctx) error {
		type CreateSessionRequest struct {
			AnonymousID string                      `json:"anonymous_id"`           // From frontend
			Title       string                      `json:"title"`                  // Session title
			RoutePath   string                      `json:"route_path"`             // Route pattern
			RouteParams map[string]string           `json:"route_params,omitempty"` // ✅ Optional route params
			Frontend    *appcontext.FrontendContext `json:"frontend,omitempty"`     // ✅ Optional frontend context
		}

		var req CreateSessionRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid request body",
			})
		}

		// Validate or generate anonymous ID
		anonymousID := req.AnonymousID
		if anonymousID == "" {
			anonymousID = generateAnonymousID()
			logx.Debug("Generated anonymous ID for session creation")
		}

		// Default values
		if req.Title == "" {
			req.Title = "New Chat Session"
		}
		if req.RoutePath == "" {
			req.RoutePath = "/"
		}

		// ✅ Create session with optional route params and frontend context
		sessionID, err := orch.CreateSessionWithContext(
			c.Context(),
			anonymousID,
			req.Title,
			req.RoutePath,
			req.RouteParams, // Frontend sends this when it has route data
			req.Frontend,    // Frontend sends accessibility/viewport data
		)
		if err != nil {
			logx.WithError(err).Error("Failed to create session")
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to create session",
			})
		}

		return c.Status(fiber.StatusCreated).JSON(fiber.Map{
			"session_id":   sessionID,
			"anonymous_id": anonymousID,
			"title":        req.Title,
		})
	})

	// List sessions for an anonymous user
	sessionAPI.Get("/", func(c *fiber.Ctx) error {
		anonymousID := c.Query("anonymous_id")
		if anonymousID == "" {
			anonymousID = c.Get("X-Anonymous-ID")
		}

		if anonymousID == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "anonymous_id is required (query param or X-Anonymous-ID header)",
			})
		}

		limit := c.QueryInt("limit", 20)
		offset := c.QueryInt("offset", 0)

		sessions, err := orch.ListUserSessions(c.Context(), anonymousID, limit, offset)
		if err != nil {
			logx.WithError(err).Error("Failed to list sessions")
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to list sessions",
			})
		}

		return c.JSON(fiber.Map{
			"sessions":     sessions,
			"count":        len(sessions),
			"limit":        limit,
			"offset":       offset,
			"anonymous_id": anonymousID,
		})
	})

	// Get session by ID
	sessionAPI.Get("/:session_id", func(c *fiber.Ctx) error {
		sessionID := c.Params("session_id")

		session, err := orch.GetSession(c.Context(), sessionID)
		if err != nil {
			logx.WithError(err).Error("Failed to get session")
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Session not found",
			})
		}

		return c.JSON(session)
	})

	// Get session with messages
	sessionAPI.Get("/:session_id/messages", func(c *fiber.Ctx) error {
		sessionID := c.Params("session_id")

		sessionWithMessages, err := orch.GetSessionWithMessages(c.Context(), sessionID)
		if err != nil {
			logx.WithError(err).Error("Failed to get session with messages")
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Session not found",
			})
		}

		return c.JSON(sessionWithMessages)
	})

	// Delete session
	sessionAPI.Delete("/:session_id", func(c *fiber.Ctx) error {
		sessionID := c.Params("session_id")

		if err := orch.DeleteSession(c.Context(), sessionID); err != nil {
			logx.WithError(err).Error("Failed to delete session")
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to delete session",
			})
		}

		return c.SendStatus(fiber.StatusNoContent)
	})

	// Utility: Generate new anonymous ID
	app.Get("/api/v1/anonymous-id", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"anonymous_id": generateAnonymousID(),
		})
	})
}

// ============================================================================
// Setup & Configuration
// ============================================================================

func initLogger(cfg *config.Config) {
	switch cfg.Server.LogLevel {
	case "debug":
		logx.SetLevel(logx.LevelDebug)
	case "trace":
		logx.SetLevel(logx.LevelTrace)
	case "warn":
		logx.SetLevel(logx.LevelWarn)
	case "error":
		logx.SetLevel(logx.LevelError)
	default:
		logx.SetLevel(logx.LevelInfo)
	}

	logx.WithField("level", cfg.Server.LogLevel).Info("Logger initialized")
}

func setupMiddleware(app *fiber.App, cfg *config.Config) {
	// Recover from panics
	app.Use(recover.New(recover.Config{
		EnableStackTrace: cfg.IsDevelopment(),
	}))

	// Request ID
	app.Use(requestid.New(requestid.Config{
		Header: "X-Request-ID",
	}))

	// CORS
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept, Authorization, X-Request-ID, X-Anonymous-ID",
		AllowMethods: "GET, POST, PUT, DELETE, OPTIONS",
	}))

	// Request logging
	app.Use(logger.New(logger.Config{
		Format:     "${time} | ${status} | ${latency} | ${method} ${path}\n",
		TimeFormat: "15:04:05",
		TimeZone:   "Local",
	}))
}

func globalErrorHandler(cfg *config.Config) fiber.ErrorHandler {
	return func(c *fiber.Ctx, err error) error {
		logx.WithFields(logx.Fields{
			"path":       c.Path(),
			"method":     c.Method(),
			"request_id": c.Get("X-Request-ID"),
		}).Errorf("Request error: %v", err)

		if e, ok := err.(*errx.Error); ok {
			return c.Status(e.HTTPStatus).JSON(fiber.Map{
				"error":  e.Message,
				"code":   e.Code,
				"status": e.HTTPStatus,
			})
		}

		if e, ok := err.(*fiber.Error); ok {
			return c.Status(e.Code).JSON(fiber.Map{
				"error": e.Message,
			})
		}

		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Internal Server Error",
		})
	}
}

func startServer(app *fiber.App, cfg *config.Config) {
	port := fmt.Sprintf("%d", cfg.Server.Port)

	go func() {
		logx.Infof("🚀 Server listening on port %s", port)
		logx.Infof("📡 Health check: http://localhost:%s/health", port)
		logx.Infof("📋 Routes list: http://localhost:%s/routes", port)
		if err := app.Listen(":" + port); err != nil {
			logx.Fatalf("Server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	logx.Info("🛑 Shutting down server...")

	if err := app.ShutdownWithTimeout(10 * time.Second); err != nil {
		logx.Errorf("Server forced to shutdown: %v", err)
	}

	logx.Info("✅ Server exited gracefully")
}
