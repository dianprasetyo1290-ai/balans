// Package apperr mendefinisikan error yang ditujukan ke user beserta kode stabilnya.
package apperr

import (
	"errors"
	"fmt"
)

// Error adalah error yang aman dikirim ke client. Err adalah penyebab internal dan tidak pernah dikirim.
type Error struct {
	Code    Code
	Message string
	Fields  map[string]string
	Err     error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.Err }

// New membuat error dengan pesan default kode.
func New(code Code) *Error {
	return &Error{Code: code, Message: code.DefaultMessage()}
}

// WithMessage membuat error dengan pesan khusus untuk user.
func WithMessage(code Code, message string) *Error {
	return &Error{Code: code, Message: message}
}

// Wrap membuat error dengan penyebab internal.
func Wrap(code Code, err error) *Error {
	return &Error{Code: code, Message: code.DefaultMessage(), Err: err}
}

// Validation membuat VALIDATION_FAILED dengan error per field.
func Validation(fields map[string]string) *Error {
	return &Error{Code: CodeValidationFailed, Message: CodeValidationFailed.DefaultMessage(), Fields: fields}
}

// InvalidStateTransition dipakai tabel transisi status dokumen (Technical Guideline 5.4).
func InvalidStateTransition(entity, from, to string) *Error {
	return &Error{
		Code:    CodeInvalidStateTransition,
		Message: CodeInvalidStateTransition.DefaultMessage(),
		Err:     fmt.Errorf("%s: transition %s -> %s not allowed", entity, from, to),
	}
}

// As mengambil *Error dari rantai error. Error lain dianggap INTERNAL_ERROR.
func As(err error) *Error {
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr
	}
	return Wrap(CodeInternal, err)
}

// HasCode memeriksa apakah rantai error memuat kode tertentu.
func HasCode(err error, code Code) bool {
	var appErr *Error
	return errors.As(err, &appErr) && appErr.Code == code
}
