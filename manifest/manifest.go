// manifest/manifest.go
package manifest

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sync"

	"gopkg.in/yaml.v3"
)

// Manifest represents the complete application configuration
type Manifest struct {
	Version  string  `json:"version" yaml:"version"`
	Routes   []Route `json:"routes" yaml:"routes"`
	Fallback *Route  `json:"fallback,omitempty" yaml:"fallback,omitempty"`
}

// Route represents a single route configuration
type Route struct {
	Pattern           string  `json:"pattern" yaml:"pattern"`
	Name              string  `json:"name" yaml:"name"`
	Description       string  `json:"description" yaml:"description"`
	Context           Context `json:"context" yaml:"context"`
	Tools             []Tool  `json:"tools" yaml:"tools"` // ✅ Changed from []string
	AgentInstructions string  `json:"agent_instructions" yaml:"agent_instructions"`
	Safety            Safety  `json:"safety" yaml:"safety"`
}

// Context holds context provider configurations
type Context struct {
	Providers []Provider `json:"providers" yaml:"providers"`
}

// Provider defines a single context provider
type Provider struct {
	Type      string            `json:"type" yaml:"type"`
	Name      string            `json:"name" yaml:"name"`
	URL       string            `json:"url" yaml:"url"`
	Method    string            `json:"method" yaml:"method"`
	Headers   map[string]string `json:"headers" yaml:"headers"`
	Body      any               `json:"body,omitempty" yaml:"body,omitempty"`
	Timeout   string            `json:"timeout" yaml:"timeout"`
	Params    map[string]any    `json:"params" yaml:"params"`
	Condition string            `json:"condition" yaml:"condition"`
	Optional  bool              `json:"optional" yaml:"optional"`
}

// Tool represents a tool definition in the manifest
type Tool struct {
	Name        string          `json:"name" yaml:"name"`
	Description string          `json:"description" yaml:"description"`
	Type        string          `json:"type" yaml:"type"` // "http", "internal"
	Config      ToolConfig      `json:"config" yaml:"config"`
	Parameters  []ToolParameter `json:"parameters" yaml:"parameters"`
}

// ToolConfig holds tool-specific configuration
type ToolConfig struct {
	// HTTP config
	Method  string            `json:"method,omitempty" yaml:"method,omitempty"`
	URL     string            `json:"url,omitempty" yaml:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`
	Body    any               `json:"body,omitempty" yaml:"body,omitempty"`
	Timeout string            `json:"timeout,omitempty" yaml:"timeout,omitempty"`

	// Response handling
	ResponsePath string `json:"response_path,omitempty" yaml:"response_path,omitempty"`

	// Internal config (for future)
	Operation string `json:"operation,omitempty" yaml:"operation,omitempty"`
}

// ToolParameter defines a tool parameter
type ToolParameter struct {
	Name        string   `json:"name" yaml:"name"`
	Type        string   `json:"type" yaml:"type"` // "string", "number", "boolean", "object", "array"
	Description string   `json:"description" yaml:"description"`
	Required    bool     `json:"required" yaml:"required"`
	Source      string   `json:"source" yaml:"source"` // "agent" or "context"
	ContextPath string   `json:"context_path,omitempty" yaml:"context_path,omitempty"`
	Enum        []string `json:"enum,omitempty" yaml:"enum,omitempty"`
	Default     any      `json:"default,omitempty" yaml:"default,omitempty"`
}

// Safety holds safety settings for the route
type Safety struct {
	RequireConfirmation []string `json:"require_confirmation" yaml:"require_confirmation"`
	MaxCostPerQuery     float64  `json:"max_cost_per_query" yaml:"max_cost_per_query"`
	PIIProtection       bool     `json:"pii_protection" yaml:"pii_protection"`
	RateLimitPerUser    int      `json:"rate_limit_per_user" yaml:"rate_limit_per_user"`
}

// RouteMatch represents a matched route with extracted parameters
type RouteMatch struct {
	Route  *Route
	Params map[string]string
	Query  map[string]string
}

// Registry manages route configurations
type Registry struct {
	mu       sync.RWMutex
	manifest *Manifest
	routes   []routeEntry
	fallback *Route
}

type routeEntry struct {
	route   *Route
	pattern *regexp.Regexp
	params  []string
}

// NewRegistry creates a new manifest registry
func NewRegistry() *Registry {
	return &Registry{
		routes: make([]routeEntry, 0),
	}
}

// Load loads manifest from a Manifest object
func (r *Registry) Load(manifest *Manifest) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.manifest = manifest
	r.routes = make([]routeEntry, 0, len(manifest.Routes))

	// Compile all routes
	for i := range manifest.Routes {
		entry, err := r.compileRoute(&manifest.Routes[i])
		if err != nil {
			return fmt.Errorf("error compiling route %s: %w", manifest.Routes[i].Pattern, err)
		}
		r.routes = append(r.routes, entry)
	}

	// Set fallback
	r.fallback = manifest.Fallback

	return nil
}

// compileRoute converts a route pattern to a regex
func (r *Registry) compileRoute(route *Route) (routeEntry, error) {
	pattern := route.Pattern
	params := make([]string, 0)

	// Extract parameter names from pattern
	paramRegex := regexp.MustCompile(`:(\w+)`)
	matches := paramRegex.FindAllStringSubmatch(pattern, -1)
	for _, match := range matches {
		params = append(params, match[1])
	}

	// Convert pattern to regex
	regexPattern := paramRegex.ReplaceAllString(pattern, `([^/]+)`)
	regexPattern = "^" + regexPattern + "$"

	compiled, err := regexp.Compile(regexPattern)
	if err != nil {
		return routeEntry{}, fmt.Errorf("invalid pattern: %w", err)
	}

	return routeEntry{
		route:   route,
		pattern: compiled,
		params:  params,
	}, nil
}

// Match finds a matching route for the given path
func (r *Registry) Match(path string) (*RouteMatch, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.manifest == nil {
		return nil, NewManifestNotLoadedError()
	}

	for _, entry := range r.routes {
		if match := entry.pattern.FindStringSubmatch(path); match != nil {
			params := make(map[string]string)
			for i, param := range entry.params {
				params[param] = match[i+1]
			}

			return &RouteMatch{
				Route:  entry.route,
				Params: params,
				Query:  make(map[string]string),
			}, nil
		}
	}

	// Return fallback if no match
	if r.fallback != nil {
		return &RouteMatch{
			Route:  r.fallback,
			Params: make(map[string]string),
			Query:  make(map[string]string),
		}, nil
	}

	return nil, NewRouteNotFoundError(path)
}

// MatchOrFallback is like Match but never returns an error
func (r *Registry) MatchOrFallback(path string) *RouteMatch {
	match, _ := r.Match(path)
	return match
}

// GetRouteContext returns route context with parameters and query strings
func (r *Registry) GetRouteContext(path string, query map[string]string) (*RouteMatch, error) {
	match, err := r.Match(path)
	if err != nil {
		return nil, err
	}

	if query != nil {
		match.Query = query
	}

	return match, nil
}

// GetByName returns a route by name
func (r *Registry) GetByName(name string) *Route {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, entry := range r.routes {
		if entry.route.Name == name {
			return entry.route
		}
	}

	return nil
}

// ListRoutes returns all route patterns
func (r *Registry) ListRoutes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	routes := make([]string, 0, len(r.routes))
	for _, entry := range r.routes {
		routes = append(routes, entry.route.Pattern)
	}

	return routes
}

// ListRouteConfigs returns all route configurations
func (r *Registry) ListRouteConfigs() []*Route {
	r.mu.RLock()
	defer r.mu.RUnlock()

	configs := make([]*Route, 0, len(r.routes))
	for _, entry := range r.routes {
		configs = append(configs, entry.route)
	}

	return configs
}

// GetManifest returns the loaded manifest
func (r *Registry) GetManifest() *Manifest {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.manifest
}

// Stats returns statistics about loaded routes
func (r *Registry) Stats() map[string]any {
	r.mu.RLock()
	defer r.mu.RUnlock()

	totalProviders := 0
	totalTools := 0

	for _, entry := range r.routes {
		totalProviders += len(entry.route.Context.Providers)
		totalTools += len(entry.route.Tools)
	}

	return map[string]any{
		"version":         r.manifest.Version,
		"total_routes":    len(r.routes),
		"total_providers": totalProviders,
		"total_tools":     totalTools,
		"has_fallback":    r.fallback != nil,
	}
}

// Helper methods for RouteMatch

func (rm *RouteMatch) GetParam(key string) (string, bool) {
	val, ok := rm.Params[key]
	return val, ok
}

func (rm *RouteMatch) GetQuery(key string) (string, bool) {
	val, ok := rm.Query[key]
	return val, ok
}

func (rm *RouteMatch) HasTool(toolName string) bool {
	for _, tool := range rm.Route.Tools {
		if tool.Name == toolName {
			return true
		}
	}
	return false
}

// Helper methods for Route

func (r *Route) GetProviderByName(name string) *Provider {
	for i := range r.Context.Providers {
		if r.Context.Providers[i].Name == name {
			return &r.Context.Providers[i]
		}
	}
	return nil
}

func (r *Route) GetToolByName(name string) *Tool {
	for i := range r.Tools {
		if r.Tools[i].Name == name {
			return &r.Tools[i]
		}
	}
	return nil
}

func (r *Route) HasProvider(name string) bool {
	return r.GetProviderByName(name) != nil
}

func (r *Route) GetHTTPProviders() []Provider {
	providers := make([]Provider, 0)
	for _, p := range r.Context.Providers {
		if p.Type == "http" {
			providers = append(providers, p)
		}
	}
	return providers
}

func (r *Route) IsToolConfirmationRequired(toolName string) bool {
	for _, tool := range r.Safety.RequireConfirmation {
		if tool == toolName {
			return true
		}
	}
	return false
}

func (r *Route) String() string {
	return fmt.Sprintf("Route{name=%s, pattern=%s, tools=%d}", r.Name, r.Pattern, len(r.Tools))
}

func (p *Provider) String() string {
	return fmt.Sprintf("Provider{name=%s, type=%s, url=%s}", p.Name, p.Type, p.URL)
}

func (t *Tool) String() string {
	return fmt.Sprintf("Tool{name=%s, type=%s}", t.Name, t.Type)
}

// ToJSON converts manifest to JSON
func (m *Manifest) ToJSON() ([]byte, error) {
	return json.MarshalIndent(m, "", "  ")
}

// ToYAML converts manifest to YAML
func (m *Manifest) ToYAML() ([]byte, error) {
	return yaml.Marshal(m)
}

