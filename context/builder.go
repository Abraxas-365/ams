package context

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"sync"

	"github.com/Abraxas-365/ams/manifest"
	"github.com/Abraxas-365/ams/pkg/logx"
)

// Builder builds complete context for the agent
type Builder struct {
	loader *ProviderLoader
}

// NewBuilder creates a new context builder
func NewBuilder(loader *ProviderLoader) *Builder {
	logx.Debug("Context builder created")
	return &Builder{loader: loader}
}

// Build constructs the complete context
func (b *Builder) Build(
	ctx context.Context,
	routeMatch *manifest.RouteMatch,
	frontendContext *FrontendContext,
	user *User,
) (*FullContext, error) {
	logx.WithFields(logx.Fields{
		"route_name": getRouteName(routeMatch),
		"has_user":   user != nil,
	}).Info("Building full context")

	if routeMatch == nil {
		logx.Error("Route match is nil")
		return nil, NewInvalidRouteMatchError()
	}

	if routeMatch.Route == nil {
		logx.Error("Route config is missing")
		return nil, NewMissingRouteConfigError()
	}

	fullContext := &FullContext{
		Route: RouteInfo{
			Path:   routeMatch.Route.Pattern,
			Name:   routeMatch.Route.Name,
			Params: routeMatch.Params,
			Query:  routeMatch.Query,
		},
		User:           user,
		Frontend:       frontendContext,
		Backend:        make(map[string]any),
		Instructions:   routeMatch.Route.AgentInstructions,
		AvailableTools: extractToolNames(routeMatch.Route.Tools),
	}

	logx.WithFields(logx.Fields{
		"route_path":  routeMatch.Route.Pattern,
		"route_name":  routeMatch.Route.Name,
		"param_count": len(routeMatch.Params),
		"tool_count":  len(fullContext.AvailableTools),
	}).Debug("Full context structure initialized")

	// Load context providers for this route
	providerClient, err := b.loader.LoadFromRouteConfig(routeMatch.Route)
	if err != nil {
		logx.WithError(err).Error("Failed to load providers from route config")
		return nil, NewBuildFailedError(err)
	}

	// Build parameters for providers
	params := b.buildProviderParams(routeMatch, user)
	logx.WithField("param_count", len(params)).Debug("Provider parameters built")

	// Execute all providers in parallel
	backendData, err := b.executeProviders(ctx, providerClient, routeMatch.Route.Context.Providers, params)
	if err != nil {
		// Log error but don't fail completely - partial context is OK
		logx.WithError(err).Warn("Some providers failed, continuing with partial context")
	}
	fullContext.Backend = backendData

	logx.WithFields(logx.Fields{
		"route_name":       routeMatch.Route.Name,
		"backend_keys":     len(backendData),
		"has_instructions": fullContext.Instructions != "",
	}).Info("Full context built successfully")

	return fullContext, nil
}

// buildProviderParams builds parameters for context providers
func (b *Builder) buildProviderParams(match *manifest.RouteMatch, user *User) map[string]any {
	params := make(map[string]any)

	// Add route params
	for key, value := range match.Params {
		params[key] = value
	}
	if len(match.Params) > 0 {
		logx.WithField("count", len(match.Params)).Debug("Added route params")
	}

	// Add query params
	for key, value := range match.Query {
		params[key] = value
	}
	if len(match.Query) > 0 {
		logx.WithField("count", len(match.Query)).Debug("Added query params")
	}

	// Add user info
	if user != nil {
		params["user_id"] = user.ID
		if user.Email != "" {
			params["user_email"] = user.Email
		}
		if user.Name != "" {
			params["user_name"] = user.Name
		}
		params["user_authenticated"] = user.IsAuthenticated()
		logx.WithFields(logx.Fields{
			"user_id":       user.ID,
			"authenticated": user.IsAuthenticated(),
		}).Debug("Added user params")
	} else {
		params["user_authenticated"] = false
		logx.Debug("No user, setting authenticated to false")
	}

	return params
}

// executeProviders executes all context providers in parallel
func (b *Builder) executeProviders(
	ctx context.Context,
	providerClient *ProviderClient,
	providerConfigs []manifest.Provider,
	baseParams map[string]any,
) (map[string]any, error) {
	logx.WithField("provider_count", len(providerConfigs)).Info("Executing providers in parallel")

	results := make(map[string]any)
	var mu sync.Mutex
	var wg sync.WaitGroup

	failedProviders := make([]string, 0)
	providerErrors := make([]error, 0)
	skippedCount := 0
	successCount := 0

	for _, config := range providerConfigs {
		// Skip if condition not met
		if config.Condition != "" && !b.evaluateCondition(config.Condition, baseParams) {
			logx.WithFields(logx.Fields{
				"provider":  config.Name,
				"condition": config.Condition,
			}).Debug("Provider skipped due to condition")
			skippedCount++
			continue
		}

		wg.Add(1)
		go func(cfg manifest.Provider) {
			defer wg.Done()

			logx.WithFields(logx.Fields{
				"provider": cfg.Name,
				"optional": cfg.Optional,
			}).Debug("Executing provider")

			// Merge base params with provider-specific params
			params := make(map[string]any)
			maps.Copy(params, baseParams)
			for k, v := range cfg.Params {
				// Resolve template values
				params[k] = b.resolveValue(v, baseParams)
			}

			// Execute provider
			data, err := providerClient.Get(ctx, cfg.Name, params)
			if err != nil {
				mu.Lock()
				if !cfg.Optional {
					failedProviders = append(failedProviders, cfg.Name)
					providerErrors = append(providerErrors, err)
					logx.WithFields(logx.Fields{
						"provider": cfg.Name,
					}).WithError(err).Error("Required provider failed")
				} else {
					logx.WithFields(logx.Fields{
						"provider": cfg.Name,
					}).WithError(err).Warn("Optional provider failed")
				}
				mu.Unlock()
				return
			}

			mu.Lock()
			results[cfg.Name] = data
			successCount++
			logx.WithField("provider", cfg.Name).Debug("Provider executed successfully")
			mu.Unlock()
		}(config)
	}

	wg.Wait()

	logx.WithFields(logx.Fields{
		"total":   len(providerConfigs),
		"success": successCount,
		"failed":  len(failedProviders),
		"skipped": skippedCount,
	}).Info("Provider execution completed")

	// Return error if non-optional providers failed
	if len(providerErrors) > 0 {
		return results, NewMultipleProvidersFailedError(failedProviders, providerErrors)
	}

	return results, nil
}

// evaluateCondition evaluates a condition string
func (b *Builder) evaluateCondition(condition string, params map[string]any) bool {
	var result bool
	switch condition {
	case "user.authenticated":
		if auth, ok := params["user_authenticated"].(bool); ok {
			result = auth
		} else {
			result = false
		}
	case "user.guest":
		if auth, ok := params["user_authenticated"].(bool); ok {
			result = !auth
		} else {
			result = true
		}
	default:
		// Unknown condition - default to true
		logx.WithField("condition", condition).Debug("Unknown condition, defaulting to true")
		result = true
	}

	logx.WithFields(logx.Fields{
		"condition": condition,
		"result":    result,
	}).Trace("Condition evaluated")

	return result
}

// resolveValue resolves template values in provider parameters
func (b *Builder) resolveValue(template any, params map[string]any) any {
	// If it's a string, resolve templates
	if str, ok := template.(string); ok {
		resolved := b.resolveStringTemplate(str, params)
		if resolved != str {
			logx.WithFields(logx.Fields{
				"template": str,
				"resolved": resolved,
			}).Trace("Template resolved")
		}
		return resolved
	}

	// Otherwise return as-is
	return template
}

// resolveStringTemplate resolves template placeholders in a string
func (b *Builder) resolveStringTemplate(template string, params map[string]any) string {
	result := template

	// Replace {key} with params[key]
	for key, value := range params {
		placeholder := "{" + key + "}"
		valueStr := fmt.Sprintf("%v", value)
		result = strings.ReplaceAll(result, placeholder, valueStr)
	}

	// Special handling for nested paths like {route.params.id}
	for key, value := range params {
		routeParam := "{route.params." + key + "}"
		valueStr := fmt.Sprintf("%v", value)
		result = strings.ReplaceAll(result, routeParam, valueStr)
	}

	return result
}

// BuildMinimal builds a minimal context without executing providers
func (b *Builder) BuildMinimal(
	routeMatch *manifest.RouteMatch,
	user *User,
) (*FullContext, error) {
	logx.WithFields(logx.Fields{
		"route_name": getRouteName(routeMatch),
		"has_user":   user != nil,
	}).Info("Building minimal context")

	if routeMatch == nil {
		logx.Error("Route match is nil")
		return nil, NewInvalidRouteMatchError()
	}

	if routeMatch.Route == nil {
		logx.Error("Route config is missing")
		return nil, NewMissingRouteConfigError()
	}

	context := &FullContext{
		Route: RouteInfo{
			Path:   routeMatch.Route.Pattern,
			Name:   routeMatch.Route.Name,
			Params: routeMatch.Params,
			Query:  routeMatch.Query,
		},
		User:           user,
		Frontend:       nil,
		Backend:        make(map[string]any),
		Instructions:   routeMatch.Route.AgentInstructions,
		AvailableTools: extractToolNames(routeMatch.Route.Tools),
	}

	logx.WithField("route_name", routeMatch.Route.Name).Debug("Minimal context built successfully")

	return context, nil
}

func extractToolNames(tools []manifest.Tool) []string {
	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = tool.Name
	}
	return names
}

func getRouteName(routeMatch *manifest.RouteMatch) string {
	if routeMatch == nil || routeMatch.Route == nil {
		return "unknown"
	}
	return routeMatch.Route.Name
}
