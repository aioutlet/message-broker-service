package models

import "errors"

var (
	// ErrInvalidTopic is returned when topic is empty or invalid
	ErrInvalidTopic = errors.New("invalid topic")

	// ErrEmptyData is returned when message data is empty
	ErrEmptyData = errors.New("message data cannot be empty")

	// ErrBrokerNotConnected is returned when broker is not connected
	ErrBrokerNotConnected = errors.New("broker not connected")

	// ErrPublishFailed is returned when message publishing fails
	ErrPublishFailed = errors.New("failed to publish message")

	// ErrInvalidRequest is returned when request validation fails
	ErrInvalidRequest = errors.New("invalid request")

	// ErrUnauthorized is returned when API key is invalid
	ErrUnauthorized = errors.New("unauthorized")

	// ErrRateLimitExceeded is returned when rate limit is exceeded
	ErrRateLimitExceeded = errors.New("rate limit exceeded")
)
