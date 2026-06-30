package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/bosocmputer/paperless-v2/backend/internal/models"
	"github.com/bosocmputer/paperless-v2/backend/internal/store"
)

type smlRelatedDocumentsResponse struct {
	Success bool                            `json:"success"`
	Data    models.SMLRelatedDocumentsGraph `json:"data"`
	Error   *smlAPIError                    `json:"error"`
	Message string                          `json:"message"`
}

var documentFlowEventNames = map[string]bool{
	"document_flow_open":         true,
	"document_flow_search":       true,
	"document_flow_load_success": true,
	"document_flow_load_error":   true,
	"document_flow_node_click":   true,
	"document_flow_pdf_open":     true,
}

type documentFlowEventRequest struct {
	Event         string `json:"event"`
	SessionID     string `json:"sessionId"`
	DocFormatCode string `json:"docFormatCode"`
	ElapsedMS     int64  `json:"elapsedMs"`
	NodeCount     int    `json:"nodeCount"`
	ErrorCode     string `json:"errorCode"`
}

func (s *Server) getAdminDocumentFlow(w http.ResponseWriter, r *http.Request) {
	docNo := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("doc_no")))
	if docNo == "" {
		writeError(w, http.StatusBadRequest, "doc_no_required", "Please enter a document number.")
		return
	}
	docFormatCode := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("doc_format_code")))
	graph, ok := s.writeRelatedDocuments(w, r, docFormatCode, docNo, true)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"documentFlow": graph})
}

func (s *Server) recordDocumentFlowEvent(w http.ResponseWriter, r *http.Request) {
	actor, _ := currentUser(r)
	req, err := decodeDocumentFlowEventPayload(r.Body, maxSigningEventBytes)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_document_flow_event", "Document flow event payload is invalid.")
		return
	}
	metadata, err := normalizeDocumentFlowEventMetadata(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_document_flow_event", err.Error())
		return
	}
	if err := s.store.WriteAuditWithMetadata(r.Context(), actor.ID, "document_flow.ux_event", "document_flow", "", clientIP(r), r.UserAgent(), metadata); err != nil {
		s.logger.Warn("write document flow event failed", "error", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getSigningDocumentRelatedDocuments(w http.ResponseWriter, r *http.Request) {
	document, err := s.store.FindSigningDocumentByID(r.Context(), strings.TrimSpace(r.PathValue("id")))
	if errors.Is(err, store.ErrSigningDocumentNotFound) {
		writeError(w, http.StatusNotFound, "signing_document_not_found", "Signing document was not found.")
		return
	}
	if err != nil {
		s.logger.Error("load signing document for related documents failed", "error", err)
		writeError(w, http.StatusInternalServerError, "signing_document_failed", "Cannot load signing document right now.")
		return
	}
	graph, ok := s.writeRelatedDocuments(w, r, document.DocFormatCode, document.DocNo, true)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"relatedDocuments": graph})
}

func (s *Server) getMySigningTaskRelatedDocuments(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r)
	signer, err := s.store.FindSigningTaskByID(r.Context(), strings.TrimSpace(r.PathValue("taskId")))
	if errors.Is(err, store.ErrSigningTaskNotFound) || (err == nil && !strings.EqualFold(signer.SignerUser, user.Username)) {
		writeError(w, http.StatusNotFound, "signing_task_not_found", "Signing task was not found.")
		return
	}
	if err != nil {
		s.logger.Error("load signing task for related documents failed", "error", err)
		writeError(w, http.StatusInternalServerError, "signing_task_failed", "Cannot load signing task right now.")
		return
	}
	document, err := s.store.FindSigningDocumentByID(r.Context(), signer.DocumentID)
	if err != nil {
		s.logger.Error("load signer document for related documents failed", "error", err)
		writeError(w, http.StatusInternalServerError, "signing_document_failed", "Cannot load signing document right now.")
		return
	}
	graph, ok := s.writeRelatedDocuments(w, r, document.DocFormatCode, document.DocNo, false)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"relatedDocuments": graph})
}

func (s *Server) getPublicSigningRelatedDocuments(w http.ResponseWriter, r *http.Request) {
	signer, ok := s.externalSignerFromRequest(w, r)
	if !ok {
		return
	}
	document, err := s.store.FindSigningDocumentByID(r.Context(), signer.DocumentID)
	if err != nil {
		s.logger.Error("load public document for related documents failed", "error", err)
		writeError(w, http.StatusInternalServerError, "signing_document_failed", "Cannot load signing document right now.")
		return
	}
	graph, ok := s.writeRelatedDocuments(w, r, document.DocFormatCode, document.DocNo, false)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"relatedDocuments": graph})
}

func (s *Server) writeRelatedDocuments(w http.ResponseWriter, r *http.Request, docFormatCode, docNo string, admin bool) (models.SMLRelatedDocumentsGraph, bool) {
	depth := strings.TrimSpace(r.URL.Query().Get("depth"))
	if depth == "" {
		depth = "3"
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.SMLPaperlessTimeout)
	defer cancel()

	graph, err := s.fetchSMLRelatedDocuments(ctx, docFormatCode, docNo, depth)
	if errors.Is(err, errSMLConfigMissing) {
		writeError(w, http.StatusServiceUnavailable, "sml_not_configured", "SML Paperless API is not configured.")
		return models.SMLRelatedDocumentsGraph{}, false
	}
	if err != nil {
		s.logger.Warn("fetch sml related documents failed", "error", err, "docFormatCode", docFormatCode, "docNo", docNo)
		writeError(w, http.StatusBadGateway, "sml_related_documents_failed", fmt.Sprintf("Cannot load related documents from SML: %s", err.Error()))
		return models.SMLRelatedDocumentsGraph{}, false
	}
	graph, err = s.enrichRelatedDocuments(r.Context(), graph, admin)
	if err != nil {
		s.logger.Error("enrich related documents failed", "error", err)
		writeError(w, http.StatusInternalServerError, "related_documents_enrich_failed", "Cannot prepare related documents right now.")
		return models.SMLRelatedDocumentsGraph{}, false
	}
	if !admin {
		graph = sanitizeRelatedDocumentsForSigner(graph)
	}
	return graph, true
}

func (s *Server) fetchSMLRelatedDocuments(ctx context.Context, docFormatCode, docNo, depth string) (models.SMLRelatedDocumentsGraph, error) {
	if strings.TrimSpace(s.cfg.SMLPaperlessBaseURL) == "" ||
		strings.TrimSpace(s.cfg.SMLPaperlessAPIKey) == "" ||
		strings.TrimSpace(s.cfg.SMLPaperlessTenant) == "" {
		return models.SMLRelatedDocumentsGraph{}, errSMLConfigMissing
	}
	endpoint, err := url.Parse(s.cfg.SMLPaperlessBaseURL + "/api/v1/documents/" + url.PathEscape(docNo) + "/related")
	if err != nil {
		return models.SMLRelatedDocumentsGraph{}, fmt.Errorf("invalid SML base URL")
	}
	query := endpoint.Query()
	if strings.TrimSpace(docFormatCode) != "" {
		query.Set("doc_format_code", docFormatCode)
	}
	query.Set("depth", depth)
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return models.SMLRelatedDocumentsGraph{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Api-Key", s.cfg.SMLPaperlessAPIKey)
	req.Header.Set("X-Tenant", s.cfg.SMLPaperlessTenant)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return models.SMLRelatedDocumentsGraph{}, err
	}
	defer resp.Body.Close()

	var payload smlRelatedDocumentsResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&payload); err != nil {
		return models.SMLRelatedDocumentsGraph{}, fmt.Errorf("cannot parse SML response")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return models.SMLRelatedDocumentsGraph{}, errors.New(smlErrorMessage(payload.Error, payload.Message, resp.Status))
	}
	if !payload.Success {
		return models.SMLRelatedDocumentsGraph{}, errors.New(smlErrorMessage(payload.Error, payload.Message, "SML request failed"))
	}
	return payload.Data, nil
}

func (s *Server) enrichRelatedDocuments(ctx context.Context, graph models.SMLRelatedDocumentsGraph, canOpen bool) (models.SMLRelatedDocumentsGraph, error) {
	docNos := make([]string, 0, len(graph.Nodes))
	for _, node := range graph.Nodes {
		docNos = append(docNos, node.DocNo)
	}
	refs, err := s.store.ListSigningDocumentReferencesByDocNos(ctx, docNos)
	if err != nil {
		return graph, err
	}
	byKey := map[string][]models.SigningDocumentReference{}
	byDocNo := map[string][]models.SigningDocumentReference{}
	for _, ref := range refs {
		ref.CanOpenPaperless = canOpen
		ref.CanViewCurrentPDF = canOpen && ref.HasCurrentPDF
		ref.CanViewSignedPDF = canOpen && ref.HasFinalPDF
		if canOpen {
			ref.CurrentPDFURL = fmt.Sprintf("/api/signing-documents/%s/pdf?version=current", ref.ID)
			if ref.HasFinalPDF {
				ref.SignedPDFURL = fmt.Sprintf("/api/signing-documents/%s/pdf?version=final", ref.ID)
			}
		}
		key := relatedReferenceKey(ref.DocFormatCode, ref.DocNo)
		docNoKey := strings.ToUpper(strings.TrimSpace(ref.DocNo))
		byKey[key] = append(byKey[key], ref)
		byDocNo[docNoKey] = append(byDocNo[docNoKey], ref)
	}
	for i := range graph.Nodes {
		matches := byKey[relatedReferenceKey(graph.Nodes[i].DocFormatCode, graph.Nodes[i].DocNo)]
		if len(matches) == 0 {
			matches = byDocNo[strings.ToUpper(strings.TrimSpace(graph.Nodes[i].DocNo))]
		}
		if len(matches) == 0 {
			continue
		}
		ref := matches[0]
		graph.Nodes[i].PaperlessStatus = ref.Status
		graph.Nodes[i].CanOpenPaperless = canOpen
		graph.Nodes[i].HasCurrentPDF = ref.HasCurrentPDF
		graph.Nodes[i].HasFinalPDF = ref.HasFinalPDF
		graph.Nodes[i].CanViewCurrentPDF = ref.CanViewCurrentPDF
		graph.Nodes[i].CanViewSignedPDF = ref.CanViewSignedPDF
		graph.Nodes[i].CurrentPDFURL = ref.CurrentPDFURL
		graph.Nodes[i].SignedPDFURL = ref.SignedPDFURL
		graph.Nodes[i].MatchCount = len(matches)
		graph.Nodes[i].PaperlessMatches = matches
		if canOpen {
			graph.Nodes[i].PaperlessDocumentID = ref.ID
		}
		if len(matches) > 1 {
			graph.Warnings = append(graph.Warnings, models.SMLRelatedDocumentWarning{
				Code:    "paperless_multiple_matches",
				DocNo:   graph.Nodes[i].DocNo,
				Message: "พบเอกสารนี้ใน PaperLess มากกว่า 1 รายการ",
			})
		}
		if strings.EqualFold(graph.Root.DocNo, graph.Nodes[i].DocNo) && strings.EqualFold(graph.Root.DocFormatCode, graph.Nodes[i].DocFormatCode) {
			graph.Root.PaperlessStatus = graph.Nodes[i].PaperlessStatus
			graph.Root.CanOpenPaperless = graph.Nodes[i].CanOpenPaperless
			graph.Root.PaperlessDocumentID = graph.Nodes[i].PaperlessDocumentID
			graph.Root.HasCurrentPDF = graph.Nodes[i].HasCurrentPDF
			graph.Root.HasFinalPDF = graph.Nodes[i].HasFinalPDF
			graph.Root.CanViewCurrentPDF = graph.Nodes[i].CanViewCurrentPDF
			graph.Root.CanViewSignedPDF = graph.Nodes[i].CanViewSignedPDF
			graph.Root.CurrentPDFURL = graph.Nodes[i].CurrentPDFURL
			graph.Root.SignedPDFURL = graph.Nodes[i].SignedPDFURL
			graph.Root.MatchCount = graph.Nodes[i].MatchCount
			graph.Root.PaperlessMatches = graph.Nodes[i].PaperlessMatches
		}
	}
	return graph, nil
}

func sanitizeRelatedDocumentsForSigner(graph models.SMLRelatedDocumentsGraph) models.SMLRelatedDocumentsGraph {
	graph.Root.PaperlessDocumentID = ""
	graph.Root.CanOpenPaperless = false
	graph.Root.HasCurrentPDF = false
	graph.Root.HasFinalPDF = false
	graph.Root.CanViewCurrentPDF = false
	graph.Root.CanViewSignedPDF = false
	graph.Root.CurrentPDFURL = ""
	graph.Root.SignedPDFURL = ""
	graph.Root.MatchCount = 0
	graph.Root.PaperlessMatches = nil
	for i := range graph.Nodes {
		graph.Nodes[i].PaperlessDocumentID = ""
		graph.Nodes[i].CanOpenPaperless = false
		graph.Nodes[i].HasCurrentPDF = false
		graph.Nodes[i].HasFinalPDF = false
		graph.Nodes[i].CanViewCurrentPDF = false
		graph.Nodes[i].CanViewSignedPDF = false
		graph.Nodes[i].CurrentPDFURL = ""
		graph.Nodes[i].SignedPDFURL = ""
		graph.Nodes[i].MatchCount = 0
		graph.Nodes[i].PaperlessMatches = nil
	}
	return graph
}

func relatedReferenceKey(docFormatCode, docNo string) string {
	return strings.ToLower(strings.TrimSpace(docFormatCode)) + "\x00" + strings.TrimSpace(docNo)
}

func decodeDocumentFlowEventPayload(body io.Reader, maxBytes int64) (documentFlowEventRequest, error) {
	var req documentFlowEventRequest
	decoder := json.NewDecoder(io.LimitReader(body, maxBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		return req, err
	}
	return req, nil
}

func normalizeDocumentFlowEventMetadata(req documentFlowEventRequest) (map[string]any, error) {
	event := strings.TrimSpace(req.Event)
	if !documentFlowEventNames[event] {
		return nil, fmt.Errorf("event is not allowed")
	}
	metadata := map[string]any{
		"event":         event,
		"sessionId":     truncateForMetadata(req.SessionID, 80),
		"docFormatCode": truncateForMetadata(strings.ToUpper(req.DocFormatCode), 20),
		"elapsedMs":     clampInt64(req.ElapsedMS, 0, 24*60*60*1000),
		"nodeCount":     clampInt(req.NodeCount, 0, 30),
		"errorCode":     truncateForMetadata(req.ErrorCode, 80),
	}
	return metadata, nil
}
