package apperr

import (
	"fmt"
	"net/http"
)

type ErrorType string

const (
	TypeNotFound         ErrorType = "NOT_FOUND"
	TypeBadRequest       ErrorType = "BAD_REQUEST"
	TypePreconditionFail ErrorType = "PRECONDITION_FAILED"
	TypeInternal         ErrorType = "INTERNAL_SERVER_ERROR"
)

// AppError es el error estandarizado de la plataforma
type AppError struct {
	Type    ErrorType `json:"type"`
	Message string    `json:"message"`
	Err     error     `json:"-"` // Error interno subyacente (para logging)
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Type, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s", e.Type, e.Message)
}

func (e *AppError) Unwrap() error {
	return e.Err
}

// Constructores semánticos limpios

func NewNotFound(msg string, err error) error {
	return &AppError{Type: TypeNotFound, Message: msg, Err: err}
}

func NewBadRequest(msg string, err error) error {
	return &AppError{Type: TypeBadRequest, Message: msg, Err: err}
}

func NewPreconditionFailed(msg string, err error) error {
	return &AppError{Type: TypePreconditionFail, Message: msg, Err: err}
}

func NewInternal(msg string, err error) error {
	return &AppError{Type: TypeInternal, Message: msg, Err: err}
}

// HTTPStatusCode traduce el error de dominio a un código HTTP nativo
func HTTPStatusCode(err error) int {
	if err == nil {
		return http.StatusOK
	}

	var appErr *AppError
	if stdErr, ok := err.(*AppError); ok {
		appErr = stdErr
	} else {
		return http.StatusInternalServerError
	}

	switch appErr.Type {
	case TypeNotFound:
		return http.StatusNotFound
	case TypeBadRequest:
		return http.StatusBadRequest
	case TypePreconditionFail:
		return http.StatusPreconditionFailed
	case TypeInternal:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}
