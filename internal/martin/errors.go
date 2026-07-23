package martin

import (
	"errors"
	"fmt"

	"github.com/kyle-visner/jaybase"
)

type ErrorCode string

const (
	ErrValidation ErrorCode = "validation_error"
	ErrNotFound   ErrorCode = "not_found"
	ErrConflict   ErrorCode = "conflict"
	ErrPermission ErrorCode = "permission_denied"
	ErrIntegrity  ErrorCode = "integrity_error"
	ErrCapacity   ErrorCode = "capacity_exceeded"
	ErrInternal   ErrorCode = "internal_error"
)

type AppError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
}

func (e *AppError) Error() string {
	return e.Message
}

func appErr(code ErrorCode, format string, args ...any) error {
	return &AppError{Code: code, Message: fmt.Sprintf(format, args...)}
}

func storageError(err error) error {
	if err == nil {
		return nil
	}
	var dbErr *jaybase.AppError
	if !errors.As(err, &dbErr) {
		return err
	}
	code := ErrInternal
	switch dbErr.Code {
	case jaybase.ErrValidation:
		code = ErrValidation
	case jaybase.ErrNotFound:
		code = ErrNotFound
	case jaybase.ErrConflict:
		code = ErrConflict
	case jaybase.ErrPermission:
		code = ErrPermission
	case jaybase.ErrIntegrity:
		code = ErrIntegrity
	case jaybase.ErrCapacity:
		code = ErrCapacity
	}
	return &AppError{Code: code, Message: dbErr.Message}
}
