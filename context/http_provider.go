package context

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Abraxas-365/ams/pkg/logx"
)

// HTTPProvider makes HTTP requests to fetch context
type HTTPProvider struct {
	name   string
	config HTTPConfig
	client *http.Client
}

// HTTPConfig configures the HTTP provider
type HTTPConfig struct {
	URL     string            // API endpoint URL (supports templating)
	Method  string            // HTTP method (GET, POST, PUT, DELETE, PATCH)
	Headers map[string]string // HTTP headers (supports templating)
	Body    interface{}       // Request body (for POST/PUT/PATCH)
	Timeout time.Duration     // Request timeout
}

// NewHTTPProvider creates a new HTTP context provider
func NewHTTPProvider(name string, config HTTPConfig) *HTTPProvider {
	// Set defaults
	if config.Method == "" {
		config.Method = "GET"
	}
	if config.Timeout == 0 {
		config.Timeout = 10 * time.Second
	}

	logx.WithFields(logx.Fields{
		"provider": name,
		"method":   config.Method,
		"url":      config.URL,
		"timeout":  config.Timeout,
	}).Debug("HTTP provider created")

	return &HTTPProvider{
		name:   name,
		config: config,
		client: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

// Name returns the provider name
func (p *HTTPProvider) Name() string {
	return p.name
}

// GetContext executes the HTTP request and returns the response
func (p *HTTPProvider) GetContext(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	logx.WithFields(logx.Fields{
		"provider":    p.name,
		"method":      p.config.Method,
		"param_count": len(params),
	}).Debug("HTTP provider fetching context")

	// 1. Resolve URL with parameters
	url := p.resolveTemplate(p.config.URL, params)
	logx.WithFields(logx.Fields{
		"provider": p.name,
		"url":      url,
	}).Debug("URL resolved")

	// 2. Prepare request body if needed
	var bodyReader io.Reader
	if p.config.Body != nil {
		bodyJSON, err := json.Marshal(p.config.Body)
		if err != nil {
			logx.WithFields(logx.Fields{
				"provider": p.name,
			}).WithError(err).Error("Failed to marshal request body")
			return nil, NewProviderFailedError(p.name, fmt.Errorf("error marshaling body: %w", err))
		}

		// Also resolve templates in body
		bodyStr := p.resolveTemplate(string(bodyJSON), params)
		bodyReader = bytes.NewReader([]byte(bodyStr))
		logx.WithFields(logx.Fields{
			"provider":    p.name,
			"body_length": len(bodyStr),
		}).Debug("Request body prepared")
	}

	// 3. Create HTTP request
	req, err := http.NewRequestWithContext(ctx, p.config.Method, url, bodyReader)
	if err != nil {
		logx.WithFields(logx.Fields{
			"provider": p.name,
			"url":      url,
		}).WithError(err).Error("Failed to create HTTP request")
		return nil, NewProviderFailedError(p.name, fmt.Errorf("error creating request: %w", err))
	}

	// 4. Add headers (with template resolution)
	headerCount := 0
	for key, value := range p.config.Headers {
		resolvedValue := p.resolveTemplate(value, params)
		req.Header.Set(key, resolvedValue)
		headerCount++
	}

	// Set Content-Type for requests with body
	if p.config.Body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
		headerCount++
	}

	if headerCount > 0 {
		logx.WithFields(logx.Fields{
			"provider":     p.name,
			"header_count": headerCount,
		}).Debug("Request headers set")
	}

	// 5. Execute request
	logx.WithFields(logx.Fields{
		"provider": p.name,
		"method":   p.config.Method,
		"url":      url,
	}).Info("Executing HTTP request")

	startTime := time.Now()
	resp, err := p.client.Do(req)
	duration := time.Since(startTime)

	if err != nil {
		// Check if it's a timeout
		if ctx.Err() == context.DeadlineExceeded {
			logx.WithFields(logx.Fields{
				"provider": p.name,
				"url":      url,
				"duration": duration,
			}).Warn("HTTP request timeout")
			return nil, NewProviderTimeoutError(p.name)
		}
		logx.WithFields(logx.Fields{
			"provider": p.name,
			"url":      url,
			"duration": duration,
		}).WithError(err).Error("HTTP request failed")
		return nil, NewProviderFailedError(p.name, fmt.Errorf("error executing request: %w", err))
	}
	defer resp.Body.Close()

	logx.WithFields(logx.Fields{
		"provider":    p.name,
		"status_code": resp.StatusCode,
		"duration":    duration,
	}).Debug("HTTP response received")

	// 6. Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		logx.WithFields(logx.Fields{
			"provider":    p.name,
			"status_code": resp.StatusCode,
			"url":         url,
			"body":        string(body),
		}).Error("HTTP request returned error status")
		return nil, NewProviderFailedError(p.name,
			fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body)))
	}

	// 7. Read and parse response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logx.WithFields(logx.Fields{
			"provider": p.name,
		}).WithError(err).Error("Failed to read response body")
		return nil, NewProviderFailedError(p.name, fmt.Errorf("error reading response: %w", err))
	}

	logx.WithFields(logx.Fields{
		"provider":    p.name,
		"body_length": len(body),
	}).Debug("Response body read")

	// 8. Try to parse as JSON
	var result interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		// If not JSON, return as string
		logx.WithField("provider", p.name).Debug("Response is not JSON, returning as string")
		return string(body), nil
	}

	logx.WithFields(logx.Fields{
		"provider": p.name,
		"duration": duration,
	}).Info("HTTP provider context fetched successfully")

	return result, nil
}

// resolveTemplate replaces placeholders with actual values
// Supports:
//   - {param_name} - from params map
//   - {env.VAR_NAME} - from environment variables
func (p *HTTPProvider) resolveTemplate(template string, params map[string]interface{}) string {
	result := template

	// Replace {param_name} with values from params
	replacements := 0
	for key, value := range params {
		placeholder := "{" + key + "}"
		if strings.Contains(result, placeholder) {
			valueStr := fmt.Sprintf("%v", value)
			result = strings.ReplaceAll(result, placeholder, valueStr)
			replacements++
		}
	}

	// Replace {env.VAR_NAME} with environment variables
	result = p.resolveEnvVars(result)

	if replacements > 0 {
		logx.WithFields(logx.Fields{
			"provider":     p.name,
			"replacements": replacements,
		}).Trace("Template placeholders resolved")
	}

	return result
}

// resolveEnvVars replaces {env.VAR_NAME} with environment variables
func (p *HTTPProvider) resolveEnvVars(input string) string {
	result := input
	envVarsReplaced := 0

	// Find all {env.VAR_NAME} patterns
	for {
		start := strings.Index(result, "{env.")
		if start == -1 {
			break
		}

		end := strings.Index(result[start:], "}")
		if end == -1 {
			break
		}
		end += start

		// Extract variable name
		varName := result[start+5 : end]

		// Get environment variable
		varValue := os.Getenv(varName)
		if varValue == "" {
			logx.WithFields(logx.Fields{
				"provider": p.name,
				"var_name": varName,
			}).Warn("Environment variable not found or empty")
		}

		// Replace
		result = result[:start] + varValue + result[end+1:]
		envVarsReplaced++
	}

	if envVarsReplaced > 0 {
		logx.WithFields(logx.Fields{
			"provider":  p.name,
			"env_count": envVarsReplaced,
		}).Debug("Environment variables resolved")
	}

	return result
}
