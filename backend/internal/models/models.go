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
	SMLTenant     string    `json:"smlTenant"`
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

type DocumentConfigWorkflowSummary struct {
	DocFormatCode   string         `json:"docFormatCode"`
	ScreenCode      string         `json:"screenCode"`
	DocFormat       SMLDocFormat   `json:"docFormat"`
	StepCount       int            `json:"stepCount"`
	UserCount       int            `json:"userCount"`
	ConditionCounts map[string]int `json:"conditionCounts"`
	WarningCount    int            `json:"warningCount"`
	UpdatedAt       *time.Time     `json:"updatedAt,omitempty"`
	Revision        string         `json:"revision"`
}

type DocumentConfigPresetWarning struct {
	Code         string `json:"code"`
	PositionCode string `json:"positionCode"`
	BoxCount     int    `json:"boxCount"`
	Message      string `json:"message"`
}

type DocumentConfigWorkflow struct {
	DocFormat      SMLDocFormat                  `json:"docFormat"`
	Steps          []DocumentConfigStep          `json:"steps"`
	Revision       string                        `json:"revision"`
	PresetWarnings []DocumentConfigPresetWarning `json:"presetWarnings"`
}

type DocumentConfigWorkflowSaveRequest struct {
	Revision string                      `json:"revision"`
	Steps    []DocumentConfigStepRequest `json:"steps"`
}

type DocumentConfigWorkflowCopyRequest struct {
	SourceDocFormatCode string `json:"sourceDocFormatCode"`
	Revision            string `json:"revision"`
}

type DocumentConfigWorkflowEventRequest struct {
	Event                string `json:"event"`
	SessionID            string `json:"sessionId"`
	StepCount            int    `json:"stepCount"`
	ValidationIssueCount int    `json:"validationIssueCount"`
	ElapsedMs            int64  `json:"elapsedMs"`
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
	ID             string                 `json:"id"`
	SMLTenant      string                 `json:"smlTenant"`
	ScreenCode     string                 `json:"screenCode"`
	DocFormatCode  string                 `json:"docFormatCode"`
	Version        int                    `json:"version"`
	Status         string                 `json:"status"`
	SampleFileID   string                 `json:"sampleFileId"`
	SampleFile     *UploadedFile          `json:"sampleFile,omitempty"`
	Revision       int                    `json:"revision"`
	CreatedBy      string                 `json:"createdBy"`
	PublishedBy    string                 `json:"publishedBy"`
	CreatedAt      time.Time              `json:"createdAt"`
	UpdatedAt      time.Time              `json:"updatedAt"`
	PublishedAt    *time.Time             `json:"publishedAt,omitempty"`
	Boxes          []SignatureTemplateBox `json:"boxes"`
	LegalNoticeBox *LegalNoticeBox        `json:"legalNoticeBox,omitempty"`
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

type LegalNoticeBox struct {
	PageNo      int     `json:"pageNo"`
	XRatio      float64 `json:"xRatio"`
	YRatio      float64 `json:"yRatio"`
	WidthRatio  float64 `json:"widthRatio"`
	HeightRatio float64 `json:"heightRatio"`
	Label       string  `json:"label"`
	Source      string  `json:"source,omitempty"`
}

type LegalNoticeBoxRequest struct {
	PageNo      int     `json:"pageNo"`
	XRatio      float64 `json:"xRatio"`
	YRatio      float64 `json:"yRatio"`
	WidthRatio  float64 `json:"widthRatio"`
	HeightRatio float64 `json:"heightRatio"`
	Label       string  `json:"label"`
	Source      string  `json:"source"`
}

type LegalNoticeSnapshot struct {
	Text        string  `json:"text"`
	TextVersion string  `json:"textVersion"`
	Source      string  `json:"source"`
	PageNo      int     `json:"pageNo"`
	XRatio      float64 `json:"xRatio"`
	YRatio      float64 `json:"yRatio"`
	WidthRatio  float64 `json:"widthRatio"`
	HeightRatio float64 `json:"heightRatio"`
	Label       string  `json:"label"`
}

type SignaturePlacementSnapshot struct {
	PositionCode  string  `json:"positionCode"`
	PositionName  string  `json:"positionName"`
	SequenceNo    float64 `json:"sequenceNo"`
	ConditionType int     `json:"conditionType"`
	SignerSlot    int     `json:"signerSlot"`
	SignerType    string  `json:"signerType"`
	SignerUser    string  `json:"signerUser"`
	SignerName    string  `json:"signerName"`
	PageNo        int     `json:"pageNo"`
	XRatio        float64 `json:"xRatio"`
	YRatio        float64 `json:"yRatio"`
	WidthRatio    float64 `json:"widthRatio"`
	HeightRatio   float64 `json:"heightRatio"`
	Label         string  `json:"label"`
}

type SaveSignatureBoxesRequest struct {
	Revision       int                           `json:"revision"`
	Boxes          []SignatureTemplateBoxRequest `json:"boxes"`
	LegalNoticeBox *LegalNoticeBoxRequest        `json:"legalNoticeBox"`
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

type SMLRelatedDocumentsGraph struct {
	Root      SMLRelatedDocumentNode      `json:"root"`
	Nodes     []SMLRelatedDocumentNode    `json:"nodes"`
	Edges     []SMLRelatedDocumentEdge    `json:"edges"`
	Warnings  []SMLRelatedDocumentWarning `json:"warnings,omitempty"`
	Depth     int                         `json:"depth"`
	Truncated bool                        `json:"truncated"`
}

type SMLRelatedDocumentNode struct {
	DocNo               string                     `json:"doc_no"`
	DocDate             string                     `json:"doc_date"`
	DocTime             string                     `json:"doc_time,omitempty"`
	DocFormatCode       string                     `json:"doc_format_code"`
	DocFormatName       string                     `json:"doc_format_name,omitempty"`
	TransFlag           int                        `json:"trans_flag"`
	TransFlagMenu       string                     `json:"trans_flag_menu,omitempty"`
	TransFlagNameTH     string                     `json:"trans_flag_name_th,omitempty"`
	TransFlagNameEN     string                     `json:"trans_flag_name_en,omitempty"`
	TransType           int                        `json:"trans_type,omitempty"`
	Table               string                     `json:"table"`
	PartyCode           string                     `json:"party_code"`
	PartyName           string                     `json:"party_name"`
	PartyType           string                     `json:"party_type"`
	TotalAmount         float64                    `json:"total_amount"`
	SourceDocNo         string                     `json:"source_doc_no,omitempty"`
	IsLockRecord        int                        `json:"is_lock_record"`
	PaperlessDocumentID string                     `json:"paperlessDocumentId,omitempty"`
	PaperlessStatus     string                     `json:"paperlessStatus,omitempty"`
	CanOpenPaperless    bool                       `json:"canOpenPaperless"`
	HasCurrentPDF       bool                       `json:"hasCurrentPdf"`
	HasFinalPDF         bool                       `json:"hasFinalPdf"`
	CanViewCurrentPDF   bool                       `json:"canViewCurrentPdf"`
	CanViewSignedPDF    bool                       `json:"canViewSignedPdf"`
	CurrentPDFURL       string                     `json:"currentPdfUrl,omitempty"`
	SignedPDFURL        string                     `json:"signedPdfUrl,omitempty"`
	MatchCount          int                        `json:"matchCount"`
	PaperlessMatches    []SigningDocumentReference `json:"paperlessMatches,omitempty"`
}

type SMLRelatedDocumentEdge struct {
	FromDocNo    string `json:"from_doc_no"`
	ToDocNo      string `json:"to_doc_no"`
	Relation     string `json:"relation"`
	SourceTable  string `json:"source_table"`
	SourceColumn string `json:"source_column"`
}

type SMLRelatedDocumentWarning struct {
	Code    string `json:"code"`
	DocNo   string `json:"doc_no,omitempty"`
	Message string `json:"message"`
}

type SigningDocumentReference struct {
	ID                string    `json:"id"`
	DocNo             string    `json:"docNo"`
	DocFormatCode     string    `json:"docFormatCode"`
	Status            string    `json:"status"`
	HasCurrentPDF     bool      `json:"hasCurrentPdf"`
	HasFinalPDF       bool      `json:"hasFinalPdf"`
	CanOpenPaperless  bool      `json:"canOpenPaperless"`
	CanViewCurrentPDF bool      `json:"canViewCurrentPdf"`
	CanViewSignedPDF  bool      `json:"canViewSignedPdf"`
	CurrentPDFURL     string    `json:"currentPdfUrl,omitempty"`
	SignedPDFURL      string    `json:"signedPdfUrl,omitempty"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

type SigningDocument struct {
	ID                  string                       `json:"id"`
	SMLTenant           string                       `json:"smlTenant"`
	SMLDataGroup        string                       `json:"smlDataGroup"`
	SMLDataCode         string                       `json:"smlDataCode"`
	ScreenCode          string                       `json:"screenCode"`
	DocFormatCode       string                       `json:"docFormatCode"`
	DocNo               string                       `json:"docNo"`
	SMLTable            string                       `json:"smlTable"`
	TransFlag           int                          `json:"transFlag"`
	PartyCode           string                       `json:"partyCode"`
	PartyName           string                       `json:"partyName"`
	PartyType           string                       `json:"partyType"`
	DocDate             string                       `json:"docDate"`
	TotalAmount         float64                      `json:"totalAmount"`
	SMLIsLockRecord     int                          `json:"smlIsLockRecord"`
	Status              string                       `json:"status"`
	CurrentVersion      int                          `json:"currentVersion"`
	OriginalFileID      string                       `json:"originalFileId"`
	CurrentFileID       string                       `json:"currentFileId"`
	FinalFileID         string                       `json:"finalFileId"`
	SignatureTemplateID string                       `json:"signatureTemplateId"`
	CreatedBy           string                       `json:"createdBy"`
	CreatedAt           time.Time                    `json:"createdAt"`
	UpdatedAt           time.Time                    `json:"updatedAt"`
	CompletedAt         *time.Time                   `json:"completedAt,omitempty"`
	LockedAt            *time.Time                   `json:"lockedAt,omitempty"`
	LegalNoticeSnapshot *LegalNoticeSnapshot         `json:"legalNoticeSnapshot,omitempty"`
	LegalNoticeBoxes    []LegalNoticeSnapshot        `json:"legalNoticeBoxes,omitempty"`
	SignaturePlacements []SignaturePlacementSnapshot `json:"signaturePlacements,omitempty"`
	OriginalFile        *UploadedFile                `json:"originalFile,omitempty"`
	CurrentFile         *UploadedFile                `json:"currentFile,omitempty"`
	FinalFile           *UploadedFile                `json:"finalFile,omitempty"`
	Steps               []SigningDocumentStep        `json:"steps,omitempty"`
	Signers             []SigningDocumentSigner      `json:"signers,omitempty"`
	PendingSigners      []SigningDocumentSigner      `json:"pendingSigners,omitempty"`
	Events              []SigningDocumentEvent       `json:"events,omitempty"`
	Attachments         []SigningDocumentAttachment  `json:"attachments,omitempty"`
	PrintEvents         []SigningDocumentPrintEvent  `json:"printEvents,omitempty"`
}

type AdminDashboard struct {
	Totals            SigningDocumentTotals             `json:"totals"`
	RecentDocuments   []SigningDocument                 `json:"recentDocuments"`
	NeedsAttention    []SigningDocument                 `json:"needsAttention"`
	WorkflowSummary   AdminDashboardWorkflowSummary     `json:"workflowSummary"`
	PendingByPosition []AdminDashboardPendingByPosition `json:"pendingByPosition"`
	PendingDocuments  []AdminDashboardPendingDocument   `json:"pendingDocuments"`
}

type SigningDocumentTotals struct {
	Total                   int `json:"total"`
	Draft                   int `json:"draft"`
	InProgress              int `json:"inProgress"`
	PendingConfirm          int `json:"pendingConfirm"`
	Rejected                int `json:"rejected"`
	Completed               int `json:"completed"`
	CompletedEvidenceFailed int `json:"completedEvidenceFailed"`
	CompletedImageFailed    int `json:"completedImageFailed"`
	CompletedLockFailed     int `json:"completedLockFailed"`
	Cancelled               int `json:"cancelled"`
}

type AdminDashboardWorkflowSummary struct {
	PendingDocuments   int `json:"pendingDocuments"`
	PendingSigners     int `json:"pendingSigners"`
	PendingConfirm     int `json:"pendingConfirm"`
	AttentionDocuments int `json:"attentionDocuments"`
	CompletedDocuments int `json:"completedDocuments"`
	EvidenceFailed     int `json:"evidenceFailed"`
	ImageFailed        int `json:"imageFailed"`
	LockFailed         int `json:"lockFailed"`
}

type AdminDashboardPendingByPosition struct {
	PositionCode  string `json:"positionCode"`
	PositionName  string `json:"positionName"`
	ConditionType int    `json:"conditionType"`
	DocumentCount int    `json:"documentCount"`
	SignerCount   int    `json:"signerCount"`
}

type AdminDashboardPendingDocument struct {
	ID                  string    `json:"id"`
	DocNo               string    `json:"docNo"`
	DocFormatCode       string    `json:"docFormatCode"`
	PartyName           string    `json:"partyName"`
	PartyCode           string    `json:"partyCode"`
	CurrentPositionName string    `json:"currentPositionName"`
	PendingSignerCount  int       `json:"pendingSignerCount"`
	UpdatedAt           time.Time `json:"updatedAt"`
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

type MySigningTaskQueue struct {
	Documents        []MySigningTaskDocument `json:"documents"`
	WaitingDocuments []MySigningTaskDocument `json:"waitingDocuments"`
	Counts           MySigningTaskCounts     `json:"counts"`
	Pagination       MySigningTaskPagination `json:"pagination"`
}

type MySigningTaskCounts struct {
	Ready   int `json:"ready"`
	Waiting int `json:"waiting"`
}

type MySigningTaskPagination struct {
	Ready   PageMeta `json:"ready"`
	Waiting PageMeta `json:"waiting"`
}

type PageMeta struct {
	Page    int  `json:"page"`
	Size    int  `json:"size"`
	HasMore bool `json:"hasMore"`
}

type MySigningTaskDocument struct {
	ID            string                 `json:"id"`
	DocNo         string                 `json:"docNo"`
	DocFormatCode string                 `json:"docFormatCode"`
	PartyCode     string                 `json:"partyCode"`
	PartyName     string                 `json:"partyName"`
	DocDate       string                 `json:"docDate"`
	TotalAmount   float64                `json:"totalAmount"`
	Status        string                 `json:"status"`
	UpdatedAt     time.Time              `json:"updatedAt"`
	Task          MySigningTaskSigner    `json:"task"`
	Signers       []MySigningTaskSigner  `json:"signers,omitempty"`
	BlockedBy     []MySigningTaskBlocker `json:"blockedBy,omitempty"`
	BlockSummary  string                 `json:"blockSummary,omitempty"`
}

type MySigningTaskSigner struct {
	ID            string     `json:"id"`
	DocumentID    string     `json:"documentId"`
	StepID        string     `json:"stepId"`
	PositionCode  string     `json:"positionCode"`
	PositionName  string     `json:"positionName"`
	SequenceNo    float64    `json:"sequenceNo"`
	ConditionType int        `json:"conditionType"`
	SignerSlot    int        `json:"signerSlot"`
	SignerType    string     `json:"signerType"`
	SignerUser    string     `json:"signerUser"`
	SignerName    string     `json:"signerName"`
	Status        string     `json:"status"`
	SignedAt      *time.Time `json:"signedAt,omitempty"`
	RejectedAt    *time.Time `json:"rejectedAt,omitempty"`
}

type MySigningTaskBlocker struct {
	PositionCode  string                `json:"positionCode"`
	PositionName  string                `json:"positionName"`
	SequenceNo    float64               `json:"sequenceNo"`
	ConditionType int                   `json:"conditionType"`
	Status        string                `json:"status"`
	Signers       []MySigningTaskSigner `json:"signers"`
	Summary       string                `json:"summary"`
}

type MySigningHistoryResult struct {
	Documents []MySigningHistoryDocument `json:"documents"`
	Page      int                        `json:"page"`
	Size      int                        `json:"size"`
	Total     int                        `json:"total"`
	HasMore   bool                       `json:"hasMore"`
}

type MySigningHistoryDocument struct {
	ID             string     `json:"id"`
	DocNo          string     `json:"docNo"`
	DocFormatCode  string     `json:"docFormatCode"`
	PartyCode      string     `json:"partyCode"`
	PartyName      string     `json:"partyName"`
	DocDate        string     `json:"docDate"`
	TotalAmount    float64    `json:"totalAmount"`
	DocumentStatus string     `json:"documentStatus"`
	UpdatedAt      time.Time  `json:"updatedAt"`
	TaskID         string     `json:"taskId"`
	PositionCode   string     `json:"positionCode"`
	PositionName   string     `json:"positionName"`
	SignerName     string     `json:"signerName"`
	TaskStatus     string     `json:"taskStatus"`
	SignedAt       *time.Time `json:"signedAt,omitempty"`
	RejectedAt     *time.Time `json:"rejectedAt,omitempty"`
	RejectReason   string     `json:"rejectReason,omitempty"`
	HasFinalPDF    bool       `json:"hasFinalPdf"`
	HasCurrentPDF  bool       `json:"hasCurrentPdf"`
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

type SigningDocumentPrintEvent struct {
	ID              string       `json:"id"`
	DocumentID      string       `json:"documentId"`
	FileID          string       `json:"fileId"`
	Channel         string       `json:"channel"`
	PrinterName     string       `json:"printerName"`
	DeviceIDHash    string       `json:"deviceIdHash"`
	ClientTimezone  string       `json:"clientTimezone"`
	FinalFileSHA256 string       `json:"finalFileSha256"`
	PrintedBy       string       `json:"printedBy"`
	IPAddress       string       `json:"ipAddress"`
	UserAgent       string       `json:"userAgent"`
	PrintedAt       time.Time    `json:"printedAt"`
	File            UploadedFile `json:"file"`
}

type CreateSigningDocumentRequest struct {
	DocFormatCode string `json:"docFormatCode"`
	DocNo         string `json:"docNo"`
}

type CreatePrintCopyRequest struct {
	Channel        string `json:"channel"`
	DeviceID       string `json:"deviceId"`
	PrinterName    string `json:"printerName"`
	ClientTimezone string `json:"clientTimezone"`
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
	Username     string `json:"username"`
	Password     string `json:"password"`
	DatabaseName string `json:"databaseName"`
	AuthSource   string `json:"authSource"`
}

type LoginResponse struct {
	Token            string              `json:"token,omitempty"`
	TokenType        string              `json:"tokenType,omitempty"`
	ExpiresAt        *time.Time          `json:"expiresAt,omitempty"`
	User             *User               `json:"user,omitempty"`
	Session          *AuthSession        `json:"session,omitempty"`
	DatabaseRequired bool                `json:"databaseRequired,omitempty"`
	Databases        []SMLAuthDatabase   `json:"databases,omitempty"`
	AuthSource       string              `json:"authSource,omitempty"`
	TenantReadiness  *SMLTenantReadiness `json:"tenantReadiness,omitempty"`
}

type AuthSession struct {
	SMLProvider  string `json:"smlProvider"`
	SMLDataGroup string `json:"smlDataGroup"`
	SMLDataCode  string `json:"smlDataCode"`
	SMLTenant    string `json:"smlTenant"`
	AuthSource   string `json:"authSource"`
}

type SMLAuthDatabase struct {
	DataGroup    string              `json:"dataGroup"`
	DataCode     string              `json:"dataCode"`
	DataName     string              `json:"dataName"`
	DatabaseName string              `json:"databaseName"`
	Tenant       string              `json:"tenant"`
	Readiness    *SMLTenantReadiness `json:"readiness,omitempty"`
}

type SMLTenantReadiness struct {
	OK            bool                  `json:"ok"`
	Status        string                `json:"status"`
	Message       string                `json:"message"`
	Tenant        string                `json:"tenant"`
	ImageDatabase string                `json:"imageDatabase"`
	Template      string                `json:"template,omitempty"`
	Checks        []SMLTenantReadyCheck `json:"checks,omitempty"`
}

type SMLTenantReadyCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type APIError struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}
