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

type CreateUserRequest struct {
	DisplayName string `json:"displayName"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	Role        string `json:"role"`
	Status      string `json:"status"`
}

type UpdateUserRequest struct {
	DisplayName string `json:"displayName"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	Role        string `json:"role"`
	Status      string `json:"status"`
}

type SeedUser struct {
	DisplayName string
	Username    string
	Password    string
	Role        string
}

type SMLDocFormat struct {
	Code       string `json:"code"`
	Name1      string `json:"name_1"`
	Name2      string `json:"name_2"`
	Format     string `json:"format"`
	ScreenCode string `json:"screen_code"`
}

type DocumentConfigStep struct {
	ID            string    `json:"id"`
	ScreenCode    string    `json:"screenCode"`
	DocFormatCode string    `json:"docFormatCode"`
	PositionCode  string    `json:"positionCode"`
	PositionName  string    `json:"positionName"`
	User01        string    `json:"user01"`
	User02        string    `json:"user02"`
	User03        string    `json:"user03"`
	SequenceNo    float64   `json:"sequenceNo"`
	ConditionType int       `json:"conditionType"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type DocumentConfigStepRequest struct {
	ScreenCode    string  `json:"screenCode"`
	DocFormatCode string  `json:"docFormatCode"`
	PositionCode  string  `json:"positionCode"`
	PositionName  string  `json:"positionName"`
	User01        string  `json:"user01"`
	User02        string  `json:"user02"`
	User03        string  `json:"user03"`
	SequenceNo    float64 `json:"sequenceNo"`
	ConditionType int     `json:"conditionType"`
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
