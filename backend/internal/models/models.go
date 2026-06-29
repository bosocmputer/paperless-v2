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

type SMLScreenCode struct {
	Code  string `json:"code"`
	Count int    `json:"count"`
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

type UploadedFile struct {
	ID           string    `json:"id"`
	OriginalName string    `json:"originalName"`
	StoredName   string    `json:"storedName"`
	StoragePath  string    `json:"-"`
	ContentType  string    `json:"contentType"`
	SizeBytes    int64     `json:"sizeBytes"`
	PageCount    int       `json:"pageCount"`
	SHA256       string    `json:"sha256"`
	CreatedBy    string    `json:"createdBy"`
	CreatedAt    time.Time `json:"createdAt"`
}

type SignatureTemplate struct {
	ID            string                 `json:"id"`
	ScreenCode    string                 `json:"screenCode"`
	DocFormatCode string                 `json:"docFormatCode"`
	Version       int                    `json:"version"`
	Status        string                 `json:"status"`
	SampleFileID  string                 `json:"sampleFileId"`
	SampleFile    *UploadedFile          `json:"sampleFile,omitempty"`
	Revision      int                    `json:"revision"`
	CreatedBy     string                 `json:"createdBy"`
	PublishedBy   string                 `json:"publishedBy"`
	CreatedAt     time.Time              `json:"createdAt"`
	UpdatedAt     time.Time              `json:"updatedAt"`
	PublishedAt   *time.Time             `json:"publishedAt,omitempty"`
	Boxes         []SignatureTemplateBox `json:"boxes"`
}

type SignatureTemplateBox struct {
	ID           string    `json:"id"`
	TemplateID   string    `json:"templateId"`
	PositionCode string    `json:"positionCode"`
	SignerSlot   int       `json:"signerSlot"`
	SignerType   string    `json:"signerType"`
	SignerUser   string    `json:"signerUser"`
	PageNo       int       `json:"pageNo"`
	XRatio       float64   `json:"xRatio"`
	YRatio       float64   `json:"yRatio"`
	WidthRatio   float64   `json:"widthRatio"`
	HeightRatio  float64   `json:"heightRatio"`
	Label        string    `json:"label"`
	CreatedAt    time.Time `json:"createdAt"`
}

type SignatureTemplateBoxRequest struct {
	PositionCode string  `json:"positionCode"`
	SignerSlot   int     `json:"signerSlot"`
	SignerType   string  `json:"signerType"`
	SignerUser   string  `json:"signerUser"`
	PageNo       int     `json:"pageNo"`
	XRatio       float64 `json:"xRatio"`
	YRatio       float64 `json:"yRatio"`
	WidthRatio   float64 `json:"widthRatio"`
	HeightRatio  float64 `json:"heightRatio"`
	Label        string  `json:"label"`
}

type SaveSignatureBoxesRequest struct {
	Revision int                           `json:"revision"`
	Boxes    []SignatureTemplateBoxRequest `json:"boxes"`
}

type SignatureValidationIssue struct {
	Code         string `json:"code"`
	PositionCode string `json:"positionCode,omitempty"`
	Message      string `json:"message"`
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
