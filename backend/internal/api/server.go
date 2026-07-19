package api

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/bosocmputer/paperless-v2/backend/internal/config"
	"github.com/bosocmputer/paperless-v2/backend/internal/store"
)

type Server struct {
	cfg        config.Config
	store      *store.Store
	logger     *slog.Logger
	httpClient *http.Client
}

func NewServer(cfg config.Config, store *store.Store, logger *slog.Logger) *Server {
	return &Server{
		cfg:        cfg,
		store:      store,
		logger:     logger,
		httpClient: &http.Client{Timeout: cfg.SMLPaperlessTimeout},
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", s.live)
	mux.HandleFunc("GET /health/ready", s.ready)
	mux.HandleFunc("POST /api/auth/login", s.login)
	mux.HandleFunc("POST /api/auth/sml/verify-database", s.verifySMLTenantReadinessForLogin)
	mux.HandleFunc("POST /api/auth/sml/provision-image-db", s.provisionSMLTenantImageDatabaseForLogin)
	mux.Handle("GET /api/auth/me", s.requireAuth(http.HandlerFunc(s.me)))
	mux.Handle("POST /api/auth/logout", s.requireAuth(http.HandlerFunc(s.logout)))
	mux.Handle("GET /api/users", s.requireSuperAdmin(http.HandlerFunc(s.listUsers)))
	mux.Handle("POST /api/users/sync-sml", s.requireSuperAdmin(http.HandlerFunc(s.syncSMLUsers)))
	mux.Handle("POST /api/users", s.requireSuperAdmin(http.HandlerFunc(s.createUser)))
	mux.Handle("GET /api/users/{id}/saved-signature", s.requireSuperAdmin(http.HandlerFunc(s.getUserSavedSignature)))
	mux.Handle("PUT /api/users/{id}", s.requireSuperAdmin(http.HandlerFunc(s.updateUser)))
	mux.Handle("DELETE /api/users/{id}", s.requireSuperAdmin(http.HandlerFunc(s.deactivateUser)))
	mux.Handle("GET /api/sml/screen-codes", s.requireAdmin(http.HandlerFunc(s.listSMLScreenCodes)))
	mux.Handle("GET /api/sml/doc-formats", s.requireAdmin(http.HandlerFunc(s.listSMLDocFormats)))
	mux.Handle("GET /api/sml/doc-format", s.requireAdmin(http.HandlerFunc(s.getSMLDocFormatByCode)))
	mux.Handle("GET /api/sml/document-candidates", s.requireAdmin(http.HandlerFunc(s.listSMLDocumentCandidates)))
	mux.Handle("GET /api/sml/document-candidates/{docNo}", s.requireAdmin(http.HandlerFunc(s.getSMLDocumentCandidate)))
	mux.Handle("GET /api/document-configs", s.requireAdmin(http.HandlerFunc(s.listDocumentConfigSteps)))
	mux.Handle("POST /api/document-configs", s.requireSuperAdmin(http.HandlerFunc(s.createDocumentConfigStep)))
	mux.Handle("PUT /api/document-configs/{id}", s.requireSuperAdmin(http.HandlerFunc(s.updateDocumentConfigStep)))
	mux.Handle("DELETE /api/document-configs/{id}", s.requireSuperAdmin(http.HandlerFunc(s.deleteDocumentConfigStep)))
	mux.Handle("GET /api/document-config-workflows", s.requireSuperAdmin(http.HandlerFunc(s.listDocumentConfigWorkflows)))
	mux.Handle("GET /api/document-config-workflows/{docFormatCode}", s.requireSuperAdmin(http.HandlerFunc(s.getDocumentConfigWorkflow)))
	mux.Handle("PUT /api/document-config-workflows/{docFormatCode}", s.requireSuperAdmin(http.HandlerFunc(s.saveDocumentConfigWorkflow)))
	mux.Handle("POST /api/document-config-workflows/{docFormatCode}/copy", s.requireSuperAdmin(http.HandlerFunc(s.copyDocumentConfigWorkflow)))
	mux.Handle("POST /api/document-config-workflows/{docFormatCode}/events", s.requireSuperAdmin(http.HandlerFunc(s.recordDocumentConfigWorkflowEvent)))
	mux.Handle("GET /api/signature-templates", s.requireAdmin(http.HandlerFunc(s.getSignatureTemplateState)))
	mux.Handle("POST /api/signature-templates/sample-pdf", s.requireSuperAdmin(http.HandlerFunc(s.uploadSignatureTemplateSamplePDF)))
	mux.Handle("GET /api/signature-templates/{id}/sample-pdf", s.requireSuperAdmin(http.HandlerFunc(s.getSignatureTemplateSamplePDF)))
	mux.Handle("PUT /api/signature-templates/{id}/boxes", s.requireSuperAdmin(http.HandlerFunc(s.saveSignatureTemplateBoxes)))
	mux.Handle("POST /api/signature-templates/{id}/publish", s.requireSuperAdmin(http.HandlerFunc(s.publishSignatureTemplate)))
	mux.Handle("POST /api/signature-templates/{id}/designer-events", s.requireSuperAdmin(http.HandlerFunc(s.recordSignatureTemplateDesignerEvent)))
	mux.Handle("GET /api/admin/dashboard", s.requireAdmin(http.HandlerFunc(s.getAdminDashboard)))
	mux.Handle("GET /api/admin/document-flow", s.requireAdmin(http.HandlerFunc(s.getAdminDocumentFlow)))
	mux.Handle("POST /api/admin/document-flow/events", s.requireAdmin(http.HandlerFunc(s.recordDocumentFlowEvent)))
	mux.Handle("GET /api/signing-documents", s.requireAdmin(http.HandlerFunc(s.listSigningDocuments)))
	mux.Handle("GET /api/signing-documents/duplicate-check", s.requireAdmin(http.HandlerFunc(s.checkSigningDocumentDuplicate)))
	mux.Handle("POST /api/signing-documents/uploads", s.requireAdmin(http.HandlerFunc(s.uploadSigningDocumentPDF)))
	mux.Handle("GET /api/signing-documents/uploads/{fileId}/pdf", s.requireAdmin(http.HandlerFunc(s.getSigningDocumentUploadPDF)))
	mux.Handle("DELETE /api/signing-documents/uploads/{fileId}", s.requireAdmin(http.HandlerFunc(s.deleteSigningDocumentUpload)))
	mux.Handle("POST /api/signing-documents/batch-validation", s.requireAdmin(http.HandlerFunc(s.validateSigningDocumentBatch)))
	mux.Handle("POST /api/signing-documents/batch-items", s.requireAdmin(http.HandlerFunc(s.createSigningDocumentBatchItem)))
	mux.Handle("POST /api/signing-documents/batch-events", s.requireAdmin(http.HandlerFunc(s.recordSigningDocumentBatchEvent)))
	mux.Handle("POST /api/signing-documents/create-events", s.requireAdmin(http.HandlerFunc(s.recordSigningDocumentCreateEvent)))
	mux.Handle("POST /api/signing-documents", s.requireAdmin(http.HandlerFunc(s.createSigningDocument)))
	mux.Handle("GET /api/signing-documents/{id}", s.requireAdmin(http.HandlerFunc(s.getSigningDocument)))
	mux.Handle("GET /api/signing-documents/{id}/related-documents", s.requireAdmin(http.HandlerFunc(s.getSigningDocumentRelatedDocuments)))
	mux.Handle("GET /api/signing-documents/{id}/reference-check", s.requireAdmin(http.HandlerFunc(s.getSigningDocumentReferenceCheck)))
	mux.Handle("GET /api/signing-documents/{id}/attachments", s.requireAdmin(http.HandlerFunc(s.listSigningDocumentAttachments)))
	mux.Handle("GET /api/signing-documents/{id}/attachments/{attachmentId}/file", s.requireAdmin(http.HandlerFunc(s.getSigningDocumentAttachmentFile)))
	mux.Handle("GET /api/signing-documents/{id}/pdf", s.requireAuth(http.HandlerFunc(s.getSigningDocumentPDF)))
	mux.Handle("POST /api/signing-documents/{id}/send", s.requireAdmin(http.HandlerFunc(s.sendSigningDocument)))
	mux.Handle("POST /api/signing-documents/{id}/confirm", s.requireAdmin(http.HandlerFunc(s.confirmSigningDocument)))
	mux.Handle("POST /api/signing-documents/{id}/cancel", s.requireAdmin(http.HandlerFunc(s.cancelSigningDocument)))
	mux.Handle("POST /api/signing-documents/{id}/retry-final-pdf", s.requireAdmin(http.HandlerFunc(s.retrySigningDocumentFinalPDF)))
	mux.Handle("POST /api/signing-documents/{id}/retry-sml-images", s.requireAdmin(http.HandlerFunc(s.retrySigningDocumentImages)))
	mux.Handle("POST /api/signing-documents/{id}/retry-sml-lock", s.requireAdmin(http.HandlerFunc(s.retrySigningDocumentLock)))
	mux.Handle("POST /api/signing-documents/{id}/print-copies", s.requireAdmin(http.HandlerFunc(s.createSigningDocumentPrintCopy)))
	mux.Handle("GET /api/signing-documents/{id}/print-copies/{printCopyId}/pdf", s.requireAdmin(http.HandlerFunc(s.getSigningDocumentPrintCopyPDF)))
	mux.Handle("POST /api/signing-documents/external-token/{signerId}/regenerate", s.requireAdmin(http.HandlerFunc(s.regenerateExternalToken)))
	mux.Handle("GET /api/my/signing-tasks", s.requireAuth(http.HandlerFunc(s.listMySigningTasks)))
	mux.Handle("GET /api/my/signing-history", s.requireAuth(http.HandlerFunc(s.listMySigningHistory)))
	mux.Handle("GET /api/my/signing-history/{taskId}/pdf", s.requireAuth(http.HandlerFunc(s.getMySigningHistoryPDF)))
	mux.Handle("GET /api/my/signing-history/{taskId}", s.requireAuth(http.HandlerFunc(s.getMySigningHistory)))
	mux.Handle("GET /api/my/signing-tasks/{taskId}", s.requireAuth(http.HandlerFunc(s.getMySigningTask)))
	mux.Handle("GET /api/my/signing-tasks/{taskId}/saved-signature", s.requireAuth(http.HandlerFunc(s.getMySigningTaskSavedSignature)))
	mux.Handle("GET /api/my/signing-tasks/{taskId}/related-documents", s.requireAuth(http.HandlerFunc(s.getMySigningTaskRelatedDocuments)))
	mux.Handle("GET /api/my/signing-tasks/{taskId}/reference-check", s.requireAuth(http.HandlerFunc(s.getMySigningTaskReferenceCheck)))
	mux.Handle("GET /api/my/signing-tasks/{taskId}/reference-status", s.requireAuth(http.HandlerFunc(s.getMySigningTaskReferenceStatus)))
	mux.Handle("POST /api/my/signing-tasks/{taskId}/events", s.requireAuth(http.HandlerFunc(s.recordMySigningTaskEvent)))
	mux.Handle("GET /api/my/signing-tasks/{taskId}/attachments", s.requireAuth(http.HandlerFunc(s.listMySigningTaskAttachments)))
	mux.Handle("GET /api/my/signing-tasks/{taskId}/attachments/{attachmentId}/file", s.requireAuth(http.HandlerFunc(s.getMySigningTaskAttachmentFile)))
	mux.Handle("POST /api/my/signing-tasks/{taskId}/attachments", s.requireAuth(http.HandlerFunc(s.uploadMySigningTaskAttachment)))
	mux.Handle("POST /api/my/signing-tasks/{taskId}/sign", s.requireAuth(http.HandlerFunc(s.signMySigningTask)))
	mux.Handle("POST /api/my/signing-tasks/{taskId}/reject", s.requireAuth(http.HandlerFunc(s.rejectMySigningTask)))
	mux.HandleFunc("POST /api/public/signing/{token}/verify-otp", s.verifyExternalOTP)
	mux.HandleFunc("GET /api/public/signing/{token}", s.getPublicSigningDocument)
	mux.HandleFunc("GET /api/public/signing/{token}/related-documents", s.getPublicSigningRelatedDocuments)
	mux.HandleFunc("GET /api/public/signing/{token}/pdf", s.getPublicSigningPDF)
	mux.HandleFunc("POST /api/public/signing/{token}/events", s.recordPublicSigningTaskEvent)
	mux.HandleFunc("GET /api/public/signing/{token}/attachments", s.listPublicSigningTaskAttachments)
	mux.HandleFunc("GET /api/public/signing/{token}/attachments/{attachmentId}/file", s.getPublicSigningTaskAttachmentFile)
	mux.HandleFunc("POST /api/public/signing/{token}/attachments", s.uploadPublicSigningTaskAttachment)
	mux.HandleFunc("POST /api/public/signing/{token}/sign", s.signPublicSigningTask)
	mux.HandleFunc("POST /api/public/signing/{token}/reject", s.rejectPublicSigningTask)

	return s.recover(s.cors(mux))
}

func (s *Server) cors(next http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, origin := range s.cfg.CORSOrigins {
		allowed[origin] = true
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && (allowed[origin] || allowed["*"]) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		return strings.TrimSpace(strings.Split(forwarded, ",")[0])
	}
	return r.RemoteAddr
}
