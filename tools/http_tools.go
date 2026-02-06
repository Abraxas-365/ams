package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/Abraxas-365/ams/manifest"
	"github.com/Abraxas-365/ams/pkg/ai/llm"
	"github.com/Abraxas-365/ams/pkg/logx"
)

// HTTPTool implements toolx.Toolx for HTTP-based tools
type HTTPTool struct {
	definition      manifest.Tool
	workflowContext map[string]any
	userToken       string
	client          *http.Client
}

// NewHTTPTool creates a new HTTP tool
func NewHTTPTool(definition manifest.Tool, workflowContext map[string]any, userToken string) *HTTPTool {
	timeout := 30 * time.Second
	if definition.Config.Timeout != "" {
		if d, err := time.ParseDuration(definition.Config.Timeout); err == nil {
			timeout = d
		}
	}

	logx.WithFields(logx.Fields{
		"tool":     definition.Name,
		"method":   definition.Config.Method,
		"url":      definition.Config.URL,
		"timeout":  timeout,
		"has_auth": userToken != "",
	}).Debug("HTTP tool created")

	return &HTTPTool{
		definition:      definition,
		workflowContext: workflowContext,
		userToken:       userToken,
		client:          &http.Client{Timeout: timeout},
	}
}

// Name returns the tool name (sanitized for LLM)
func (t *HTTPTool) Name() string {
	return sanitizeName(t.definition.Name)
}

// GetTool returns the LLM tool definition
func (t *HTTPTool) GetTool() llm.Tool {
	tool := llm.Tool{
		Type: "function",
		Function: llm.Function{
			Name:        t.Name(),
			Description: t.definition.Description,
			Parameters:  t.buildParameters(),
		},
	}

	logx.WithFields(logx.Fields{
		"tool":        t.Name(),
		"param_count": len(t.definition.Parameters),
	}).Trace("Tool definition built")

	return tool
}

// Call executes the HTTP tool
func (t *HTTPTool) Call(ctx context.Context, inputs string) (any, error) {
	logx.WithFields(logx.Fields{
		"tool":   t.definition.Name,
		"inputs": inputs,
	}).Info("Executing HTTP tool")

	// 1. Parse agent parameters
	var agentParams map[string]any
	if inputs != "" {
		if err := json.Unmarshal([]byte(inputs), &agentParams); err != nil {
			logx.WithFields(logx.Fields{
				"tool":   t.definition.Name,
				"inputs": inputs,
			}).WithError(err).Error("Failed to parse tool inputs")
			return nil, NewToolExecutionError(t.definition.Name, fmt.Errorf("failed to parse inputs: %w", err))
		}
		logx.WithFields(logx.Fields{
			"tool":        t.definition.Name,
			"param_count": len(agentParams),
		}).Debug("Agent parameters parsed")
	} else {
		agentParams = make(map[string]any)
		logx.WithField("tool", t.definition.Name).Debug("No agent parameters provided")
	}

	// 2. Resolve all parameters (agent + context)
	completeParams, err := t.resolveParameters(agentParams)
	if err != nil {
		logx.WithFields(logx.Fields{
			"tool": t.definition.Name,
		}).WithError(err).Error("Failed to resolve parameters")
		return nil, err
	}
	logx.WithFields(logx.Fields{
		"tool":        t.definition.Name,
		"param_count": len(completeParams),
	}).Debug("Parameters resolved")

	// 3. Build HTTP request
	req, err := t.buildRequest(ctx, completeParams)
	if err != nil {
		logx.WithFields(logx.Fields{
			"tool": t.definition.Name,
		}).WithError(err).Error("Failed to build HTTP request")
		return nil, NewToolExecutionError(t.definition.Name, err)
	}
	logx.WithFields(logx.Fields{
		"tool":   t.definition.Name,
		"method": req.Method,
		"url":    req.URL.String(),
	}).Debug("HTTP request built")

	// 4. Execute request
	startTime := time.Now()
	resp, err := t.client.Do(req)
	duration := time.Since(startTime)

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			logx.WithFields(logx.Fields{
				"tool":     t.definition.Name,
				"duration": duration,
			}).Warn("HTTP tool request timeout")
			return nil, NewToolTimeoutError(t.definition.Name)
		}
		logx.WithFields(logx.Fields{
			"tool":     t.definition.Name,
			"duration": duration,
		}).WithError(err).Error("HTTP tool request failed")
		return nil, NewToolExecutionError(t.definition.Name, fmt.Errorf("HTTP request failed: %w", err))
	}
	defer resp.Body.Close()

	logx.WithFields(logx.Fields{
		"tool":        t.definition.Name,
		"status_code": resp.StatusCode,
		"duration":    duration,
	}).Debug("HTTP response received")

	// 5. Handle response
	result, err := t.handleResponse(resp)
	if err != nil {
		logx.WithFields(logx.Fields{
			"tool":        t.definition.Name,
			"status_code": resp.StatusCode,
		}).WithError(err).Error("Failed to handle HTTP response")
		return nil, err
	}

	logx.WithFields(logx.Fields{
		"tool":     t.definition.Name,
		"duration": duration,
	}).Info("HTTP tool executed successfully")

	return result, nil
}

// buildParameters creates JSON Schema for LLM (only agent parameters)
func (t *HTTPTool) buildParameters() map[string]any {
	properties := make(map[string]any)
	required := []string{}
	agentParamCount := 0

	for _, param := range t.definition.Parameters {
		// Only include agent parameters (LLM needs to provide these)
		if param.Source != "agent" {
			continue
		}

		agentParamCount++

		propDef := map[string]any{
			"type":        param.Type,
			"description": param.Description,
		}

		// Add enum constraint
		if len(param.Enum) > 0 {
			propDef["enum"] = param.Enum
		}

		// Add default value
		if param.Default != nil {
			propDef["default"] = param.Default
		}

		// Handle array type
		if param.Type == "array" {
			propDef["items"] = map[string]any{"type": "string"}
		}

		properties[param.Name] = propDef

		if param.Required {
			required = append(required, param.Name)
		}
	}

	logx.WithFields(logx.Fields{
		"tool":            t.definition.Name,
		"agent_params":    agentParamCount,
		"required_params": len(required),
	}).Trace("Built parameter schema")

	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}

	if len(required) > 0 {
		schema["required"] = required
	}

	return schema
}

// resolveParameters merges agent params with auto-injected context params
func (t *HTTPTool) resolveParameters(agentParams map[string]any) (map[string]any, error) {
	result := make(map[string]any)
	agentCount := 0
	contextCount := 0
	routeCount := 0

	for _, param := range t.definition.Parameters {
		switch param.Source {
		case "agent":
			agentCount++
			// Get from agent-provided parameters
			if val, ok := agentParams[param.Name]; ok {
				result[param.Name] = val
				logx.WithFields(logx.Fields{
					"tool":  t.definition.Name,
					"param": param.Name,
					"value": val,
				}).Trace("Resolved agent parameter")
			} else if param.Required {
				logx.WithFields(logx.Fields{
					"tool":  t.definition.Name,
					"param": param.Name,
				}).Error("Missing required agent parameter")
				return nil, NewMissingParameterError(t.definition.Name, param.Name)
			} else if param.Default != nil {
				result[param.Name] = param.Default
				logx.WithFields(logx.Fields{
					"tool":    t.definition.Name,
					"param":   param.Name,
					"default": param.Default,
				}).Trace("Using default parameter value")
			}

		case "route":
			// ✅ Extract from route parameters (top-level workflow context)
			routeCount++
			if value, ok := t.workflowContext[param.Name]; ok {
				result[param.Name] = value
				logx.WithFields(logx.Fields{
					"tool":  t.definition.Name,
					"param": param.Name,
					"value": value,
				}).Trace("Resolved route parameter")
			} else if param.Required {
				logx.WithFields(logx.Fields{
					"tool":  t.definition.Name,
					"param": param.Name,
				}).Error("Missing required route parameter")
				return nil, NewMissingParameterError(t.definition.Name, param.Name)
			}

		case "context":
			contextCount++
			// Auto-inject from workflow context
			if param.ContextPath == "" {
				logx.WithFields(logx.Fields{
					"tool":  t.definition.Name,
					"param": param.Name,
				}).Error("Context path missing for parameter")
				return nil, NewInvalidToolError(fmt.Sprintf("context_path missing for parameter: %s", param.Name))
			}

			value, err := extractFromContext(param.ContextPath, t.workflowContext)
			if err != nil {
				if param.Required {
					logx.WithFields(logx.Fields{
						"tool":         t.definition.Name,
						"param":        param.Name,
						"context_path": param.ContextPath,
					}).WithError(err).Error("Failed to extract required context parameter")
					return nil, NewParameterResolutionError(t.definition.Name, param.Name, err)
				}
				// Optional parameter not found - skip it
				logx.WithFields(logx.Fields{
					"tool":         t.definition.Name,
					"param":        param.Name,
					"context_path": param.ContextPath,
				}).Debug("Optional context parameter not found, skipping")
				continue
			}

			result[param.Name] = value
			logx.WithFields(logx.Fields{
				"tool":         t.definition.Name,
				"param":        param.Name,
				"context_path": param.ContextPath,
			}).Trace("Resolved context parameter")
		}
	}

	logx.WithFields(logx.Fields{
		"tool":           t.definition.Name,
		"agent_params":   agentCount,
		"route_params":   routeCount,
		"context_params": contextCount,
		"total_resolved": len(result),
	}).Debug("Parameter resolution completed")

	return result, nil
}

// buildRequest creates the HTTP request with auth
func (t *HTTPTool) buildRequest(ctx context.Context, params map[string]any) (*http.Request, error) {
	// 1. Resolve URL with parameters
	originalURL := t.definition.Config.URL
	resolvedURL := t.resolveTemplate(originalURL, params)

	// ✅ DEBUG LOGGING - Shows template resolution details
	logx.WithFields(logx.Fields{
		"tool":         t.definition.Name,
		"original_url": originalURL,
		"resolved_url": resolvedURL,
		"params":       params,
		"param_count":  len(params),
	}).Info("🔍 URL RESOLUTION DEBUG")

	// Also log individual param replacements
	for key, value := range params {
		placeholder := "{" + key + "}"
		if strings.Contains(originalURL, placeholder) {
			logx.WithFields(logx.Fields{
				"tool":        t.definition.Name,
				"placeholder": placeholder,
				"value":       value,
				"found":       strings.Contains(originalURL, placeholder),
			}).Debug("🔍 Placeholder check")
		}
	}

	logx.WithFields(logx.Fields{
		"tool": t.definition.Name,
		"url":  resolvedURL,
	}).Debug("URL resolved")

	method := t.definition.Config.Method
	if method == "" {
		method = "GET"
	}

	var bodyReader io.Reader

	// 2. Handle request body
	if t.definition.Config.Body != nil {
		bodyJSON, err := json.Marshal(t.definition.Config.Body)
		if err != nil {
			logx.WithField("tool", t.definition.Name).WithError(err).Error("Failed to marshal request body")
			return nil, fmt.Errorf("failed to marshal body: %w", err)
		}
		bodyStr := t.resolveTemplate(string(bodyJSON), params)
		bodyReader = bytes.NewReader([]byte(bodyStr))
		logx.WithFields(logx.Fields{
			"tool":        t.definition.Name,
			"body_length": len(bodyStr),
		}).Debug("Request body prepared")
	}

	// 3. Create request
	req, err := http.NewRequestWithContext(ctx, method, resolvedURL, bodyReader)
	if err != nil {
		logx.WithFields(logx.Fields{
			"tool":   t.definition.Name,
			"method": method,
			"url":    resolvedURL,
		}).WithError(err).Error("Failed to create HTTP request")
		return nil, err
	}

	// 4. Add headers with authentication resolution
	headerCount := 0
	for key, value := range t.definition.Config.Headers {
		resolvedValue := t.resolveTemplate(value, params)
		req.Header.Set(key, resolvedValue)
		headerCount++
	}

	// 5. Set Content-Type if body present
	if t.definition.Config.Body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
		headerCount++
	}

	if headerCount > 0 {
		logx.WithFields(logx.Fields{
			"tool":         t.definition.Name,
			"header_count": headerCount,
		}).Debug("Request headers set")
	}

	return req, nil
}

// resolveTemplate resolves all template variables including auth
func (t *HTTPTool) resolveTemplate(template string, params map[string]any) string {
	result := template
	replacements := 0

	// 1. Replace {user.token} with actual user bearer token
	if strings.Contains(result, "{user.token}") {
		result = strings.ReplaceAll(result, "{user.token}", t.userToken)
		replacements++
		logx.WithField("tool", t.definition.Name).Trace("Replaced user token in template")
	}

	// 2. Replace {env.VAR} with environment variables
	re := regexp.MustCompile(`\{env\.([^}]+)\}`)
	envMatches := re.FindAllStringSubmatch(result, -1)
	result = re.ReplaceAllStringFunc(result, func(match string) string {
		varName := match[5 : len(match)-1]
		value := os.Getenv(varName)
		if value == "" {
			logx.WithFields(logx.Fields{
				"tool":     t.definition.Name,
				"var_name": varName,
			}).Warn("Environment variable not found or empty")
		}
		replacements++
		return value
	})
	if len(envMatches) > 0 {
		logx.WithFields(logx.Fields{
			"tool":      t.definition.Name,
			"env_count": len(envMatches),
		}).Trace("Replaced environment variables in template")
	}

	// 3. Replace {param} and {{param}} with parameter values
	for key, value := range params {
		placeholder1 := "{" + key + "}"
		placeholder2 := "{{" + key + "}}"
		valueStr := fmt.Sprint(value)

		if strings.Contains(result, placeholder1) || strings.Contains(result, placeholder2) {
			result = strings.ReplaceAll(result, placeholder1, valueStr)
			result = strings.ReplaceAll(result, placeholder2, valueStr)
			replacements++
			logx.WithFields(logx.Fields{
				"tool":        t.definition.Name,
				"key":         key,
				"value":       valueStr,
				"placeholder": placeholder1,
			}).Trace("Replaced parameter in template")
		}
	}

	if replacements > 0 {
		logx.WithFields(logx.Fields{
			"tool":         t.definition.Name,
			"replacements": replacements,
		}).Trace("Template resolution completed")
	}

	return result
}

// handleResponse processes the HTTP response
func (t *HTTPTool) handleResponse(resp *http.Response) (any, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logx.WithField("tool", t.definition.Name).WithError(err).Error("Failed to read response body")
		return nil, NewToolExecutionError(t.definition.Name, fmt.Errorf("failed to read response: %w", err))
	}

	logx.WithFields(logx.Fields{
		"tool":        t.definition.Name,
		"status_code": resp.StatusCode,
		"body_length": len(body),
	}).Debug("Response body read")

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		logx.WithFields(logx.Fields{
			"tool":        t.definition.Name,
			"status_code": resp.StatusCode,
			"body":        string(body),
		}).Error("HTTP request returned error status")
		return nil, NewToolExecutionError(
			t.definition.Name,
			fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body)),
		)
	}

	// Try to parse as JSON
	var result any
	if err := json.Unmarshal(body, &result); err != nil {
		// Not JSON, return as string
		logx.WithField("tool", t.definition.Name).Debug("Response is not JSON, returning as string")
		return string(body), nil
	}

	logx.WithField("tool", t.definition.Name).Debug("Response parsed as JSON")

	// Extract specific path if configured
	if t.definition.Config.ResponsePath != "" {
		result = extractJSONPath(result, t.definition.Config.ResponsePath)
		logx.WithFields(logx.Fields{
			"tool":          t.definition.Name,
			"response_path": t.definition.Config.ResponsePath,
		}).Debug("Extracted response path")
	}

	return result, nil
}

// Helper functions

func sanitizeName(name string) string {
	// Remove spaces and special characters
	reg := regexp.MustCompile(`[^a-zA-Z0-9_-]+`)
	name = strings.ReplaceAll(name, " ", "_")
	name = reg.ReplaceAllString(name, "")
	return strings.ToLower(name)
}

func extractFromContext(contextPath string, workflowContext map[string]any) (any, error) {
	// Remove {{ and }}
	path := strings.TrimSpace(contextPath)
	path = strings.TrimPrefix(path, "{{")
	path = strings.TrimSuffix(path, "}}")
	path = strings.TrimSpace(path)

	logx.WithFields(logx.Fields{
		"context_path": contextPath,
		"parsed_path":  path,
	}).Trace("Extracting from context")

	// Handle dot notation: "user.id"
	parts := strings.Split(path, ".")
	var current any = workflowContext

	for i, part := range parts {
		part = strings.TrimSpace(part)

		if m, ok := current.(map[string]any); ok {
			if value, exists := m[part]; exists {
				current = value
			} else {
				logx.WithFields(logx.Fields{
					"context_path": contextPath,
					"segment":      part,
					"level":        i,
				}).Debug("Context path segment not found")
				return nil, fmt.Errorf("path segment '%s' not found at level %d", part, i)
			}
		} else {
			logx.WithFields(logx.Fields{
				"context_path": contextPath,
				"segment":      part,
				"type":         fmt.Sprintf("%T", current),
			}).Debug("Cannot traverse non-map in context")
			return nil, fmt.Errorf("cannot traverse non-map at '%s' (type: %T)", part, current)
		}
	}

	logx.WithFields(logx.Fields{
		"context_path": contextPath,
		"value_type":   fmt.Sprintf("%T", current),
	}).Trace("Context extraction successful")

	return current, nil
}

func extractJSONPath(data any, path string) any {
	if path == "" {
		return data
	}

	parts := strings.Split(path, ".")
	current := data

	for _, part := range parts {
		if m, ok := current.(map[string]any); ok {
			if val, exists := m[part]; exists {
				current = val
			} else {
				logx.WithFields(logx.Fields{
					"json_path": path,
					"segment":   part,
				}).Debug("JSON path segment not found")
				return nil
			}
		} else {
			logx.WithFields(logx.Fields{
				"json_path": path,
				"segment":   part,
				"type":      fmt.Sprintf("%T", current),
			}).Debug("Cannot traverse non-map in JSON")
			return nil
		}
	}

	return current
}
