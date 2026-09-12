package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/bosocmputer/paperless-v2/backend/internal/models"
	"github.com/bosocmputer/paperless-v2/backend/internal/store"
)

// smlDocumentImagesMaxBytes bounds how much of an upstream image response we are
// willing to relay, mirroring the per-image ceiling sml-api enforces at both ends.
const smlDocumentImagesMaxBytes = 4 << 20

type smlDocumentImageListItem struct {
	PageNo int    `json:"page_no"`
	GUID   string `json:"guid_code"`
	Bytes  int    `json:"bytes"`
}

type smlDocumentImagesListResponse struct {
	Success bool `json:"success"`
	Data    struct {
		DocNo      string                     `json:"doc_no"`
		ImageCount int                        `json:"image_count"`
		TotalBytes int                        `json:"total_bytes"`
		Images     []smlDocumentImageListItem `json:"images"`
	} `json:"data"`
	Error   *smlAPIError `json:"error"`
	Message string       `json:"message"`
}

func (s *Server) fetchSMLDocumentImageList(ctx context.Context, docNo string) (smlDocumentImagesListResponse, error) {
	var payload smlDocumentImagesListResponse

	tenant, ok := s.hasSMLAPIConfig(ctx)
	if !ok {
		return payload, errSMLConfigMissing
	}

	endpoint, err := url.Parse(s.cfg.SMLPaperlessBaseURL + "/api/v1/documents/" + url.PathEscape(docNo) + "/images")
	if err != nil {
		return payload, fmt.Errorf("invalid SML base URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return payload, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Api-Key", s.cfg.SMLPaperlessAPIKey)
	req.Header.Set("X-Tenant", tenant)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return payload, err
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&payload); err != nil {
		return payload, fmt.Errorf("cannot parse SML response")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return payload, newSMLRequestError(payload.Error, payload.Message, resp.Status)
	}
	if !payload.Success {
		return payload, newSMLRequestError(payload.Error, payload.Message, "SML request failed")
	}
	return payload, nil
}

// streamSMLDocumentImage relays one image's bytes straight to w. The upstream
// response body is copied rather than buffered so concurrent viewers do not each
// pin a multi-megabyte image in memory.
func (s *Server) streamSMLDocumentImage(ctx context.Context, w http.ResponseWriter, docNo string, pageNo int) error {
	tenant, ok := s.hasSMLAPIConfig(ctx)
	if !ok {
		return errSMLConfigMissing
	}

	endpoint, err := url.Parse(s.cfg.SMLPaperlessBaseURL + "/api/v1/documents/" + url.PathEscape(docNo) + "/images/" + strconv.Itoa(pageNo) + "/file")
	if err != nil {
		return fmt.Errorf("invalid SML base URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "image/jpeg")
	req.Header.Set("X-Api-Key", s.cfg.SMLPaperlessAPIKey)
	req.Header.Set("X-Tenant", tenant)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		// An error response is JSON, not an image: decode it so the caller can map
		// the upstream code onto a message the UI knows how to present.
		var payload smlDocumentImagesListResponse
		_ = json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&payload)
		return newSMLRequestError(payload.Error, payload.Message, resp.Status)
	}

	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if !strings.HasPrefix(strings.ToLower(contentType), "image/") {
		return fmt.Errorf("unexpected content type from SML")
	}

	// These are signed business documents: no browser or intermediary should keep
	// a copy, matching how PaperLess serves its own document files.
	setNoStoreHeaders(w)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if length := strings.TrimSpace(resp.Header.Get("Content-Length")); length != "" {
		w.Header().Set("Content-Length", length)
	}
	w.WriteHeader(http.StatusOK)

	if _, err := io.Copy(w, io.LimitReader(resp.Body, smlDocumentImagesMaxBytes)); err != nil {
		// Headers are already committed, so the response cannot be turned into an
		// error now; the client most likely disconnected mid-image.
		return nil
	}
	return nil
}

// listSigningDocumentSMLImages reports the images SML holds for a document.
// SML ERP's own screen only displays the first 8, so this is the only place a
// user can see every image a document actually has.
func (s *Server) listSigningDocumentSMLImages(w http.ResponseWriter, r *http.Request) {
	user, document, ok := s.authorizeSigningDocumentAttachmentAccess(w, r)
	if !ok {
		return
	}
	if !documentSupportsSMLImages(document) {
		writeError(w, http.StatusNotFound, "sml_images_unsupported", "This document does not have SML images.")
		return
	}

	ctx := store.WithSMLTenant(r.Context(), document.SMLTenant)
	payload, err := s.fetchSMLDocumentImageList(ctx, document.DocNo)
	if err != nil {
		writeSMLDocumentImagesError(w, err)
		return
	}

	images := payload.Data.Images
	if images == nil {
		images = []smlDocumentImageListItem{}
	}

	// Audited once per listing rather than per image: a viewer paging through a
	// gallery would otherwise write one audit row per page view.
	metadata := map[string]any{
		"documentId": document.ID,
		"docNo":      document.DocNo,
		"imageCount": payload.Data.ImageCount,
		"totalBytes": payload.Data.TotalBytes,
	}
	if err := s.store.WriteAuditWithMetadata(r.Context(), user.ID, "signing_document.sml_images_view", "signing_document", document.ID, clientIP(r), r.UserAgent(), metadata); err != nil {
		s.logger.Warn("write sml images view audit failed", "error", err, "documentID", document.ID)
	}

	setNoStoreHeaders(w)
	writeJSON(w, http.StatusOK, map[string]any{
		"docNo":      payload.Data.DocNo,
		"imageCount": payload.Data.ImageCount,
		"totalBytes": payload.Data.TotalBytes,
		"images":     images,
	})
}

// getSigningDocumentSMLImageFile streams one stored SML image to the viewer.
func (s *Server) getSigningDocumentSMLImageFile(w http.ResponseWriter, r *http.Request) {
	_, document, ok := s.authorizeSigningDocumentAttachmentAccess(w, r)
	if !ok {
		return
	}
	if !documentSupportsSMLImages(document) {
		writeError(w, http.StatusNotFound, "sml_images_unsupported", "This document does not have SML images.")
		return
	}

	pageNo, err := strconv.Atoi(strings.TrimSpace(r.PathValue("pageNo")))
	if err != nil || pageNo < 1 {
		writeError(w, http.StatusBadRequest, "sml_image_page_invalid", "Image page number is invalid.")
		return
	}

	ctx := store.WithSMLTenant(r.Context(), document.SMLTenant)
	if err := s.streamSMLDocumentImage(ctx, w, document.DocNo, pageNo); err != nil {
		writeSMLDocumentImagesError(w, err)
		return
	}
}

// documentSupportsSMLImages reports whether images could exist for a document at
// all. Internal documents never reach SML, so they have no images by definition.
func documentSupportsSMLImages(document models.SigningDocument) bool {
	if strings.EqualFold(strings.TrimSpace(document.DocumentSource), "internal") {
		return false
	}
	return strings.TrimSpace(document.DocNo) != ""
}

// writeSMLDocumentImagesError maps an upstream failure onto a code the UI turns
// into a message that tells the user what to do next.
func writeSMLDocumentImagesError(w http.ResponseWriter, err error) {
	if errors.Is(err, errSMLConfigMissing) {
		writeError(w, http.StatusServiceUnavailable, "sml_not_configured", "SML connection is not configured.")
		return
	}
	var smlErr *smlRequestError
	if errors.As(err, &smlErr) {
		switch strings.TrimSpace(smlErr.Code) {
		case "document_image_not_found":
			writeError(w, http.StatusNotFound, "sml_image_not_found", "This image was not found in SML.")
			return
		case "document_image_too_large":
			writeError(w, http.StatusUnprocessableEntity, "sml_image_too_large", "This image is too large to display.")
			return
		}
	}
	writeError(w, http.StatusBadGateway, "sml_images_unavailable", "Cannot reach SML to load images right now.")
}
