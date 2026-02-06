// context/errors.go
package context

import (
	"net/http"

	"github.com/Abraxas-365/ams/pkg/errx"
)

// Error registry for context package
var errRegistry = errx.NewRegistry("CONTEXT")

// Error codes
var (
	// Provider errors
	ErrCodeProviderFailed = errRegistry.Register(
		"PROVIDER_FAILED",
		errx.TypeExternal,
		http.StatusBadGateway,
		"Context provider failed to retrieve data",
	)

	ErrCodeProviderNotFound = errRegistry.Register(
		"PROVIDER_NOT_FOUND",
		errx.TypeNotFound,
		http.StatusNotFound,
		"Context provider not found",
	)

	ErrCodeProviderTimeout = errRegistry.Register(
		"PROVIDER_TIMEOUT",
		errx.TypeExternal,
		http.StatusGatewayTimeout,
		"Context provider request timeout",
	)

	ErrCodeMultipleProvidersFailed = errRegistry.Register(
		"MULTIPLE_PROVIDERS_FAILED",
		errx.TypeExternal,
		http.StatusBadGateway,
		"Multiple context providers failed",
	)

	// Builder errors
	ErrCodeBuildFailed = errRegistry.Register(
		"BUILD_FAILED",
		errx.TypeInternal,
		http.StatusInternalServerError,
		"Failed to build context",
	)

	ErrCodeInvalidRouteMatch = errRegistry.Register(
		"INVALID_ROUTE_MATCH",
		errx.TypeValidation,
		http.StatusBadRequest,
		"Invalid route match provided",
	)

	ErrCodeMissingRouteConfig = errRegistry.Register(
		"MISSING_ROUTE_CONFIG",
		errx.TypeValidation,
		http.StatusBadRequest,
		"Route configuration is missing",
	)

	// Loader errors
	ErrCodeProviderLoadFailed = errRegistry.Register(
		"PROVIDER_LOAD_FAILED",
		errx.TypeInternal,
		http.StatusInternalServerError,
		"Failed to load provider from configuration",
	)

	ErrCodeUnsupportedProviderType = errRegistry.Register(
		"UNSUPPORTED_PROVIDER_TYPE",
		errx.TypeValidation,
		http.StatusBadRequest,
		"Unsupported provider type",
	)

	ErrCodeInvalidProviderConfig = errRegistry.Register(
		"INVALID_PROVIDER_CONFIG",
		errx.TypeValidation,
		http.StatusBadRequest,
		"Invalid provider configuration",
	)

	// Parameter errors
	ErrCodeMissingParameter = errRegistry.Register(
		"MISSING_PARAMETER",
		errx.TypeValidation,
		http.StatusBadRequest,
		"Required parameter is missing",
	)

	ErrCodeInvalidParameter = errRegistry.Register(
		"INVALID_PARAMETER",
		errx.TypeValidation,
		http.StatusBadRequest,
		"Invalid parameter value",
	)

	// Condition errors
	ErrCodeConditionNotMet = errRegistry.Register(
		"CONDITION_NOT_MET",
		errx.TypeBusiness,
		http.StatusPreconditionFailed,
		"Provider condition not met",
	)
)

// Error constructors

// NewProviderFailedError creates a provider failed error
func NewProviderFailedError(providerName string, cause error) *errx.Error {
	return errRegistry.NewWithCause(ErrCodeProviderFailed, cause).
		WithDetail("provider_name", providerName)
}

// NewProviderNotFoundError creates a provider not found error
func NewProviderNotFoundError(providerName string) *errx.Error {
	return errRegistry.New(ErrCodeProviderNotFound).
		WithDetail("provider_name", providerName)
}

// NewProviderTimeoutError creates a provider timeout error
func NewProviderTimeoutError(providerName string) *errx.Error {
	return errRegistry.New(ErrCodeProviderTimeout).
		WithDetail("provider_name", providerName)
}

// NewMultipleProvidersFailedError creates a multiple providers failed error
func NewMultipleProvidersFailedError(failedProviders []string, errors []error) *errx.Error {
	err := errRegistry.New(ErrCodeMultipleProvidersFailed).
		WithDetail("failed_providers", failedProviders).
		WithDetail("failure_count", len(errors))

	errorMessages := make([]string, len(errors))
	for i, e := range errors {
		errorMessages[i] = e.Error()
	}
	err.WithDetail("errors", errorMessages)

	return err
}

// NewBuildFailedError creates a build failed error
func NewBuildFailedError(cause error) *errx.Error {
	return errRegistry.NewWithCause(ErrCodeBuildFailed, cause)
}

// NewInvalidRouteMatchError creates an invalid route match error
func NewInvalidRouteMatchError() *errx.Error {
	return errRegistry.New(ErrCodeInvalidRouteMatch)
}

// NewMissingRouteConfigError creates a missing route config error
func NewMissingRouteConfigError() *errx.Error {
	return errRegistry.New(ErrCodeMissingRouteConfig)
}

// NewProviderLoadFailedError creates a provider load failed error
func NewProviderLoadFailedError(providerName string, cause error) *errx.Error {
	return errRegistry.NewWithCause(ErrCodeProviderLoadFailed, cause).
		WithDetail("provider_name", providerName)
}

// NewUnsupportedProviderTypeError creates an unsupported provider type error
func NewUnsupportedProviderTypeError(providerType string) *errx.Error {
	return errRegistry.New(ErrCodeUnsupportedProviderType).
		WithDetail("provider_type", providerType)
}

// NewInvalidProviderConfigError creates an invalid provider config error
func NewInvalidProviderConfigError(providerName string, reason string) *errx.Error {
	return errRegistry.New(ErrCodeInvalidProviderConfig).
		WithDetail("provider_name", providerName).
		WithDetail("reason", reason)
}

// NewMissingParameterError creates a missing parameter error
func NewMissingParameterError(paramName string, providerName string) *errx.Error {
	return errRegistry.New(ErrCodeMissingParameter).
		WithDetail("parameter_name", paramName).
		WithDetail("provider_name", providerName)
}

// NewInvalidParameterError creates an invalid parameter error
func NewInvalidParameterError(paramName string, value interface{}, reason string) *errx.Error {
	return errRegistry.New(ErrCodeInvalidParameter).
		WithDetail("parameter_name", paramName).
		WithDetail("value", value).
		WithDetail("reason", reason)
}

// NewConditionNotMetError creates a condition not met error
func NewConditionNotMetError(condition string, providerName string) *errx.Error {
	return errRegistry.New(ErrCodeConditionNotMet).
		WithDetail("condition", condition).
		WithDetail("provider_name", providerName)
}
