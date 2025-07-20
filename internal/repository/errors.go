package repository

import (
	"errors"
)

var (
	ErrSecretNotFound         = errors.New("synced secret not found")
	ErrDatabaseUnavailable    = errors.New("database is unavailable")
	ErrDatabaseGeneric        = errors.New("database error occurred while processing request")
	ErrInvalidQueryParameters = errors.New("invalid query parameters provided for synced secret operation")
)