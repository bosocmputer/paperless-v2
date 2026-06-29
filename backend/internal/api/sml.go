package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/bosocmputer/paperless-v2/backend/internal/models"
)

var (
	errDocFormatNotFound          = errors.New("doc format code was not found in SML")
	errDocFormatAmbiguous         = errors.New("doc format code matches more than one screen code in SML")
	errDocFormatInvalidScreenCode = errors.New("doc format code has no valid screen code in SML")
	errSMLConfigMissing           = errors.New("sml paperless api config is incomplete")
)

type smlDocFormatsResponse struct {
	Success bool                  `json:"success"`
	Data    []models.SMLDocFormat `json:"data"`
	Error   *smlAPIError          `json:"error"`
	Message string                `json:"message"`
}

type smlDocFormatResponse struct {
	Success bool                `json:"success"`
	Data    models.SMLDocFormat `json:"data"`
	Error   *smlAPIError        `json:"error"`
	Message string              `json:"message"`
}

type smlDocumentCandidatesResponse struct {
	Success bool                          `json:"success"`
	Data    []models.SMLDocumentCandidate `json:"data"`
	Page    int                           `json:"page"`
	Size    int                           `json:"size"`
	Total   int                           `json:"total"`
	HasMore bool                          `json:"hasMore"`
	Error   *smlAPIError                  `json:"error"`
	Message string                        `json:"message"`
}

type smlDocumentCandidateResponse struct {
	Success bool                        `json:"success"`
	Data    models.SMLDocumentCandidate `json:"data"`
	Error   *smlAPIError                `json:"error"`
	Message string                      `json:"message"`
}

type smlLockResponse struct {
	Success bool `json:"success"`
	Data    struct {
		DocNo         string `json:"doc_no"`
		Table         string `json:"table"`
		TransFlag     int    `json:"trans_flag"`
		IsLockRecord  int    `json:"is_lock_record"`
		AlreadyLocked bool   `json:"already_locked"`
	} `json:"data"`
	Error   *smlAPIError `json:"error"`
	Message string       `json:"message"`
}

type smlAPIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (s *Server) listSMLScreenCodes(w http.ResponseWriter, r *http.Request) {
	formats, err := s.fetchSMLDocFormats(r.Context(), "")
	if errors.Is(err, errSMLConfigMissing) {
		writeError(w, http.StatusServiceUnavailable, "sml_not_configured", "SML Paperless API is not configured.")
		return
	}
	if err != nil {
		s.logger.Warn("fetch sml screen codes failed", "error", err)
		writeError(w, http.StatusBadGateway, "sml_screen_codes_failed", fmt.Sprintf("Cannot load screen codes from SML: %s", err.Error()))
		return
	}

	counts := map[string]int{}
	for _, format := range formats {
		screenCode := normalizeScreenCode(format.ScreenCode)
		if screenCode == "" {
			continue
		}
		counts[screenCode]++
	}

	screenCodes := make([]models.SMLScreenCode, 0, len(counts))
	for code, count := range counts {
		screenCodes = append(screenCodes, models.SMLScreenCode{Code: code, Count: count})
	}
	sort.Slice(screenCodes, func(i, j int) bool {
		return screenCodes[i].Code < screenCodes[j].Code
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"tenant":      s.cfg.SMLPaperlessTenant,
		"screenCodes": screenCodes,
		"source":      "sml-api-bybos-paperless",
		"sourceTable": "erp_doc_format",
	})
}

func (s *Server) listSMLDocFormats(w http.ResponseWriter, r *http.Request) {
	screenCode := normalizeScreenCode(r.URL.Query().Get("screen_code"))
	if screenCode != "" && !isValidScreenCode(screenCode) {
		writeError(w, http.StatusBadRequest, "invalid_screen_code", "screen_code is invalid.")
		return
	}

	formats, err := s.fetchSMLDocFormats(r.Context(), screenCode)
	if errors.Is(err, errSMLConfigMissing) {
		writeError(w, http.StatusServiceUnavailable, "sml_not_configured", "SML Paperless API is not configured.")
		return
	}
	if err != nil {
		s.logger.Warn("fetch sml doc formats failed", "error", err, "screenCode", screenCode)
		writeError(w, http.StatusBadGateway, "sml_doc_formats_failed", fmt.Sprintf("Cannot load document formats from SML: %s", err.Error()))
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"screenCode":  screenCode,
		"tenant":      s.cfg.SMLPaperlessTenant,
		"docFormats":  formats,
		"source":      "sml-api-bybos-paperless",
		"sourceTable": "erp_doc_format",
	})
}

func (s *Server) getSMLDocFormatByCode(w http.ResponseWriter, r *http.Request) {
	docFormatCode := strings.TrimSpace(r.URL.Query().Get("doc_format_code"))
	if docFormatCode == "" {
		docFormatCode = strings.TrimSpace(r.URL.Query().Get("code"))
	}
	if docFormatCode == "" {
		writeError(w, http.StatusBadRequest, "doc_format_code_required", "doc_format_code is required.")
		return
	}

	format, err := s.fetchSMLDocFormatByCode(r.Context(), docFormatCode)
	if errors.Is(err, errSMLConfigMissing) {
		writeError(w, http.StatusServiceUnavailable, "sml_not_configured", "SML Paperless API is not configured.")
		return
	}
	if err != nil {
		s.writeDocFormatValidationError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"tenant":      s.cfg.SMLPaperlessTenant,
		"docFormat":   format,
		"source":      "sml-api-bybos-paperless",
		"sourceTable": "erp_doc_format",
	})
}

func (s *Server) listSMLDocumentCandidates(w http.ResponseWriter, r *http.Request) {
	docFormatCode := strings.TrimSpace(r.URL.Query().Get("doc_format_code"))
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	page := strings.TrimSpace(r.URL.Query().Get("page"))
	size := strings.TrimSpace(r.URL.Query().Get("size"))
	if docFormatCode == "" {
		writeError(w, http.StatusBadRequest, "doc_format_code_required", "doc_format_code is required.")
		return
	}
	payload, err := s.fetchSMLDocumentCandidates(r.Context(), docFormatCode, search, page, size)
	if errors.Is(err, errSMLConfigMissing) {
		writeError(w, http.StatusServiceUnavailable, "sml_not_configured", "SML Paperless API is not configured.")
		return
	}
	if err != nil {
		s.logger.Warn("fetch sml document candidates failed", "error", err, "docFormatCode", docFormatCode)
		writeError(w, http.StatusBadGateway, "sml_document_candidates_failed", fmt.Sprintf("Cannot search SML documents: %s", err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"documents": payload.Data,
		"page":      payload.Page,
		"size":      payload.Size,
		"total":     payload.Total,
		"hasMore":   payload.HasMore,
	})
}

func (s *Server) fetchSMLDocFormats(ctx context.Context, screenCode string) ([]models.SMLDocFormat, error) {
	if strings.TrimSpace(s.cfg.SMLPaperlessBaseURL) == "" ||
		strings.TrimSpace(s.cfg.SMLPaperlessAPIKey) == "" ||
		strings.TrimSpace(s.cfg.SMLPaperlessTenant) == "" {
		return nil, errSMLConfigMissing
	}

	endpoint, err := url.Parse(s.cfg.SMLPaperlessBaseURL + "/api/v1/ic/doc-formats")
	if err != nil {
		return nil, fmt.Errorf("invalid SML base URL")
	}
	query := endpoint.Query()
	if screenCode != "" {
		query.Set("screen_code", screenCode)
	}
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Api-Key", s.cfg.SMLPaperlessAPIKey)
	req.Header.Set("X-Tenant", s.cfg.SMLPaperlessTenant)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var payload smlDocFormatsResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("cannot parse SML response")
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, errors.New(smlErrorMessage(payload.Error, payload.Message, resp.Status))
	}
	if !payload.Success {
		return nil, errors.New(smlErrorMessage(payload.Error, payload.Message, "SML request failed"))
	}

	for i := range payload.Data {
		if payload.Data[i].ScreenCode == "" {
			payload.Data[i].ScreenCode = screenCode
		}
	}
	return payload.Data, nil
}

func (s *Server) fetchSMLDocFormatByCode(ctx context.Context, docFormatCode string) (models.SMLDocFormat, error) {
	if strings.TrimSpace(s.cfg.SMLPaperlessBaseURL) == "" ||
		strings.TrimSpace(s.cfg.SMLPaperlessAPIKey) == "" ||
		strings.TrimSpace(s.cfg.SMLPaperlessTenant) == "" {
		return models.SMLDocFormat{}, errSMLConfigMissing
	}

	endpoint, err := url.Parse(s.cfg.SMLPaperlessBaseURL + "/api/v1/ic/doc-formats/by-code")
	if err != nil {
		return models.SMLDocFormat{}, fmt.Errorf("invalid SML base URL")
	}
	query := endpoint.Query()
	query.Set("doc_format_code", docFormatCode)
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return models.SMLDocFormat{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Api-Key", s.cfg.SMLPaperlessAPIKey)
	req.Header.Set("X-Tenant", s.cfg.SMLPaperlessTenant)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return models.SMLDocFormat{}, err
	}
	defer resp.Body.Close()

	var payload smlDocFormatResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&payload); err != nil {
		return models.SMLDocFormat{}, fmt.Errorf("cannot parse SML response")
	}

	if resp.StatusCode == http.StatusNotFound {
		return models.SMLDocFormat{}, errDocFormatNotFound
	}
	if resp.StatusCode == http.StatusConflict {
		return models.SMLDocFormat{}, errDocFormatAmbiguous
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return models.SMLDocFormat{}, errors.New(smlErrorMessage(payload.Error, payload.Message, resp.Status))
	}
	if !payload.Success {
		return models.SMLDocFormat{}, errors.New(smlErrorMessage(payload.Error, payload.Message, "SML request failed"))
	}
	if payload.Data.Code == "" {
		return models.SMLDocFormat{}, errDocFormatNotFound
	}
	return payload.Data, nil
}

func (s *Server) fetchSMLDocumentCandidates(ctx context.Context, docFormatCode, search, page, size string) (smlDocumentCandidatesResponse, error) {
	if strings.TrimSpace(s.cfg.SMLPaperlessBaseURL) == "" ||
		strings.TrimSpace(s.cfg.SMLPaperlessAPIKey) == "" ||
		strings.TrimSpace(s.cfg.SMLPaperlessTenant) == "" {
		return smlDocumentCandidatesResponse{}, errSMLConfigMissing
	}

	endpoint, err := url.Parse(s.cfg.SMLPaperlessBaseURL + "/api/v1/ic/document-candidates")
	if err != nil {
		return smlDocumentCandidatesResponse{}, fmt.Errorf("invalid SML base URL")
	}
	query := endpoint.Query()
	query.Set("doc_format_code", docFormatCode)
	if search != "" {
		query.Set("search", search)
	}
	if page != "" {
		query.Set("page", page)
	}
	if size != "" {
		query.Set("size", size)
	}
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return smlDocumentCandidatesResponse{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Api-Key", s.cfg.SMLPaperlessAPIKey)
	req.Header.Set("X-Tenant", s.cfg.SMLPaperlessTenant)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return smlDocumentCandidatesResponse{}, err
	}
	defer resp.Body.Close()

	var payload smlDocumentCandidatesResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&payload); err != nil {
		return smlDocumentCandidatesResponse{}, fmt.Errorf("cannot parse SML response")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return smlDocumentCandidatesResponse{}, errors.New(smlErrorMessage(payload.Error, payload.Message, resp.Status))
	}
	if !payload.Success {
		return smlDocumentCandidatesResponse{}, errors.New(smlErrorMessage(payload.Error, payload.Message, "SML request failed"))
	}
	return payload, nil
}

func (s *Server) fetchSMLDocumentCandidate(ctx context.Context, docFormatCode, docNo string) (models.SMLDocumentCandidate, error) {
	if strings.TrimSpace(s.cfg.SMLPaperlessBaseURL) == "" ||
		strings.TrimSpace(s.cfg.SMLPaperlessAPIKey) == "" ||
		strings.TrimSpace(s.cfg.SMLPaperlessTenant) == "" {
		return models.SMLDocumentCandidate{}, errSMLConfigMissing
	}
	endpoint, err := url.Parse(s.cfg.SMLPaperlessBaseURL + "/api/v1/ic/document-candidates/" + url.PathEscape(docNo))
	if err != nil {
		return models.SMLDocumentCandidate{}, fmt.Errorf("invalid SML base URL")
	}
	query := endpoint.Query()
	query.Set("doc_format_code", docFormatCode)
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return models.SMLDocumentCandidate{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Api-Key", s.cfg.SMLPaperlessAPIKey)
	req.Header.Set("X-Tenant", s.cfg.SMLPaperlessTenant)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return models.SMLDocumentCandidate{}, err
	}
	defer resp.Body.Close()

	var payload smlDocumentCandidateResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&payload); err != nil {
		return models.SMLDocumentCandidate{}, fmt.Errorf("cannot parse SML response")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return models.SMLDocumentCandidate{}, errors.New(smlErrorMessage(payload.Error, payload.Message, resp.Status))
	}
	if !payload.Success {
		return models.SMLDocumentCandidate{}, errors.New(smlErrorMessage(payload.Error, payload.Message, "SML request failed"))
	}
	return payload.Data, nil
}

func (s *Server) lockSMLDocument(ctx context.Context, docNo string) (map[string]any, error) {
	if strings.TrimSpace(s.cfg.SMLPaperlessBaseURL) == "" ||
		strings.TrimSpace(s.cfg.SMLPaperlessAPIKey) == "" ||
		strings.TrimSpace(s.cfg.SMLPaperlessTenant) == "" {
		return nil, errSMLConfigMissing
	}
	endpoint, err := url.Parse(s.cfg.SMLPaperlessBaseURL + "/api/v1/documents/" + url.PathEscape(docNo) + "/lock")
	if err != nil {
		return nil, fmt.Errorf("invalid SML base URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Api-Key", s.cfg.SMLPaperlessAPIKey)
	req.Header.Set("X-Tenant", s.cfg.SMLPaperlessTenant)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var payload smlLockResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("cannot parse SML response")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, errors.New(smlErrorMessage(payload.Error, payload.Message, resp.Status))
	}
	if !payload.Success {
		return nil, errors.New(smlErrorMessage(payload.Error, payload.Message, "SML request failed"))
	}
	return map[string]any{
		"docNo":         payload.Data.DocNo,
		"table":         payload.Data.Table,
		"transFlag":     payload.Data.TransFlag,
		"isLockRecord":  payload.Data.IsLockRecord,
		"alreadyLocked": payload.Data.AlreadyLocked,
	}, nil
}

func smlErrorMessage(apiErr *smlAPIError, message, fallback string) string {
	if apiErr != nil && apiErr.Message != "" {
		return apiErr.Message
	}
	if message != "" {
		return message
	}
	return fallback
}
