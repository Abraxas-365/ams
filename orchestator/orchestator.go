package orchestator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Abraxas-365/ams/manifest"
	"github.com/Abraxas-365/ams/pkg/ai/llm"
	"github.com/Abraxas-365/ams/pkg/ai/llm/agentx"
	"github.com/Abraxas-365/ams/pkg/ai/llm/memoryx"
	"github.com/Abraxas-365/ams/pkg/ai/llm/memoryx/memorysrv"
	"github.com/Abraxas-365/ams/pkg/ai/llm/toolx"
	"github.com/Abraxas-365/ams/pkg/logx"
	"github.com/Abraxas-365/ams/tools"

	appcontext "github.com/Abraxas-365/ams/context"
)

// Orchestrator orchestrates the entire AI assistant flow
type Orchestrator struct {
	llmClient      llm.Client
	contextBuilder *appcontext.Builder
	manifestReg    *manifest.Registry
	toolLoader     *tools.ToolLoader
	memoryFactory  MemoryFactory
	sessionService *memorysrv.SessionService
}

// Config holds orchestrator configuration
type Config struct {
	LLMClient      llm.Client
	ContextBuilder *appcontext.Builder
	ManifestReg    *manifest.Registry
	MemoryFactory  MemoryFactory             // For backward compatibility (buffer memory)
	SessionService *memorysrv.SessionService // For session-based memory
}

// NewOrchestrator creates a new orchestrator
func NewOrchestrator(config Config) *Orchestrator {
	return &Orchestrator{
		llmClient:      config.LLMClient,
		contextBuilder: config.ContextBuilder,
		manifestReg:    config.ManifestReg,
		toolLoader:     tools.NewToolLoader(),
		memoryFactory:  config.MemoryFactory,
		sessionService: config.SessionService,
	}
}

// HandleChat processes a chat request and returns a response
// HandleChat processes a chat request and returns a response
func (o *Orchestrator) HandleChat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	// 1. Validate request
	if err := o.validateRequest(req); err != nil {
		return nil, err
	}

	// 2. Match route
	routeMatch, err := o.matchRoute(req.Route.Path, req.Route.Query)
	if err != nil {
		return nil, err
	}

	// ✅ 3. CRITICAL FIX: Override route params IMMEDIATELY if provided by frontend
	if len(req.RouteParams) > 0 {
		logx.WithFields(logx.Fields{
			"old_params": routeMatch.Params,
			"new_params": req.RouteParams,
			"route_path": req.Route.Path,
		}).Info("⚠️ OVERRIDING route params with frontend values")

		// IMPORTANT: Create NEW map to ensure clean override
		routeMatch.Params = make(map[string]string)
		for k, v := range req.RouteParams {
			routeMatch.Params[k] = v
			logx.WithFields(logx.Fields{
				"key":   k,
				"value": v,
			}).Debug("Setting route param")
		}

		logx.WithFields(logx.Fields{
			"final_params": routeMatch.Params,
		}).Info("✅ Route params overridden successfully")

		// Validate: Ensure no params contain route pattern syntax
		for k, v := range routeMatch.Params {
			if strings.HasPrefix(v, ":") {
				logx.WithFields(logx.Fields{
					"param": k,
					"value": v,
				}).Error("❌ Route param still contains pattern syntax!")
			}
		}
	} else {
		logx.WithFields(logx.Fields{
			"extracted_params": routeMatch.Params,
			"route_path":       req.Route.Path,
		}).Debug("No route_params provided by frontend, using extracted params")
	}

	// 4. Prepare user context (with token if provided)
	user := req.User
	if user != nil && req.BearerToken != "" {
		user.Token = req.BearerToken
	}

	// ✅ 5. Build fresh context ONLY if needed
	var fullContext *appcontext.FullContext
	shouldBuildContext := req.ShouldFetchContext || len(req.RouteParams) > 0

	if shouldBuildContext {
		logx.WithFields(logx.Fields{
			"route_params": routeMatch.Params,
			"route_name":   routeMatch.Route.Name,
		}).Info("Building fresh context with route params")

		fullContext, err = o.contextBuilder.Build(ctx, routeMatch, req.Frontend, user)
		if err != nil {
			return nil, NewContextBuildFailedError(err)
		}

		logx.WithFields(logx.Fields{
			"backend_keys": len(fullContext.Backend),
			"route_name":   fullContext.Route.Name,
			"route_params": fullContext.Route.Params,
		}).Info("Fresh context built successfully")
	} else {
		// Minimal context (no backend data)
		logx.Info("Using minimal context (no backend fetch)")
		fullContext, err = o.contextBuilder.BuildMinimal(routeMatch, user)
		if err != nil {
			return nil, NewContextBuildFailedError(err)
		}
	}

	// 6. Get or create memory (session-based or buffer)
	memory, sessionID, err := o.getOrCreateMemory(ctx, req, fullContext, routeMatch)
	if err != nil {
		return nil, err
	}

	// ✅ 7. Inject fresh backend context into existing session (if applicable)
	contextInjected := false
	if req.SessionID != "" && shouldBuildContext && len(fullContext.Backend) > 0 {
		logx.WithFields(logx.Fields{
			"session_id":   sessionID,
			"backend_keys": len(fullContext.Backend),
		}).Info("Injecting fresh backend context into existing session")

		// Create context injection message
		contextMsg := o.createContextInjectionMessage(fullContext)

		// Add it to memory (as a system message)
		if err := memory.Add(contextMsg); err != nil {
			logx.WithError(err).Warn("Failed to inject context message, continuing anyway")
		} else {
			contextInjected = true
			logx.Info("✅ Fresh context injected into session memory")
		}
	}

	// 8. Create agent with tools
	agent, err := o.createAgentWithMemory(ctx, memory, fullContext, routeMatch, req.BearerToken)
	if err != nil {
		return nil, err
	}

	// 9. Run agent
	response, err := agent.Run(ctx, req.Message)
	if err != nil {
		return nil, NewAgentExecutionFailedError(err)
	}

	// 10. Get usage information
	messages, _ := agent.Messages()
	usage := o.calculateUsage(messages)

	return &ChatResponse{
		Response:       response,
		SessionID:      sessionID,
		ConversationID: req.ConversationID,
		Usage:          usage,
		Metadata: map[string]any{
			"route":            routeMatch.Route.Name,
			"tools_count":      len(routeMatch.Route.Tools),
			"context_injected": contextInjected,
		},
	}, nil
}

// HandleChatStream processes a chat request with streaming
func (o *Orchestrator) HandleChatStream(
	ctx context.Context,
	req ChatRequest,
	streamHandler func(chunk StreamChunk),
) error {
	// 1. Validate request
	if err := o.validateRequest(req); err != nil {
		streamHandler(StreamChunk{
			Error: err.Error(),
			Done:  true,
		})
		return err
	}

	// 2. Match route
	routeMatch, err := o.matchRoute(req.Route.Path, req.Route.Query)
	if err != nil {
		streamHandler(StreamChunk{
			Error: err.Error(),
			Done:  true,
		})
		return err
	}

	// ✅ 3. Override route params if provided by frontend
	if len(req.RouteParams) > 0 {
		routeMatch.Params = req.RouteParams
		logx.WithFields(logx.Fields{
			"route_params": req.RouteParams,
			"route_path":   req.Route.Path,
		}).Info("Route params provided by frontend (streaming), will fetch fresh context")
	}

	// 4. Prepare user context
	user := req.User
	if user != nil && req.BearerToken != "" {
		user.Token = req.BearerToken
	}

	// ✅ 5. Build context conditionally
	var fullContext *appcontext.FullContext
	shouldBuildContext := req.ShouldFetchContext || len(req.RouteParams) > 0

	if shouldBuildContext {
		logx.Info("Building fresh context for streaming (route params or explicit request)")
		fullContext, err = o.contextBuilder.Build(ctx, routeMatch, req.Frontend, user)
		if err != nil {
			err = NewContextBuildFailedError(err)
			streamHandler(StreamChunk{
				Error: err.Error(),
				Done:  true,
			})
			return err
		}

		logx.WithFields(logx.Fields{
			"backend_keys": len(fullContext.Backend),
			"route_name":   fullContext.Route.Name,
		}).Info("Fresh context built for streaming")
	} else {
		logx.Info("Using minimal context for streaming (no backend fetch)")
		fullContext, err = o.contextBuilder.BuildMinimal(routeMatch, user)
		if err != nil {
			err = NewContextBuildFailedError(err)
			streamHandler(StreamChunk{
				Error: err.Error(),
				Done:  true,
			})
			return err
		}
	}

	// 6. Get or create memory (session-based or buffer)
	memory, sessionID, err := o.getOrCreateMemory(ctx, req, fullContext, routeMatch)
	if err != nil {
		streamHandler(StreamChunk{
			Error: err.Error(),
			Done:  true,
		})
		return err
	}

	// ✅ 7. Inject fresh context
	contextInjected := false
	if req.SessionID != "" && shouldBuildContext && len(fullContext.Backend) > 0 {
		logx.WithFields(logx.Fields{
			"session_id":   sessionID,
			"backend_keys": len(fullContext.Backend),
		}).Info("Injecting fresh backend context into existing session (streaming)")

		contextMsg := o.createContextInjectionMessage(fullContext)
		if err := memory.Add(contextMsg); err != nil {
			logx.WithError(err).Warn("Failed to inject context message (streaming), continuing anyway")
		} else {
			contextInjected = true
		}
	}

	// 8. Create agent with tools
	agent, err := o.createAgentWithMemory(ctx, memory, fullContext, routeMatch, req.BearerToken)
	if err != nil {
		streamHandler(StreamChunk{
			Error: err.Error(),
			Done:  true,
		})
		return err
	}

	// 9. Stream agent response
	err = agent.StreamWithTools(ctx, req.Message, func(chunk string) {
		streamHandler(StreamChunk{
			Content: chunk,
			Done:    false,
		})
	})

	if err != nil {
		streamHandler(StreamChunk{
			Error: err.Error(),
			Done:  true,
		})
		return NewAgentExecutionFailedError(err)
	}

	// Send final chunk
	streamHandler(StreamChunk{
		Done:      true,
		SessionID: sessionID,
		Metadata: map[string]any{
			"route":            routeMatch.Route.Name,
			"context_injected": contextInjected,
		},
	})

	return nil
}

// ✅ NEW: createContextInjectionMessage creates a system message with fresh backend data
func (o *Orchestrator) createContextInjectionMessage(fullContext *appcontext.FullContext) llm.Message {
	var sb strings.Builder

	sb.WriteString("=== UPDATED CONTEXT FOR CURRENT ROUTE ===\n\n")

	// Route info
	fmt.Fprintf(&sb, "Current Route: %s (%s)\n", fullContext.Route.Name, fullContext.Route.Path)
	if len(fullContext.Route.Params) > 0 {
		fmt.Fprintf(&sb, "Parameters: %v\n", fullContext.Route.Params)
	}
	sb.WriteString("\n")

	// Backend data (the fresh part!)
	if len(fullContext.Backend) > 0 {
		sb.WriteString("=== FRESH BACKEND DATA ===\n\n")
		for key, value := range fullContext.Backend {
			fmt.Fprintf(&sb, "%s:\n", key)
			jsonData, _ := json.MarshalIndent(value, "", "  ")
			sb.WriteString(string(jsonData))
			sb.WriteString("\n\n")
		}
	}

	sb.WriteString("Use this fresh data to answer the user's question.\n")
	sb.WriteString("=== END UPDATED CONTEXT ===\n")

	return llm.NewSystemMessage(sb.String())
}

// getOrCreateMemory gets existing session memory or creates new one
func (o *Orchestrator) getOrCreateMemory(
	ctx context.Context,
	req ChatRequest,
	fullContext *appcontext.FullContext,
	routeMatch *manifest.RouteMatch,
) (memoryx.Memory, string, error) {
	// If session service is available and session ID is provided, use session memory
	if o.sessionService != nil && req.SessionID != "" {
		logx.WithField("session_id", req.SessionID).Debug("Using existing session")

		sessionID := memoryx.SessionID(req.SessionID)
		memory, err := o.sessionService.GetSessionMemory(ctx, sessionID)
		if err != nil {
			logx.WithError(err).Error("Failed to get session memory")
			return nil, "", err
		}

		return memory, string(sessionID), nil
	}

	// If session service is available but no session ID, create new session
	if o.sessionService != nil {
		logx.Info("Creating new session")

		// Create system message from fullContext
		systemMsg := fullContext.ToSystemMessage()

		logx.WithFields(logx.Fields{
			"message_length":      len(systemMsg.Content),
			"has_backend_section": strings.Contains(systemMsg.Content, "=== BACKEND DATA ==="),
		}).Debug("System message created for new session")

		userID := "guest"
		if req.User != nil {
			userID = req.User.ID
		}

		// Create session with system message
		session, err := o.sessionService.CreateSession(
			ctx,
			userID,
			"Chat - "+routeMatch.Route.Name,
			systemMsg,
		)
		if err != nil {
			logx.WithError(err).Error("Failed to create session")
			return nil, "", err
		}

		logx.WithFields(logx.Fields{
			"session_id":     session.ID,
			"user_id":        userID,
			"message_length": len(systemMsg.Content),
		}).Info("✅ New session created and saved to database")

		// Get session memory
		memory, err := o.sessionService.GetSessionMemory(ctx, session.ID)
		if err != nil {
			logx.WithError(err).Error("Failed to get session memory for new session")
			return nil, "", err
		}

		return memory, string(session.ID), nil
	}

	// Fallback to buffer memory (backward compatibility)
	logx.Debug("Using buffer memory (no session service)")

	if o.memoryFactory == nil {
		logx.Warn("No memory factory configured, creating default")
		o.memoryFactory = NewBufferMemoryFactory()
	}

	memory := o.memoryFactory.Create(fullContext.ToSystemMessage())
	return memory, "", nil
}

// validateRequest validates the incoming request
func (o *Orchestrator) validateRequest(req ChatRequest) error {
	if req.Message == "" {
		return NewMissingMessageError()
	}

	if req.Route.Path == "" {
		return NewMissingRouteError()
	}

	return nil
}

// matchRoute matches the route and returns the route match
func (o *Orchestrator) matchRoute(path string, query map[string]string) (*manifest.RouteMatch, error) {
	match, err := o.manifestReg.GetRouteContext(path, query)
	if err != nil {
		return nil, NewRouteMatchFailedError(path, err)
	}

	if match == nil {
		return nil, NewRouteNotFoundError(path)
	}

	return match, nil
}

// createAgent creates an agent for the given context and route (legacy)
func (o *Orchestrator) createAgent(
	ctx context.Context,
	fullContext *appcontext.FullContext,
	routeMatch *manifest.RouteMatch,
	userToken string,
) (*agentx.Agent, error) {
	// Create memory with context as system message
	var memory memoryx.Memory
	if o.memoryFactory != nil {
		memory = o.memoryFactory.Create(fullContext.ToSystemMessage())
	} else {
		memory = memoryx.NewBufferMemory(fullContext.ToSystemMessage())
	}

	return o.createAgentWithMemory(ctx, memory, fullContext, routeMatch, userToken)
}

// createAgentWithMemory creates an agent with provided memory
func (o *Orchestrator) createAgentWithMemory(
	ctx context.Context,
	memory memoryx.Memory,
	fullContext *appcontext.FullContext,
	routeMatch *manifest.RouteMatch,
	userToken string,
) (*agentx.Agent, error) {
	// 1. Build workflow context for tools
	workflowContext := o.buildWorkflowContext(fullContext, routeMatch)

	// 2. Load tools from manifest
	manifestTools, err := o.toolLoader.LoadFromRoute(
		routeMatch.Route,
		workflowContext,
		userToken,
	)
	if err != nil {
		return nil, NewToolLoadFailedError(err)
	}

	// 3. Create tool registry
	var toolRegistry *toolx.ToolxClient
	if len(manifestTools) > 0 {
		toolRegistry = toolx.FromToolx(manifestTools...)
	} else {
		// Empty tool registry
		toolRegistry = toolx.FromToolx()
	}

	// 4. Create agent options
	options := []agentx.AgentOption{
		agentx.WithTools(toolRegistry),
		agentx.WithOptions(
			llm.WithTemperature(1),
		),
	}

	// 5. Create and return agent
	agent := agentx.New(o.llmClient, memory, options...)

	return agent, nil
}

// buildWorkflowContext creates the workflow context for tools
func (o *Orchestrator) buildWorkflowContext(
	fullContext *appcontext.FullContext,
	routeMatch *manifest.RouteMatch,
) map[string]any {
	workflowContext := make(map[string]any)

	// Add route params (e.g., {id} from /products/:id)
	for key, value := range routeMatch.Params {
		workflowContext[key] = value
	}

	// Add query params
	for key, value := range routeMatch.Query {
		workflowContext[key] = value
	}

	// Add user context
	if fullContext.User != nil {
		workflowContext["user"] = map[string]any{
			"id":    fullContext.User.ID,
			"email": fullContext.User.Email,
			"name":  fullContext.User.Name,
			"token": fullContext.User.Token,
		}
	}

	// Add route info
	workflowContext["route"] = map[string]any{
		"name":   fullContext.Route.Name,
		"path":   fullContext.Route.Path,
		"params": fullContext.Route.Params,
		"query":  fullContext.Route.Query,
	}

	// Add backend data (from context providers)
	if len(fullContext.Backend) > 0 {
		workflowContext["backend"] = fullContext.Backend
		logx.WithField("backend_keys", len(fullContext.Backend)).Debug("Added backend to workflow context")
	} else {
		logx.Warn("⚠️ No backend data to add to workflow context")
	}

	return workflowContext
}

// calculateUsage calculates token usage from messages
func (o *Orchestrator) calculateUsage(messages []llm.Message) *UsageInfo {
	// Simple estimation - in production you'd track this from actual API responses
	totalTokens := 0
	for _, msg := range messages {
		// Rough estimate: 1 token ≈ 4 characters
		totalTokens += len(msg.Content) / 4
	}

	return &UsageInfo{
		PromptTokens:     totalTokens / 2,
		CompletionTokens: totalTokens / 2,
		TotalTokens:      totalTokens,
	}
}

// GetRouteInfo returns information about a specific route
func (o *Orchestrator) GetRouteInfo(path string) (*manifest.Route, error) {
	match, err := o.manifestReg.Match(path)
	if err != nil {
		return nil, NewRouteMatchFailedError(path, err)
	}

	if match == nil {
		return nil, NewRouteNotFoundError(path)
	}

	return match.Route, nil
}

// ListRoutes returns all available routes
func (o *Orchestrator) ListRoutes() []string {
	return o.manifestReg.ListRoutes()
}

// Health checks the health of the orchestrator
func (o *Orchestrator) Health(ctx context.Context) error {
	// Check if manifest is loaded
	if o.manifestReg.GetManifest() == nil {
		return fmt.Errorf("manifest not loaded")
	}

	// Check if LLM client is available
	if o.llmClient == (llm.Client{}) {
		return fmt.Errorf("LLM client not initialized")
	}

	return nil
}

// Stats returns orchestrator statistics
func (o *Orchestrator) Stats() map[string]any {
	manifestStats := o.manifestReg.Stats()

	return map[string]any{
		"manifest": manifestStats,
		"healthy":  o.Health(context.Background()) == nil,
	}
}

// ============================================================================
// Session Management Methods
// ============================================================================

// CreateSessionWithContext creates a new chat session with optional full context
// This allows the frontend to control whether backend data is fetched at session creation
func (o *Orchestrator) CreateSessionWithContext(
	ctx context.Context,
	userID string,
	title string,
	routePath string,
	routeParams map[string]string, // ✅ Optional route params from frontend
	frontendContext *appcontext.FrontendContext, // ✅ Optional frontend context
) (string, error) {
	if o.sessionService == nil {
		return "", fmt.Errorf("session service not configured")
	}

	// Match route to get context
	routeMatch, err := o.matchRoute(routePath, nil)
	if err != nil {
		return "", err
	}

	var fullContext *appcontext.FullContext

	// ✅ DECISION: If frontend sends route params, fetch full context with backend data
	if len(routeParams) > 0 {
		logx.WithFields(logx.Fields{
			"route_params": routeParams,
			"has_frontend": frontendContext != nil,
			"route_path":   routePath,
		}).Info("Creating session WITH full context (backend data will be fetched)")

		// Update routeMatch with params from frontend
		routeMatch.Params = routeParams

		// Build FULL context (fetches backend data from providers)
		fullContext, err = o.contextBuilder.Build(
			ctx,
			routeMatch,
			frontendContext,
			&appcontext.User{ID: userID},
		)
		if err != nil {
			return "", err
		}

		logx.WithFields(logx.Fields{
			"backend_keys": len(fullContext.Backend),
			"user_id":      userID,
		}).Info("✅ Session will be created WITH backend data")

	} else {
		// Frontend doesn't have params yet → Create minimal session
		logx.WithField("route_path", routePath).Info("Creating session WITHOUT backend data (minimal context)")

		fullContext, err = o.contextBuilder.BuildMinimal(
			routeMatch,
			&appcontext.User{ID: userID},
		)
		if err != nil {
			return "", err
		}

		logx.Info("⚠️ Session created without backend data (backend will be fetched on first chat message)")
	}

	// Create session with system message
	session, err := o.sessionService.CreateSession(
		ctx,
		userID,
		title,
		fullContext.ToSystemMessage(),
	)
	if err != nil {
		return "", err
	}

	logx.WithFields(logx.Fields{
		"session_id":   session.ID,
		"user_id":      userID,
		"title":        title,
		"backend_keys": len(fullContext.Backend),
		"has_backend":  len(fullContext.Backend) > 0,
	}).Info("Session created successfully via CreateSessionWithContext")

	return string(session.ID), nil
}

// CreateSession creates a new chat session (backward compatibility - uses BuildMinimal)
func (o *Orchestrator) CreateSession(ctx context.Context, userID, title string, routePath string) (string, error) {
	// Use new method with no params (minimal context)
	return o.CreateSessionWithContext(ctx, userID, title, routePath, nil, nil)
}

// ListUserSessions lists all sessions for a user
func (o *Orchestrator) ListUserSessions(ctx context.Context, userID string, limit, offset int) ([]*memoryx.Session, error) {
	if o.sessionService == nil {
		return nil, fmt.Errorf("session service not configured")
	}

	return o.sessionService.ListUserSessions(ctx, userID, limit, offset)
}

// DeleteSession deletes a session
func (o *Orchestrator) DeleteSession(ctx context.Context, sessionID string) error {
	if o.sessionService == nil {
		return fmt.Errorf("session service not configured")
	}

	return o.sessionService.DeleteSession(ctx, memoryx.SessionID(sessionID))
}

// GetSession gets a session by ID
func (o *Orchestrator) GetSession(ctx context.Context, sessionID string) (*memoryx.Session, error) {
	if o.sessionService == nil {
		return nil, fmt.Errorf("session service not configured")
	}

	return o.sessionService.GetSession(ctx, memoryx.SessionID(sessionID))
}

// GetSessionWithMessages gets a session with all messages
func (o *Orchestrator) GetSessionWithMessages(ctx context.Context, sessionID string) (*memoryx.SessionWithMessages, error) {
	if o.sessionService == nil {
		return nil, fmt.Errorf("session service not configured")
	}

	return o.sessionService.GetSessionWithMessages(ctx, memoryx.SessionID(sessionID))
}
