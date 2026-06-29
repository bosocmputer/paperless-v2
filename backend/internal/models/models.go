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

type SignatureDesignerViewport struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type SignatureDesignerEventRequest struct {
	Event                string                    `json:"event"`
	SessionID            string                    `json:"sessionId"`
	DocFormatCode        string                    `json:"docFormatCode"`
	PositionCode         string                    `json:"positionCode"`
	ConditionType        int                       `json:"conditionType"`
	ElapsedMS            int64                     `json:"elapsedMs"`
	BoxCount             int                       `json:"boxCount"`
	ValidationIssueCount int                       `json:"validationIssueCount"`
	Viewport             SignatureDesignerViewport `json:"viewport"`
}

type SignatureValidationIssue struct {
	Code         string `json:"code"`
	PositionCode string `json:"positionCode,omitempty"`
	Message      string `json:"message"`
}

type SMLDocumentCandidate struct {
	DocNo         string  `json:"doc_no"`
	DocDate       string  `json:"doc_date"`
	DocFormatCode string  `json:"doc_format_code"`
	TransFlag     int     `json:"trans_flag"`
	Table         string  `json:"table"`
	PartyCode     string  `json:"party_code"`
	PartyName     string  `json:"party_name"`
	PartyType     string  `json:"party_type"`
	TotalAmount   float64 `json:"total_amount"`
	IsLockRecord  int     `json:"is_lock_record"`
}

type SigningDocument struct {
	ID                  string                      `json:"id"`
	ScreenCode          string                      `json:"screenCode"`
	DocFormatCode       string                      `json:"docFormatCode"`
	DocNo               string                      `json:"docNo"`
	SMLTable            string                      `json:"smlTable"`
	TransFlag           int                         `json:"transFlag"`
	PartyCode           string                      `json:"partyCode"`
	PartyName           string                      `json:"partyName"`
	PartyType           string                      `json:"partyType"`
	DocDate             string                      `json:"docDate"`
	TotalAmount         float64                     `json:"totalAmount"`
	SMLIsLockRecord     int                         `json:"smlIsLockRecord"`
	Status              string                      `json:"status"`
	CurrentVersion      int                         `json:"currentVersion"`
	OriginalFileID      string                      `json:"originalFileId"`
	CurrentFileID       string                      `json:"currentFileId"`
	FinalFileID         string                      `json:"finalFileId"`
	SignatureTemplateID string                      `json:"signatureTemplateId"`
	CreatedBy           string                      `json:"createdBy"`
	CreatedAt           time.Time                   `json:"createdAt"`
	UpdatedAt           time.Time                   `json:"updatedAt"`
	CompletedAt         *time.Time                  `json:"completedAt,omitempty"`
	LockedAt            *time.Time                  `json:"lockedAt,omitempty"`
	OriginalFile        *UploadedFile               `json:"originalFile,omitempty"`
	CurrentFile         *UploadedFile               `json:"currentFile,omitempty"`
	FinalFile           *UploadedFile               `json:"finalFile,omitempty"`
	Steps               []SigningDocumentStep       `json:"steps,omitempty"`
	Signers             []SigningDocumentSigner     `json:"signers,omitempty"`
	Events              []SigningDocumentEvent      `json:"events,omitempty"`
	Attachments         []SigningDocumentAttachment `json:"attachments,omitempty"`
}

type SigningDocumentStep struct {
	ID            string     `json:"id"`
	DocumentID    string     `json:"documentId"`
	PositionCode  string     `json:"positionCode"`
	PositionName  string     `json:"positionName"`
	SequenceNo    float64    `json:"sequenceNo"`
	ConditionType int        `json:"conditionType"`
	User01        string     `json:"user01"`
	User02        string     `json:"user02"`
	User03        string     `json:"user03"`
	Status        string     `json:"status"`
	CompletedAt   *time.Time `json:"completedAt,omitempty"`
}

type SigningDocumentSigner struct {
	ID              string     `json:"id"`
	DocumentID      string     `json:"documentId"`
	StepID          string     `json:"stepId"`
	PositionCode    string     `json:"positionCode"`
	PositionName    string     `json:"positionName"`
	SequenceNo      float64    `json:"sequenceNo"`
	ConditionType   int        `json:"conditionType"`
	SignerSlot      int        `json:"signerSlot"`
	SignerType      string     `json:"signerType"`
	SignerUser      string     `json:"signerUser"`
	SignerName      string     `json:"signerName"`
	Status          string     `json:"status"`
	PageNo          int        `json:"pageNo"`
	XRatio          float64    `json:"xRatio"`
	YRatio          float64    `json:"yRatio"`
	WidthRatio      float64    `json:"widthRatio"`
	HeightRatio     float64    `json:"heightRatio"`
	Label           string     `json:"label"`
	SignatureFileID string     `json:"signatureFileId"`
	SignedAt        *time.Time `json:"signedAt,omitempty"`
	RejectedAt      *time.Time `json:"rejectedAt,omitempty"`
	RejectReason    string     `json:"rejectReason"`
	DeviceID        string     `json:"deviceId"`
	IPAddress       string     `json:"ipAddress"`
	UserAgent       string     `json:"userAgent"`
	ExternalTokenID string     `json:"externalTokenId"`
	ExternalURL     string     `json:"externalUrl,omitempty"`
}

type SigningDocumentEvent struct {
	ID          string         `json:"id"`
	DocumentID  string         `json:"documentId"`
	ActorUserID string         `json:"actorUserId"`
	ActorLabel  string         `json:"actorLabel"`
	Action      string         `json:"action"`
	Message     string         `json:"message"`
	IPAddress   string         `json:"ipAddress"`
	UserAgent   string         `json:"userAgent"`
	Metadata    map[string]any `json:"metadata"`
	CreatedAt   time.Time      `json:"createdAt"`
}

type SigningDocumentAttachment struct {
	ID         string       `json:"id"`
	DocumentID string       `json:"documentId"`
	SignerID   string       `json:"signerId"`
	FileID     string       `json:"fileId"`
	Note       string       `json:"note"`
	CreatedBy  string       `json:"createdBy"`
	CreatedAt  time.Time    `json:"createdAt"`
	File       UploadedFile `json:"file"`
}

type CreateSigningDocumentRequest struct {
	DocFormatCode string `json:"docFormatCode"`
	DocNo         string `json:"docNo"`
}

type SignTaskRequest struct {
	SignatureDataURL string `json:"signatureDataUrl"`
	DeviceID         string `json:"deviceId"`
	LegalText        string `json:"legalText"`
	LegalAccepted    bool   `json:"legalAccepted"`
}

type RejectTaskRequest struct {
	Reason   string `json:"reason"`
	DeviceID string `json:"deviceId"`
}

type SigningTaskEventRequest struct {
	Event         string                    `json:"event"`
	SessionID     string                    `json:"sessionId"`
	ElapsedMS     int64                     `json:"elapsedMs"`
	Viewport      SignatureDesignerViewport `json:"viewport"`
	PDFPage       int                       `json:"pdfPage"`
	PDFPageCount  int                       `json:"pdfPageCount"`
	AttachmentCnt int                       `json:"attachmentCount"`
	ErrorCode     string                    `json:"errorCode"`
}

type VerifyExternalOTPRequest struct {
	OTP string `json:"otp"`
}

type VerifyExternalOTPResponse struct {
	SessionToken string    `json:"sessionToken"`
	ExpiresAt    time.Time `json:"expiresAt"`
}

type RegenerateExternalTokenResponse struct {
	SignerID  string    `json:"signerId"`
	URL       string    `json:"url"`
	OTP       string    `json:"otp"`
	ExpiresAt time.Time `json:"expiresAt"`
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
