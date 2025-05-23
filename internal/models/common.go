package models

type APIResponse[T any] struct {
	Result  T      `json:"result" validate:"required"`
	Message string `json:"message,omitempty"`
}
