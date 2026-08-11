package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/bosocmputer/paperless-v2/backend/internal/models"
	"github.com/bosocmputer/paperless-v2/backend/internal/store"
)

var (
	errDocFormatNotFound          = errors.New("doc format code was not found in SML")
	errDocFormatAmbiguous         = errors.New("doc format code matches more than one screen code in SML")
	errDocFormatInvalidScreenCode = errors.New("doc format code has no valid screen code in SML")
	errSMLConfigMissing           = errors.New("sml paperless api config is incomplete")
)

func smlLookupErrorView(err error) (string, int, string) {
	text := strings.ToLower(err.Error())
	switch {
	case strings.Contains(text, "no active document found") || strings.Contains(text, "not found"):
		return "sml_document_not_found", http.StatusNotFound, "ไม่พบเลขเอกสารนี้ใน SML"
	case errors.Is(err, context.DeadlineExceeded) || strings.Contains(text, "timeout") || strings.Contains(text, "deadline exceeded") || strings.Contains(text, "connection refused"):
		return "sml_unavailable", http.StatusBadGateway, "เชื่อมต่อ SML ไม่สำเร็จ กรุณาลองใหม่"
	default:
		return "sml_unavailable", http.StatusBadGateway, "เชื่อมต่อ SML ไม่สำเร็จ กรุณาลองใหม่"
	}
}

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

type smlDocumentCandidatesBatchResponse struct {
	Success       bool                          `json:"success"`
	Data          []models.SMLDocumentCandidate `json:"data"`
	MissingDocNos []string                      `json:"missingDocNos"`
	Error         *smlAPIError                  `json:"error"`
	Message       string                        `json:"message"`
}

type smlCompanyProfileResponse struct {
	Success bool                     `json:"success"`
	Data    models.SMLCompanyProfile `json:"data"`
	Error   *smlAPIError             `json:"error"`
	Message string                   `json:"message"`
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
	Details any    `json:"details,omitempty"`
}

type smlRequestError struct {
	Code    string
	Message string
	Details any
}

type smlSourceStateError struct {
	State   string
	Message string
	// Diff is populated only when the mismatch is a revision comparison
	// (not a "document not found" case, where there is nothing to diff
	// against). Nil-safe: callers must check for nil/empty before use.
	Diff []smlSourceFieldDiff
}

func (e *smlSourceStateError) Error() string { return e.Message }

func (e *smlRequestError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Code != "" {
		return e.Code
	}
	return "SML request failed"
}

func (s *Server) smlTenantForContext(ctx context.Context) string {
	if tenant, ok := store.SMLTenantFromContext(ctx); ok {
		return store.NormalizeSMLTenant(tenant)
	}
	return store.NormalizeSMLTenant(s.cfg.SMLPaperlessTenant)
}

func (s *Server) hasSMLAPIConfig(ctx context.Context) (string, bool) {
	tenant := s.smlTenantForContext(ctx)
	return tenant, strings.TrimSpace(s.cfg.SMLPaperlessBaseURL) != "" &&
		strings.TrimSpace(s.cfg.SMLPaperlessAPIKey) != "" &&
		strings.TrimSpace(tenant) != ""
}

// verifySMLDocumentSource compares the immutable PaperLess snapshot with one
// exact SML lookup. It intentionally never runs in the broad search path.
func (s *Server) verifySMLDocumentSource(ctx context.Context, document models.SigningDocument) error {
	if !requiresSMLFinalization(document) {
		return nil
	}
	checkCtx := store.WithSMLTenant(ctx, document.SMLTenant)
	candidate, err := s.fetchSMLDocumentCandidate(checkCtx, document.DocFormatCode, document.DocNo)
	if err != nil {
		var requestErr *smlRequestError
		if errors.As(err, &requestErr) && requestErr.Code == "document_not_found" {
			return &smlSourceStateError{State: "sml_source_missing", Message: "ไม่พบเอกสารนี้ใน SML แล้ว กรุณายกเลิกเอกสารและนำเข้า PDF ฉบับล่าสุดใหม่"}
		}
		return err
	}
	revision := strings.TrimSpace(candidate.SourceRevision)
	if revision == "" {
		return fmt.Errorf("SML did not return a document source revision")
	}
	if strings.TrimSpace(document.SMLSourceRevision) == "" {
		if !legacySMLSourceMatchesDocument(candidate, document) {
			return &smlSourceStateError{
				State:   "sml_source_changed",
				Message: "ข้อมูลเอกสารใน SML ถูกแก้ไขจากข้อมูลที่นำเข้า กรุณายกเลิกเอกสารและนำเข้า PDF ฉบับล่าสุดใหม่",
				Diff:    diffSMLSourceCandidate(candidate, document),
			}
		}
		return s.store.RecordSMLSourceCheck(ctx, document.ID, revision)
	}
	if document.SMLSourceRevision != revision {
		return &smlSourceStateError{
			State:   "sml_source_changed",
			Message: "ข้อมูลเอกสารใน SML ถูกแก้ไขหลังเริ่มงาน กรุณายกเลิกเอกสารและนำเข้า PDF ฉบับล่าสุดใหม่",
			Diff:    diffSMLSourceCandidate(candidate, document),
		}
	}
	return s.store.RecordSMLSourceCheck(ctx, document.ID, revision)
}

// verifySMLDocumentSourceFailOpen wraps verifySMLDocumentSource for the
// per-signer-step check (see signMySigningTask/signPublicSigningTask). It
// exists because this check runs far more often than the original
// send/finalize checkpoints (once per signer, 2-10x per document) — if SML
// is briefly unreachable, that must not turn into "nobody can sign anything
// right now." Only a *confirmed* signal from SML (an actual source-changed
// or source-missing result) blocks the signature; a transport-level
// failure (timeout, connection refused, DNS, etc.) is logged and swallowed,
// on the assumption the existing finalize-time check remains as the
// authoritative backstop (defense-in-depth, not a weakened guarantee).
//
// A short timeout is applied here specifically so a slow/unreachable SML
// does not make every "sign" click hang — this check must stay fast or
// invisible, never a new source of user-perceived latency.
func (s *Server) verifySMLDocumentSourceFailOpen(ctx context.Context, document models.SigningDocument) error {
	if !requiresSMLFinalization(document) {
		return nil
	}
	checkCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	err := s.verifySMLDocumentSource(checkCtx, document)
	if err == nil {
		return nil
	}
	var sourceErr *smlSourceStateError
	if errors.As(err, &sourceErr) {
		// SML gave a definitive, confirmed answer: the source really did
		// change or really is gone. This is exactly what the check exists
		// to catch — block the document, same as at finalize time.
		return err
	}
	// Anything else (timeout, network error, malformed response, SML
	// mis-config) is not a confirmed "the document changed" signal — do
	// not block signing on it. finalizeCompletedDocument will still catch
	// a real drift later even if this step-level check missed it here.
	s.logger.Warn("per-step SML source check failed open", "error", err, "documentID", document.ID)
	return nil
}

// rebaselineSMLSourceRevisionsResult summarizes one run of
// rebaselineSMLSourceRevisions for the audit log / admin response.
type rebaselineSMLSourceRevisionsResult struct {
	Candidates  int                                 `json:"candidates"`
	Rebaselined int                                 `json:"rebaselined"`
	Skipped     int                                 `json:"skipped"`
	Failed      int                                 `json:"failed"`
	Documents   []rebaselineSMLSourceRevisionDetail `json:"documents"`
}

type rebaselineSMLSourceRevisionDetail struct {
	DocumentID  string `json:"documentId"`
	DocNo       string `json:"docNo"`
	OldRevision string `json:"oldRevision"`
	NewRevision string `json:"newRevision,omitempty"`
	Outcome     string `json:"outcome"` // "rebaselined" | "skipped" | "failed"
	Reason      string `json:"reason,omitempty"`
}

// rebaselineSMLSourceRevisions re-baselines every still-in-flight SML
// document's stored sml_source_revision against a live SML lookup. This
// exists to run once, right after a fix to the SML-side revision hash
// formula ships (see sml-api-bybos's candidateSourceRevisionBatchQuery) —
// without it, every in-flight document would compare a NEW-formula hash
// against an OLD-formula baseline at its next finalizeCompletedDocument
// call and be flagged sml_source_changed, even with zero real SML edits.
//
// This intentionally does NOT go through verifySMLDocumentSource's
// compare-and-reject branch: the old baseline is known to be computed
// under an obsolete formula, so the only correct action is to unconditionally
// replace it with a fresh snapshot, exactly like sendSigningDocument does
// the first time a document enters the workflow — not to judge whether
// something "changed" using a comparison basis that is itself invalid now.
func (s *Server) rebaselineSMLSourceRevisions(ctx context.Context) (rebaselineSMLSourceRevisionsResult, error) {
	candidates, err := s.store.ListRebaselineCandidates(ctx)
	if err != nil {
		return rebaselineSMLSourceRevisionsResult{}, err
	}
	result := rebaselineSMLSourceRevisionsResult{Candidates: len(candidates)}
	for _, c := range candidates {
		detail := rebaselineSMLSourceRevisionDetail{
			DocumentID:  c.ID,
			DocNo:       c.DocNo,
			OldRevision: c.Revision,
		}
		checkCtx := store.WithSMLTenant(ctx, c.SMLTenant)
		fresh, err := s.fetchSMLDocumentCandidate(checkCtx, c.DocFormatCode, c.DocNo)
		if err != nil {
			detail.Outcome = "failed"
			detail.Reason = err.Error()
			result.Failed++
			result.Documents = append(result.Documents, detail)
			continue
		}
		revision := strings.TrimSpace(fresh.SourceRevision)
		if revision == "" {
			detail.Outcome = "failed"
			detail.Reason = "SML did not return a document source revision"
			result.Failed++
			result.Documents = append(result.Documents, detail)
			continue
		}
		updated, err := s.store.RebaselineSMLSourceRevision(ctx, c.ID, c.Revision, revision)
		if err != nil {
			detail.Outcome = "failed"
			detail.Reason = err.Error()
			result.Failed++
			result.Documents = append(result.Documents, detail)
			continue
		}
		if !updated {
			// Optimistic-lock miss: a normal send/finalize already moved
			// this document's revision on since we listed it. Nothing to
			// do — it is already fresh from the ordinary flow.
			detail.Outcome = "skipped"
			detail.Reason = "revision already advanced by a concurrent send/finalize"
			result.Skipped++
			result.Documents = append(result.Documents, detail)
			continue
		}
		detail.Outcome = "rebaselined"
		detail.NewRevision = revision
		result.Rebaselined++
		result.Documents = append(result.Documents, detail)
	}
	return result, nil
}

// rebaselineSMLSourceRevisionsHandler is a superadmin-only, manually
// triggered operation. It is meant to run exactly once per shop, right
// after that shop's sml-api-bybos deploy carrying a revision-hash formula
// fix goes live (see rebaselineSMLSourceRevisions doc comment) — not on a
// schedule, and not automatically on deploy, since running it before the
// formula fix is live would just re-capture stale-formula hashes and solve
// nothing.
func (s *Server) rebaselineSMLSourceRevisionsHandler(w http.ResponseWriter, r *http.Request) {
	// Bounded by the number of in-flight documents at rebaseline time
	// (small — see ListRebaselineCandidates), not the full document
	// history, but each candidate needs a real SML round-trip, so give
	// this generous headroom relative to a single-document timeout.
	timeout := 10 * s.cfg.SMLPaperlessTimeout
	if timeout < 5*time.Minute {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	result, err := s.rebaselineSMLSourceRevisions(ctx)
	if err != nil {
		s.logger.Error("sml source revision rebaseline failed", "error", err)
		writeError(w, http.StatusInternalServerError, "sml_source_rebaseline_failed", "Rebaseline failed. Check server logs.")
		return
	}
	actor, _ := currentUser(r)
	if auditErr := s.store.WriteAuditWithMetadata(r.Context(), actor.ID, "sml.source_revision.rebaseline", "signing_documents", "", clientIP(r), r.UserAgent(), map[string]any{
		"candidates":  result.Candidates,
		"rebaselined": result.Rebaselined,
		"skipped":     result.Skipped,
		"failed":      result.Failed,
		"documents":   result.Documents,
	}); auditErr != nil {
		s.logger.Warn("write sml source revision rebaseline audit failed", "error", auditErr)
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": result})
}

func legacySMLSourceMatchesDocument(candidate models.SMLDocumentCandidate, document models.SigningDocument) bool {
	if !strings.EqualFold(strings.TrimSpace(candidate.DocNo), strings.TrimSpace(document.DocNo)) ||
		!strings.EqualFold(strings.TrimSpace(candidate.DocFormatCode), strings.TrimSpace(document.DocFormatCode)) {
		return false
	}
	if strings.TrimSpace(document.DocDate) != "" && strings.TrimSpace(candidate.DocDate) != strings.TrimSpace(document.DocDate) {
		return false
	}
	if strings.TrimSpace(document.PartyCode) != "" && !strings.EqualFold(strings.TrimSpace(candidate.PartyCode), strings.TrimSpace(document.PartyCode)) {
		return false
	}
	if strings.TrimSpace(document.SMLTable) != "" && !strings.EqualFold(strings.TrimSpace(candidate.Table), strings.TrimSpace(document.SMLTable)) {
		return false
	}
	if document.TransFlag != 0 && candidate.TransFlag != document.TransFlag {
		return false
	}
	return math.Abs(candidate.TotalAmount-document.TotalAmount) < 0.005
}

// smlSourceFieldDiff is one field whose stored (PaperLess-side, as of send
// time) and current (freshly fetched from SML) values disagree. Field names
// are internal identifiers, not shown to end users — this is diagnostic
// data for support staff via the audit/event log, not customer-facing copy.
type smlSourceFieldDiff struct {
	Field   string `json:"field"`
	Stored  string `json:"stored"`
	Current string `json:"current"`
}

// diffSMLSourceCandidate compares the same coarse fields
// legacySMLSourceMatchesDocument already checks (doc_no, doc_format_code,
// doc_date, party_code, table, trans_flag, total_amount) and reports which
// ones actually disagree, instead of collapsing the comparison to a single
// bool. This exists so a "sml_source_changed" rejection can answer "what
// changed" from the audit log alone, without a manual DB investigation —
// this is the answer to "เช็คจากไหน" (which field(s) triggered the check).
//
// It intentionally reuses the SAME field set and tolerance as
// legacySMLSourceMatchesDocument rather than diffing the full opaque hash
// input — the hash itself covers far more columns (see sml-api-bybos's
// candidateSourceRevisionBatchQuery) than PaperLess has a local copy of to
// diff against; this stays a coarse, best-effort diagnostic aid; it is not
// guaranteed to be exhaustive on the exact field(s) that flipped the hash.
func diffSMLSourceCandidate(candidate models.SMLDocumentCandidate, document models.SigningDocument) []smlSourceFieldDiff {
	var diffs []smlSourceFieldDiff
	add := func(field, stored, current string) {
		if stored != current {
			diffs = append(diffs, smlSourceFieldDiff{Field: field, Stored: stored, Current: current})
		}
	}
	addFold := func(field, stored, current string) {
		if !strings.EqualFold(strings.TrimSpace(stored), strings.TrimSpace(current)) {
			diffs = append(diffs, smlSourceFieldDiff{Field: field, Stored: stored, Current: current})
		}
	}
	addFold("doc_no", document.DocNo, candidate.DocNo)
	addFold("doc_format_code", document.DocFormatCode, candidate.DocFormatCode)
	if strings.TrimSpace(document.DocDate) != "" {
		add("doc_date", document.DocDate, candidate.DocDate)
	}
	if strings.TrimSpace(document.PartyCode) != "" {
		addFold("party_code", document.PartyCode, candidate.PartyCode)
	}
	if strings.TrimSpace(document.SMLTable) != "" {
		addFold("sml_table", document.SMLTable, candidate.Table)
	}
	if document.TransFlag != 0 && candidate.TransFlag != document.TransFlag {
		diffs = append(diffs, smlSourceFieldDiff{
			Field: "trans_flag", Stored: fmt.Sprintf("%d", document.TransFlag), Current: fmt.Sprintf("%d", candidate.TransFlag),
		})
	}
	if math.Abs(candidate.TotalAmount-document.TotalAmount) >= 0.005 {
		diffs = append(diffs, smlSourceFieldDiff{
			Field: "total_amount", Stored: fmt.Sprintf("%.2f", document.TotalAmount), Current: fmt.Sprintf("%.2f", candidate.TotalAmount),
		})
	}
	return diffs
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
		"tenant":      s.smlTenantForContext(r.Context()),
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
		"tenant":      s.smlTenantForContext(r.Context()),
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
		"tenant":      s.smlTenantForContext(r.Context()),
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

func (s *Server) getSMLDocumentCandidate(w http.ResponseWriter, r *http.Request) {
	docFormatCode := strings.TrimSpace(r.URL.Query().Get("doc_format_code"))
	docNo := strings.TrimSpace(r.PathValue("docNo"))
	if docFormatCode == "" {
		writeError(w, http.StatusBadRequest, "doc_format_code_required", "doc_format_code is required.")
		return
	}
	if docNo == "" {
		writeError(w, http.StatusBadRequest, "doc_no_required", "doc_no is required.")
		return
	}
	candidate, err := s.fetchSMLDocumentCandidate(r.Context(), docFormatCode, docNo)
	if errors.Is(err, errSMLConfigMissing) {
		writeError(w, http.StatusServiceUnavailable, "sml_not_configured", "SML Paperless API is not configured.")
		return
	}
	if err != nil {
		s.logger.Warn("fetch sml document candidate failed", "error", err, "docFormatCode", docFormatCode, "docNo", docNo)
		code, status, message := smlLookupErrorView(err)
		writeError(w, status, code, message)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"document": candidate})
}

func (s *Server) fetchSMLDocFormats(ctx context.Context, screenCode string) ([]models.SMLDocFormat, error) {
	tenant, ok := s.hasSMLAPIConfig(ctx)
	if !ok {
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
	req.Header.Set("X-Tenant", tenant)

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
		requestErr := newSMLRequestError(payload.Error, payload.Message, resp.Status)
		s.invalidateTenantReadinessForStructuralError(ctx, requestErr)
		return nil, requestErr
	}
	if !payload.Success {
		requestErr := newSMLRequestError(payload.Error, payload.Message, "SML request failed")
		s.invalidateTenantReadinessForStructuralError(ctx, requestErr)
		return nil, requestErr
	}

	for i := range payload.Data {
		if payload.Data[i].ScreenCode == "" {
			payload.Data[i].ScreenCode = screenCode
		}
	}
	return payload.Data, nil
}

func (s *Server) fetchSMLCompanyProfile(ctx context.Context) (models.SMLCompanyProfile, error) {
	tenant, ok := s.hasSMLAPIConfig(ctx)
	if !ok {
		return models.SMLCompanyProfile{}, errSMLConfigMissing
	}
	endpoint := s.cfg.SMLPaperlessBaseURL + "/api/v1/company-profile"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return models.SMLCompanyProfile{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Api-Key", s.cfg.SMLPaperlessAPIKey)
	req.Header.Set("X-Tenant", tenant)
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return models.SMLCompanyProfile{}, err
	}
	defer resp.Body.Close()
	var payload smlCompanyProfileResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return models.SMLCompanyProfile{}, fmt.Errorf("cannot parse SML company profile response")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices || !payload.Success {
		requestErr := newSMLRequestError(payload.Error, payload.Message, "cannot load SML company profile")
		s.invalidateTenantReadinessForStructuralError(ctx, requestErr)
		return models.SMLCompanyProfile{}, requestErr
	}
	if strings.TrimSpace(payload.Data.DisplayName) == "" {
		return models.SMLCompanyProfile{}, &smlRequestError{Code: "company_profile_name_missing", Message: "company profile has no company name"}
	}
	return payload.Data, nil
}

func (s *Server) fetchSMLDocFormatByCode(ctx context.Context, docFormatCode string) (models.SMLDocFormat, error) {
	tenant, ok := s.hasSMLAPIConfig(ctx)
	if !ok {
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
	req.Header.Set("X-Tenant", tenant)

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
	tenant, ok := s.hasSMLAPIConfig(ctx)
	if !ok {
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
	req.Header.Set("X-Tenant", tenant)

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
	tenant, ok := s.hasSMLAPIConfig(ctx)
	if !ok {
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
	req.Header.Set("X-Tenant", tenant)

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
		return models.SMLDocumentCandidate{}, newSMLRequestError(payload.Error, payload.Message, resp.Status)
	}
	if !payload.Success {
		return models.SMLDocumentCandidate{}, newSMLRequestError(payload.Error, payload.Message, "SML request failed")
	}
	return payload.Data, nil
}

func (s *Server) fetchSMLDocumentCandidatesBatch(ctx context.Context, docFormatCode string, docNos []string) (smlDocumentCandidatesBatchResponse, error) {
	tenant, ok := s.hasSMLAPIConfig(ctx)
	if !ok {
		return smlDocumentCandidatesBatchResponse{}, errSMLConfigMissing
	}
	endpoint, err := url.Parse(s.cfg.SMLPaperlessBaseURL + "/api/v1/ic/document-candidates/batch")
	if err != nil {
		return smlDocumentCandidatesBatchResponse{}, fmt.Errorf("invalid SML base URL")
	}
	body, err := json.Marshal(map[string]any{"docFormatCode": docFormatCode, "docNos": docNos})
	if err != nil {
		return smlDocumentCandidatesBatchResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), strings.NewReader(string(body)))
	if err != nil {
		return smlDocumentCandidatesBatchResponse{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", s.cfg.SMLPaperlessAPIKey)
	req.Header.Set("X-Tenant", tenant)
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return smlDocumentCandidatesBatchResponse{}, err
	}
	defer resp.Body.Close()
	var payload smlDocumentCandidatesBatchResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&payload); err != nil {
		return smlDocumentCandidatesBatchResponse{}, fmt.Errorf("cannot parse SML response")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return smlDocumentCandidatesBatchResponse{}, newSMLRequestError(payload.Error, payload.Message, resp.Status)
	}
	if !payload.Success {
		return smlDocumentCandidatesBatchResponse{}, newSMLRequestError(payload.Error, payload.Message, "SML request failed")
	}
	return payload, nil
}

func (s *Server) lockSMLDocument(ctx context.Context, docNo string) (map[string]any, error) {
	tenant, ok := s.hasSMLAPIConfig(ctx)
	if !ok {
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
	req.Header.Set("X-Tenant", tenant)

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
		requestErr := newSMLRequestError(payload.Error, payload.Message, resp.Status)
		s.invalidateTenantReadinessForStructuralError(ctx, requestErr)
		return nil, requestErr
	}
	if !payload.Success {
		requestErr := newSMLRequestError(payload.Error, payload.Message, "SML request failed")
		s.invalidateTenantReadinessForStructuralError(ctx, requestErr)
		return nil, requestErr
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

func newSMLRequestError(apiErr *smlAPIError, message, fallback string) error {
	if apiErr != nil {
		return &smlRequestError{
			Code:    apiErr.Code,
			Message: smlErrorMessage(apiErr, message, fallback),
			Details: apiErr.Details,
		}
	}
	return &smlRequestError{Message: smlErrorMessage(nil, message, fallback)}
}
