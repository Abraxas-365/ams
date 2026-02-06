// tools/errors.go
package tools

import (
	"net/http"

	"github.com/Abraxas-365/ams/pkg/errx"
)

// Error registry for tools package
var errRegistry = errx.NewRegistry("TOOLS")

// Error codes
var (
	ErrCodeInvalidTool = errRegistry.Register(
		"INVALID_TOOL",
		errx.TypeValidation,
		http.StatusBadRequest,
		"Invalid tool configuration",
	)

	ErrCodeUnsupportedToolType = errRegistry.Register(
		"UNSUPPORTED_TOOL_TYPE",
		errx.TypeValidation,
		http.StatusBadRequest,
		"Unsupported tool type",
	)

	ErrCodeToolExecutionFailed = errRegistry.Register(
		"TOOL_EXECUTION_FAILED",
		errx.TypeInternal,
		http.StatusInternalServerError,
		"Tool execution failed",
	)

	ErrCodeMissingParameter = errRegistry.Register(
		"MISSING_PARAMETER",
		errx.TypeValidation,
		http.StatusBadRequest,
		"Missing required parameter",
	)

	ErrCodeParameterResolution = errRegistry.Register(
		"PARAMETER_RESOLUTION_FAILED",
		errx.TypeInternal,
		http.StatusInternalServerError,
		"Failed to resolve parameter from context",
	)

	ErrCodeToolTimeout = errRegistry.Register(
		"TOOL_TIMEOUT",
		errx.TypeInternal,
		http.StatusGatewayTimeout,
		"Tool execution timeout",
	)
)

// Error constructors

func NewInvalidToolError(reason string) *errx.Error {
	return errRegistry.NewWithMessage(ErrCodeInvalidTool, reason)
}

func NewUnsupportedToolTypeError(toolType string) *errx.Error {
	return errRegistry.New(ErrCodeUnsupportedToolType).
		WithDetail("tool_type", toolType)
}

func NewToolExecutionError(toolName string, cause error) *errx.Error {
	return errRegistry.NewWithCause(ErrCodeToolExecutionFailed, cause).
		WithDetail("tool_name", toolName)
}

func NewMissingParameterError(toolName string, paramName string) *errx.Error {
	return errRegistry.New(ErrCodeMissingParameter).
		WithDetail("tool_name", toolName).
		WithDetail("parameter", paramName)
}

func NewParameterResolutionError(toolName string, paramName string, cause error) *errx.Error {
	return errRegistry.NewWithCause(ErrCodeParameterResolution, cause).
		WithDetail("tool_name", toolName).
		WithDetail("parameter", paramName)
}

func NewToolTimeoutError(toolName string) *errx.Error {
	return errRegistry.New(ErrCodeToolTimeout).
		WithDetail("tool_name", toolName)
}
