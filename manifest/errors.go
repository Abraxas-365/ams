// manifest/errors.go
package manifest

import (
	"net/http"

	"github.com/Abraxas-365/ams/pkg/errx"
)

// Error registry for manifest package
var errRegistry = errx.NewRegistry("MANIFEST")

// Error codes
var (
	// File errors
	ErrCodeFileNotFound = errRegistry.Register(
		"FILE_NOT_FOUND",
		errx.TypeNotFound,
		http.StatusNotFound,
		"Manifest file not found",
	)

	ErrCodeFileReadError = errRegistry.Register(
		"FILE_READ_ERROR",
		errx.TypeInternal,
		http.StatusInternalServerError,
		"Failed to read manifest file",
	)

	ErrCodeFileWriteError = errRegistry.Register(
		"FILE_WRITE_ERROR",
		errx.TypeInternal,
		http.StatusInternalServerError,
		"Failed to write manifest file",
	)

	// Parsing errors
	ErrCodeInvalidYAML = errRegistry.Register(
		"INVALID_YAML",
		errx.TypeValidation,
		http.StatusBadRequest,
		"Invalid YAML format",
	)

	ErrCodeInvalidJSON = errRegistry.Register(
		"INVALID_JSON",
		errx.TypeValidation,
		http.StatusBadRequest,
		"Invalid JSON format",
	)

	ErrCodeInvalidFormat = errRegistry.Register(
		"INVALID_FORMAT",
		errx.TypeValidation,
		http.StatusBadRequest,
		"Invalid manifest format",
	)

	ErrCodeUnsupportedFormat = errRegistry.Register(
		"UNSUPPORTED_FORMAT",
		errx.TypeValidation,
		http.StatusBadRequest,
		"Unsupported manifest format",
	)

	// Validation errors
	ErrCodeValidationFailed = errRegistry.Register(
		"VALIDATION_FAILED",
		errx.TypeValidation,
		http.StatusBadRequest,
		"Manifest validation failed",
	)

	ErrCodeMissingVersion = errRegistry.Register(
		"MISSING_VERSION",
		errx.TypeValidation,
		http.StatusBadRequest,
		"Manifest version is required",
	)

	ErrCodeMissingRoutes = errRegistry.Register(
		"MISSING_ROUTES",
		errx.TypeValidation,
		http.StatusBadRequest,
		"At least one route is required",
	)

	ErrCodeInvalidPattern = errRegistry.Register(
		"INVALID_PATTERN",
		errx.TypeValidation,
		http.StatusBadRequest,
		"Invalid route pattern",
	)

	ErrCodeMissingRouteName = errRegistry.Register(
		"MISSING_ROUTE_NAME",
		errx.TypeValidation,
		http.StatusBadRequest,
		"Route name is required",
	)

	ErrCodeMissingRoutePattern = errRegistry.Register(
		"MISSING_ROUTE_PATTERN",
		errx.TypeValidation,
		http.StatusBadRequest,
		"Route pattern is required",
	)

	ErrCodeDuplicateRouteName = errRegistry.Register(
		"DUPLICATE_ROUTE_NAME",
		errx.TypeValidation,
		http.StatusBadRequest,
		"Duplicate route name",
	)

	// Provider errors
	ErrCodeInvalidProvider = errRegistry.Register(
		"INVALID_PROVIDER",
		errx.TypeValidation,
		http.StatusBadRequest,
		"Invalid provider configuration",
	)

	ErrCodeMissingProviderType = errRegistry.Register(
		"MISSING_PROVIDER_TYPE",
		errx.TypeValidation,
		http.StatusBadRequest,
		"Provider type is required",
	)

	ErrCodeMissingProviderName = errRegistry.Register(
		"MISSING_PROVIDER_NAME",
		errx.TypeValidation,
		http.StatusBadRequest,
		"Provider name is required",
	)

	ErrCodeMissingProviderURL = errRegistry.Register(
		"MISSING_PROVIDER_URL",
		errx.TypeValidation,
		http.StatusBadRequest,
		"Provider URL is required",
	)

	ErrCodeUnsupportedProviderType = errRegistry.Register(
		"UNSUPPORTED_PROVIDER_TYPE",
		errx.TypeValidation,
		http.StatusBadRequest,
		"Unsupported provider type",
	)

	// Route matching errors
	ErrCodeRouteNotFound = errRegistry.Register(
		"ROUTE_NOT_FOUND",
		errx.TypeNotFound,
		http.StatusNotFound,
		"Route not found",
	)

	ErrCodeRouteCompilationFailed = errRegistry.Register(
		"ROUTE_COMPILATION_FAILED",
		errx.TypeInternal,
		http.StatusInternalServerError,
		"Failed to compile route pattern",
	)

	// Registry errors
	ErrCodeRegistryNotInitialized = errRegistry.Register(
		"REGISTRY_NOT_INITIALIZED",
		errx.TypeInternal,
		http.StatusInternalServerError,
		"Manifest registry not initialized",
	)

	ErrCodeManifestNotLoaded = errRegistry.Register(
		"MANIFEST_NOT_LOADED",
		errx.TypeInternal,
		http.StatusInternalServerError,
		"Manifest not loaded",
	)
)

// Error constructors with context

// NewFileNotFoundError creates a file not found error
func NewFileNotFoundError(filepath string) *errx.Error {
	return errRegistry.New(ErrCodeFileNotFound).
		WithDetail("filepath", filepath)
}

// NewFileReadError creates a file read error
func NewFileReadError(filepath string, cause error) *errx.Error {
	return errRegistry.NewWithCause(ErrCodeFileReadError, cause).
		WithDetail("filepath", filepath)
}

// NewFileWriteError creates a file write error
func NewFileWriteError(filepath string, cause error) *errx.Error {
	return errRegistry.NewWithCause(ErrCodeFileWriteError, cause).
		WithDetail("filepath", filepath)
}

// NewInvalidYAMLError creates an invalid YAML error
func NewInvalidYAMLError(cause error) *errx.Error {
	return errRegistry.NewWithCause(ErrCodeInvalidYAML, cause)
}

// NewInvalidJSONError creates an invalid JSON error
func NewInvalidJSONError(cause error) *errx.Error {
	return errRegistry.NewWithCause(ErrCodeInvalidJSON, cause)
}

// NewInvalidFormatError creates an invalid format error
func NewInvalidFormatError(format Format) *errx.Error {
	return errRegistry.New(ErrCodeInvalidFormat).
		WithDetail("format", string(format))
}

// NewUnsupportedFormatError creates an unsupported format error
func NewUnsupportedFormatError(format string) *errx.Error {
	return errRegistry.New(ErrCodeUnsupportedFormat).
		WithDetail("format", format)
}

// NewValidationError creates a validation error
func NewValidationError(message string) *errx.Error {
	return errRegistry.NewWithMessage(ErrCodeValidationFailed, message)
}

// NewMissingVersionError creates a missing version error
func NewMissingVersionError() *errx.Error {
	return errRegistry.New(ErrCodeMissingVersion)
}

// NewMissingRoutesError creates a missing routes error
func NewMissingRoutesError() *errx.Error {
	return errRegistry.New(ErrCodeMissingRoutes)
}

// NewInvalidPatternError creates an invalid pattern error
func NewInvalidPatternError(pattern string, cause error) *errx.Error {
	return errRegistry.NewWithCause(ErrCodeInvalidPattern, cause).
		WithDetail("pattern", pattern)
}

// NewMissingRouteNameError creates a missing route name error
func NewMissingRouteNameError(pattern string) *errx.Error {
	return errRegistry.New(ErrCodeMissingRouteName).
		WithDetail("pattern", pattern)
}

// NewMissingRoutePatternError creates a missing route pattern error
func NewMissingRoutePatternError(routeName string) *errx.Error {
	return errRegistry.New(ErrCodeMissingRoutePattern).
		WithDetail("route_name", routeName)
}

// NewDuplicateRouteNameError creates a duplicate route name error
func NewDuplicateRouteNameError(routeName string) *errx.Error {
	return errRegistry.New(ErrCodeDuplicateRouteName).
		WithDetail("route_name", routeName)
}

// NewInvalidProviderError creates an invalid provider error
func NewInvalidProviderError(providerName string, message string) *errx.Error {
	return errRegistry.NewWithMessage(ErrCodeInvalidProvider, message).
		WithDetail("provider_name", providerName)
}

// NewMissingProviderTypeError creates a missing provider type error
func NewMissingProviderTypeError(providerName string) *errx.Error {
	return errRegistry.New(ErrCodeMissingProviderType).
		WithDetail("provider_name", providerName)
}

// NewMissingProviderNameError creates a missing provider name error
func NewMissingProviderNameError() *errx.Error {
	return errRegistry.New(ErrCodeMissingProviderName)
}

// NewMissingProviderURLError creates a missing provider URL error
func NewMissingProviderURLError(providerName string, providerType string) *errx.Error {
	return errRegistry.New(ErrCodeMissingProviderURL).
		WithDetail("provider_name", providerName).
		WithDetail("provider_type", providerType)
}

// NewUnsupportedProviderTypeError creates an unsupported provider type error
func NewUnsupportedProviderTypeError(providerType string) *errx.Error {
	return errRegistry.New(ErrCodeUnsupportedProviderType).
		WithDetail("provider_type", providerType)
}

// NewRouteNotFoundError creates a route not found error
func NewRouteNotFoundError(path string) *errx.Error {
	return errRegistry.New(ErrCodeRouteNotFound).
		WithDetail("path", path)
}

// NewRouteCompilationError creates a route compilation error
func NewRouteCompilationError(pattern string, cause error) *errx.Error {
	return errRegistry.NewWithCause(ErrCodeRouteCompilationFailed, cause).
		WithDetail("pattern", pattern)
}

// NewRegistryNotInitializedError creates a registry not initialized error
func NewRegistryNotInitializedError() *errx.Error {
	return errRegistry.New(ErrCodeRegistryNotInitialized)
}

// NewManifestNotLoadedError creates a manifest not loaded error
func NewManifestNotLoadedError() *errx.Error {
	return errRegistry.New(ErrCodeManifestNotLoaded)
}

// Helper function to wrap multiple validation errors
func NewMultipleValidationErrors(errors []error) *errx.Error {
	err := errRegistry.New(ErrCodeValidationFailed)

	errorMessages := make([]string, len(errors))
	for i, e := range errors {
		errorMessages[i] = e.Error()
	}

	return err.WithDetail("errors", errorMessages).
		WithDetail("count", len(errors))
}
