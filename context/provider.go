// context/provider.go
package context

import (
	"context"
)

// Provider represents a context provider that fetches data
type Provider interface {
	// GetContext retrieves context data
	GetContext(ctx context.Context, params map[string]interface{}) (interface{}, error)

	// Name returns the provider name
	Name() string
}

// ProviderClient manages multiple context providers
type ProviderClient struct {
	providers map[string]Provider
}

// NewProviderClient creates a new provider client from providers
func NewProviderClient(providers ...Provider) *ProviderClient {
	providerMap := make(map[string]Provider)
	for _, provider := range providers {
		providerMap[provider.Name()] = provider
	}
	return &ProviderClient{providers: providerMap}
}

// Get retrieves context from a specific provider
func (c *ProviderClient) Get(ctx context.Context, name string, params map[string]interface{}) (interface{}, error) {
	provider, ok := c.providers[name]
	if !ok {
		return nil, NewProviderNotFoundError(name)
	}

	return provider.GetContext(ctx, params)
}

// GetAll retrieves context from all providers in parallel
func (c *ProviderClient) GetAll(ctx context.Context, params map[string]interface{}) (map[string]interface{}, error) {
	results := make(map[string]interface{})
	errors := make([]error, 0)

	type result struct {
		name string
		data interface{}
		err  error
	}

	ch := make(chan result, len(c.providers))

	for name, provider := range c.providers {
		go func(n string, p Provider) {
			data, err := p.GetContext(ctx, params)
			ch <- result{name: n, data: data, err: err}
		}(name, provider)
	}

	// Collect results
	for i := 0; i < len(c.providers); i++ {
		res := <-ch
		if res.err != nil {
			errors = append(errors, res.err)
			continue
		}
		results[res.name] = res.data
	}

	if len(errors) > 0 {
		return results, NewMultipleProvidersFailedError(getErrorNames(errors), errors)
	}

	return results, nil
}

// List returns all provider names
func (c *ProviderClient) List() []string {
	names := make([]string, 0, len(c.providers))
	for name := range c.providers {
		names = append(names, name)
	}
	return names
}

// Has checks if a provider exists
func (c *ProviderClient) Has(name string) bool {
	_, ok := c.providers[name]
	return ok
}

// Count returns the number of providers
func (c *ProviderClient) Count() int {
	return len(c.providers)
}

// Helper function to extract provider names from errors
func getErrorNames(errors []error) []string {
	names := make([]string, 0, len(errors))
	for _, err := range errors {
		names = append(names, err.Error())
	}
	return names
}
