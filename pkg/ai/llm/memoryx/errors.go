package memoryx

import (
	"github.com/Abraxas-365/ams/pkg/errx"
	"net/http"
)

var errRegistry = errx.NewRegistry("MEMORY")

var (
	ErrCodeSessionNotFound = errRegistry.Register(
		"SESSION_NOT_FOUND",
		errx.TypeNotFound,
		http.StatusNotFound,
		"Session not found",
	)

	ErrCodeSessionInactive = errRegistry.Register(
		"SESSION_INACTIVE",
		errx.TypeBusiness,
		http.StatusGone,
		"Session is inactive",
	)

	ErrCodeMessageSerializationFailed = errRegistry.Register(
		"MESSAGE_SERIALIZATION_FAILED",
		errx.TypeInternal,
		http.StatusInternalServerError,
		"Failed to serialize message",
	)
)

func ErrSessionNotFound() *errx.Error {
	return errRegistry.New(ErrCodeSessionNotFound)
}

func ErrSessionInactive() *errx.Error {
	return errRegistry.New(ErrCodeSessionInactive)
}

func ErrMessageSerializationFailed(cause error) *errx.Error {
	return errRegistry.NewWithCause(ErrCodeMessageSerializationFailed, cause)
}
