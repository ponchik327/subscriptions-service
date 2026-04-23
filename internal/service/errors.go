package service

import "errors"

var (
	ErrNotFound      = errors.New("not found")
	ErrInvalidPeriod = errors.New("from must be <= to")
)
