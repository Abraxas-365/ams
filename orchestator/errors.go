package orchestator

import (
	"net/http"

	"github.com/Abraxas-365/ams/pkg/errx"
)

// Error registry for orchestrator package
var errRegistry = errx.NewRegistry("ORCHESTRATOR")

// Error codes
var (
	// Request errors
	ErrCodeInvalidRequest = errRegistry.Register(
		"INVALID_REQUEST",
		errx.TypeValidation,
		http.StatusBadRequest,
		"Invalid chat request",
	)

	ErrCodeMissingMessage = errRegistry.Register(
		"MISSING_MESSAGE",
		errx.TypeValidation,
		http.StatusBadRequest,
		"Message is required",
	)

	ErrCodeMissingRoute = errRegistry.Register(
		"MISSING_ROUTE",
		errx.TypeValidation,
		http.StatusBadRequest,
		"Route information is required",
	)

	// Route errors
	ErrCodeRouteNotFound = errRegistry.Register(
		"ROUTE_NOT_FOUND",
		errx.TypeNotFound,
		http.StatusNotFound,
		"Route not found",
	)

	ErrCodeRouteMatchFailed = errRegistry.Register(
		"ROUTE_MATCH_FAILED",
		errx.TypeInternal,
		http.StatusInternalServerError,
		"Failed to match route",
	)

	// Context errors
	ErrCodeContextBuildFailed = errRegistry.Register(
		"CONTEXT_BUILD_FAILED",
		errx.TypeInternal,
		http.StatusInternalServerError,
		"Failed to build context",
	)

	// Agent errors
	ErrCodeAgentCreationFailed = errRegistry.Register(
		"AGENT_CREATION_FAILED",
		errx.TypeInternal,
		http.StatusInternalServerError,
		"Failed to create agent",
	)

	ErrCodeAgentExecutionFailed = errRegistry.Register(
		"AGENT_EXECUTION_FAILED",
		errx.TypeInternal,
		http.StatusInternalServerError,
		"Agent execution failed",
	)

	// Tool errors
	ErrCodeToolLoadFailed = errRegistry.Register(
		"TOOL_LOAD_FAILED",
		errx.TypeInternal,
		http.StatusInternalServerError,
		"Failed to load tools",
	)

	// Memory errors
	ErrCodeMemoryInitFailed = errRegistry.Register(
		"MEMORY_INIT_FAILED",
		errx.TypeInternal,
		http.StatusInternalServerError,
		"Failed to initialize memory",
	)
)

// Error constructors

func NewInvalidRequestError(reason string) *errx.Error {
	return errRegistry.NewWithMessage(ErrCodeInvalidRequest, reason)
}

func NewMissingMessageError() *errx.Error {
	return errRegistry.New(ErrCodeMissingMessage)
}

func NewMissingRouteError() *errx.Error {
	return errRegistry.New(ErrCodeMissingRoute)
}

func NewRouteNotFoundError(path string) *errx.Error {
	return errRegistry.New(ErrCodeRouteNotFound).
		WithDetail("path", path)
}

func NewRouteMatchFailedError(path string, cause error) *errx.Error {
	return errRegistry.NewWithCause(ErrCodeRouteMatchFailed, cause).
		WithDetail("path", path)
}

func NewContextBuildFailedError(cause error) *errx.Error {
	return errRegistry.NewWithCause(ErrCodeContextBuildFailed, cause)
}

func NewAgentCreationFailedError(cause error) *errx.Error {
	return errRegistry.NewWithCause(ErrCodeAgentCreationFailed, cause)
}

func NewAgentExecutionFailedError(cause error) *errx.Error {
	return errRegistry.NewWithCause(ErrCodeAgentExecutionFailed, cause)
}

func NewToolLoadFailedError(cause error) *errx.Error {
	return errRegistry.NewWithCause(ErrCodeToolLoadFailed, cause)
}

func NewMemoryInitFailedError(cause error) *errx.Error {
	return errRegistry.NewWithCause(ErrCodeMemoryInitFailed, cause)
}
