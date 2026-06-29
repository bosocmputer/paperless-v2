package models

import "time"

type User struct {
	ID           string    `json:"id"`
	DisplayName  string    `json:"displayName"`
	Username     string    `json:"username"`
	Role         string    `json:"role"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"createdAt"`
	PasswordHash string    `json:"-"`
}

type SeedUser struct {
	DisplayName string
	Username    string
	Password    string
	Role        string
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token     string    `json:"token"`
	TokenType string    `json:"tokenType"`
	ExpiresAt time.Time `json:"expiresAt"`
	User      User      `json:"user"`
}

type APIError struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}
