package service

import "fmt"

const (
	TheaterErrorAuthRequired            = "AUTH_REQUIRED"
	TheaterErrorWorldNotFound           = "WORLD_NOT_FOUND"
	TheaterErrorChannelNotFound         = "CHANNEL_NOT_FOUND"
	TheaterErrorChannelWorldMismatch    = "CHANNEL_WORLD_MISMATCH"
	TheaterErrorPermissionDenied        = "STAGE_PERMISSION_DENIED"
	TheaterErrorActivationRequired      = "THEATER_ACTIVATION_REQUIRED"
	TheaterErrorNotFound                = "STAGE_NOT_FOUND"
	TheaterErrorRevisionConflict        = "STAGE_REVISION_CONFLICT"
	TheaterErrorMutationIDReused        = "MUTATION_ID_REUSED"
	TheaterErrorMutationTypeUnsupported = "MUTATION_TYPE_UNSUPPORTED"
	TheaterErrorPayloadInvalid          = "MUTATION_PAYLOAD_INVALID"
	TheaterErrorLimitExceeded           = "STAGE_LIMIT_EXCEEDED"
	TheaterErrorHistoryExpired          = "STAGE_EVENT_HISTORY_EXPIRED"
	TheaterErrorSchemaUnsupported       = "STAGE_SCHEMA_UNSUPPORTED"
	TheaterErrorResourceNotFound        = "RESOURCE_NOT_FOUND"
	TheaterErrorResourceInUse           = "RESOURCE_IN_USE"
	TheaterErrorResourceNotReady        = "RESOURCE_NOT_READY"
	TheaterErrorResourceLimitExceeded   = "RESOURCE_LIMIT_EXCEEDED"
	TheaterErrorInternal                = "INTERNAL_ERROR"
)

type TheaterError struct {
	Code       string
	Message    string
	Details    map[string]any
	HTTPStatus int
}

func (e *TheaterError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func newTheaterError(code, message string, status int, details map[string]any) *TheaterError {
	return &TheaterError{Code: code, Message: message, HTTPStatus: status, Details: details}
}

func IsTheaterErrorCode(err error, code string) bool {
	theaterErr, ok := err.(*TheaterError)
	return ok && theaterErr.Code == code
}

func NewTheaterPayloadErrorForAPI(message string) error {
	return theaterPayloadError(message)
}
