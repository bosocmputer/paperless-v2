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

func smlErrorMessage(apiErr *smlAPIError, message, fallback string) string {
	if apiErr != nil && apiErr.Message != "" {
		return apiErr.Message
	}
	if message != "" {
		return message
	}
	return fallback
}
