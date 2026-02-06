// context/loader.go
package context

import (
	"fmt"
	"time"

	"github.com/Abraxas-365/ams/manifest"
)

// ProviderLoader loads context providers from manifest configuration
type ProviderLoader struct {
}

// NewProviderLoader creates a new provider loader
func NewProviderLoader() *ProviderLoader {
	return &ProviderLoader{}
}

// LoadFromRouteConfig creates a ProviderClient from route configuration
func (l *ProviderLoader) LoadFromRouteConfig(route *manifest.Route) (*ProviderClient, error) {
	if route == nil {
		return nil, NewMissingRouteConfigError()
	}

	providers := make([]Provider, 0)

	for _, providerConfig := range route.Context.Providers {
		provider, err := l.createProvider(providerConfig)
		if err != nil {
			if providerConfig.Optional {
				// Skip optional providers that fail to load
				continue
			}
			return nil, NewProviderLoadFailedError(providerConfig.Name, err)
		}

		providers = append(providers, provider)
	}

	return NewProviderClient(providers...), nil
}

// createProvider creates a provider based on its type
func (l *ProviderLoader) createProvider(config manifest.Provider) (Provider, error) {
	switch config.Type {
	case "http":
		return l.createHTTPProvider(config)
	default:
		return nil, NewUnsupportedProviderTypeError(config.Type)
	}
}

// createHTTPProvider creates an HTTP context provider
func (l *ProviderLoader) createHTTPProvider(config manifest.Provider) (Provider, error) {
	// Validate configuration
	if config.URL == "" {
		return nil, NewInvalidProviderConfigError(config.Name, "URL is required for HTTP provider")
	}

	// Parse timeout
	timeout := 10 * time.Second
	if config.Timeout != "" {
		d, err := time.ParseDuration(config.Timeout)
		if err != nil {
			return nil, NewInvalidProviderConfigError(config.Name,
				fmt.Sprintf("invalid timeout format: %s", config.Timeout))
		}
		timeout = d
	}

	// Set default method
	method := config.Method
	if method == "" {
		method = "GET"
	}

	// Create HTTP config
	httpConfig := HTTPConfig{
		URL:     config.URL,
		Method:  method,
		Headers: config.Headers,
		Body:    config.Body,
		Timeout: timeout,
	}

	return NewHTTPProvider(config.Name, httpConfig), nil
}

// ValidateProviderConfig validates a provider configuration
func (l *ProviderLoader) ValidateProviderConfig(config manifest.Provider) error {
	if config.Name == "" {
		return NewInvalidProviderConfigError("", "provider name is required")
	}

	if config.Type == "" {
		return NewInvalidProviderConfigError(config.Name, "provider type is required")
	}

	switch config.Type {
	case "http":
		if config.URL == "" {
			return NewInvalidProviderConfigError(config.Name, "URL is required for HTTP provider")
		}
	default:
		return NewUnsupportedProviderTypeError(config.Type)
	}

	return nil
}
