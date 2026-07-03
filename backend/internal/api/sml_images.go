package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

type smlDocumentImagesRequest struct {
	Images     []smlDocumentImageRequestItem `json:"images"`
	TotalPages int                           `json:"totalPages,omitempty"`
	Truncated  bool                          `json:"truncated,omitempty"`
}

type smlDocumentImageRequestItem struct {
	PageNo      int    `json:"pageNo"`
	ContentType string `json:"contentType"`
	SHA256      string `json:"sha256"`
	Data        []byte `json:"data"`
}

type smlDocumentImagesResponse struct {
	Success bool `json:"success"`
	Data    struct {
		DocNo      string `json:"doc_no"`
		ImageCount int    `json:"image_count"`
		TotalPages int    `json:"total_pages"`
		Truncated  bool   `json:"truncated"`
		TotalBytes int    `json:"total_bytes"`
		Images     []struct {
			PageNo int    `json:"page_no"`
			GUID   string `json:"guid_code"`
			SHA256 string `json:"sha256"`
			Bytes  int    `json:"bytes"`
		} `json:"images"`
		TargetTables []string `json:"target_tables"`
	} `json:"data"`
	Error   *smlAPIError `json:"error"`
	Message string       `json:"message"`
}

func (s *Server) replaceSMLDocumentImages(ctx context.Context, docNo string, render pdfSnapshotRenderResult) (map[string]any, error) {
	tenant, ok := s.hasSMLAPIConfig(ctx)
	if !ok {
		return nil, errSMLConfigMissing
	}

	items := make([]smlDocumentImageRequestItem, 0, len(render.Images))
	for _, image := range render.Images {
		items = append(items, smlDocumentImageRequestItem{
			PageNo:      image.PageNo,
			ContentType: image.ContentType,
			SHA256:      image.SHA256,
			Data:        image.Data,
		})
	}
	body, err := json.Marshal(smlDocumentImagesRequest{
		Images:     items,
		TotalPages: render.TotalPages,
		Truncated:  render.Truncated,
	})
	if err != nil {
		return nil, err
	}

	endpoint, err := url.Parse(s.cfg.SMLPaperlessBaseURL + "/api/v1/documents/" + url.PathEscape(docNo) + "/images")
	if err != nil {
		return nil, fmt.Errorf("invalid SML base URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", s.cfg.SMLPaperlessAPIKey)
	req.Header.Set("X-Tenant", tenant)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var payload smlDocumentImagesResponse
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
		"docNo":        payload.Data.DocNo,
		"imageCount":   payload.Data.ImageCount,
		"totalPages":   payload.Data.TotalPages,
		"truncated":    payload.Data.Truncated,
		"totalBytes":   payload.Data.TotalBytes,
		"targetTables": payload.Data.TargetTables,
	}, nil
}
