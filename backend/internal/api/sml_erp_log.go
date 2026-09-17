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

	"github.com/bosocmputer/paperless-v2/backend/internal/models"
	"github.com/bosocmputer/paperless-v2/backend/internal/store"
)

// This file talks to sml-api-bybos's erp-log endpoints, which read SML's own
// audit trail — the same table the SML ERP "ประวัติ" screen displays. Using it
// means PaperLess and SML tell a user the same story about who changed a
// document and when.
//
// It replaces a content fingerprint of the raw SML rows. That fingerprint was
// accurate about bytes but wrong about intent: it also covered columns SML
// rewrites by itself (used_status when a purchase order gets consumed, stock
// recalculation, transient edit markers), so documents nobody had opened were
// reported to users as edited. The audit trail only records what a person
// actually did through the ERP UI.

// errSMLAuditLogUnsupported means the shop's SML tenant has no audit database,
// so no edit history exists to check against — a permanent configuration, not
// an outage. Shops whose SML database was restored from a backup arrive
// without their _logs sibling and SML never creates one.
//
// Documents in such a shop fall back to the previous content-fingerprint
// check, which keeps them working exactly as they do today rather than
// blocking every document on a question that can never be answered there.
var errSMLAuditLogUnsupported = errors.New("sml tenant has no audit log database")

// smlERPLogBaseline mirrors sml-api-bybos's ERPLogBaseline.
type smlERPLogBaseline struct {
	Roworder   int64  `json:"roworder"`
	LogCount   int    `json:"logCount"`
	CapturedAt string `json:"capturedAt"`
}

// smlERPFieldChange is one normalized before/after difference from the audit
// trail, already translated to Thai field labels by sml-api.
type smlERPFieldChange struct {
	Section  string `json:"section"`
	Field    string `json:"field"`
	Label    string `json:"label"`
	RowKey   string `json:"rowKey,omitempty"`
	RowLabel string `json:"rowLabel,omitempty"`
	Old      string `json:"old"`
	New      string `json:"new"`
}

// smlERPLogStatus mirrors sml-api-bybos's ERPLogStatus.
type smlERPLogStatus struct {
	DocNo                string              `json:"docNo"`
	TransFlag            int                 `json:"transFlag"`
	Baseline             int64               `json:"baseline"`
	CurrentRoworder      int64               `json:"currentRoworder"`
	EditedAfterBaseline  bool                `json:"editedAfterBaseline"`
	CreatedAfterBaseline bool                `json:"createdAfterBaseline"`
	DeletedAfterBaseline bool                `json:"deletedAfterBaseline"`
	HasMeaningfulChange  bool                `json:"hasMeaningfulChange"`
	ChangeCount          int                 `json:"changeCount"`
	DiffUnavailable      bool                `json:"diffUnavailable"`
	LatestEditor         string              `json:"latestEditor"`
	LatestComputer       string              `json:"latestComputer"`
	LatestMenu           string              `json:"latestMenu"`
	LatestEditedAt       string              `json:"latestEditedAt"`
	Changes              []smlERPFieldChange `json:"changes"`
}

type smlERPLogBaselineResponse struct {
	Success bool              `json:"success"`
	Data    smlERPLogBaseline `json:"data"`
	Error   json.RawMessage   `json:"error"`
	Message string            `json:"message"`
}

type smlERPLogStatusResponse struct {
	Success bool            `json:"success"`
	Data    smlERPLogStatus `json:"data"`
	Error   json.RawMessage `json:"error"`
	Message string          `json:"message"`
}

type smlERPLogHistoryEntry struct {
	Roworder     int64               `json:"roworder"`
	FunctionCode int                 `json:"functionCode"`
	Action       string              `json:"action"`
	UserCode     string              `json:"userCode"`
	ComputerName string              `json:"computerName"`
	MenuName     string              `json:"menuName"`
	DateTime     string              `json:"dateTime"`
	DocAmount    float64             `json:"docAmount"`
	OldDocAmount float64             `json:"oldDocAmount"`
	Changes      []smlERPFieldChange `json:"changes"`
	DiffSkipped  string              `json:"diffSkipped"`
}

type smlERPLogHistoryResponse struct {
	Success   bool                    `json:"success"`
	Data      []smlERPLogHistoryEntry `json:"data"`
	Truncated bool                    `json:"truncated"`
	Limit     int                     `json:"limit"`
	Error     json.RawMessage         `json:"error"`
	Message   string                  `json:"message"`
}

// smlERPLogRequest issues one audit-trail request and decodes it into out.
func (s *Server) smlERPLogRequest(ctx context.Context, tenant, path, docNo string, query url.Values, out any) error {
	endpoint, err := url.Parse(s.cfg.SMLPaperlessBaseURL + "/api/v1/ic/documents/" + url.PathEscape(docNo) + path)
	if err != nil {
		return fmt.Errorf("invalid SML base URL")
	}
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Api-Key", s.cfg.SMLPaperlessAPIKey)
	req.Header.Set("X-Tenant", tenant)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(out); err != nil {
		return fmt.Errorf("cannot parse SML response")
	}
	// 501 means this tenant has no audit database at all — a permanent
	// property of shops whose SML database was restored from a backup without
	// its _logs sibling, not a transient outage. Distinguished here so the
	// caller can fall back instead of retrying forever.
	if resp.StatusCode == http.StatusNotImplemented {
		return errSMLAuditLogUnsupported
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("SML audit log request failed: %s", resp.Status)
	}
	return nil
}

// fetchSMLERPLogBaseline captures the audit-trail marker for a document at
// the moment a signing job starts.
func (s *Server) fetchSMLERPLogBaseline(ctx context.Context, docNo string, transFlag int) (smlERPLogBaseline, error) {
	tenant, ok := s.hasSMLAPIConfig(ctx)
	if !ok {
		return smlERPLogBaseline{}, errSMLConfigMissing
	}
	query := url.Values{}
	query.Set("trans_flag", strconv.Itoa(transFlag))

	var payload smlERPLogBaselineResponse
	if err := s.smlERPLogRequest(ctx, tenant, "/erp-log-baseline", docNo, query, &payload); err != nil {
		return smlERPLogBaseline{}, err
	}
	if !payload.Success {
		return smlERPLogBaseline{}, fmt.Errorf("SML audit log baseline failed")
	}
	return payload.Data, nil
}

// fetchSMLERPLogStatus asks whether anything happened to a document after its
// baseline.
func (s *Server) fetchSMLERPLogStatus(ctx context.Context, docNo string, transFlag int, baseline int64) (smlERPLogStatus, error) {
	tenant, ok := s.hasSMLAPIConfig(ctx)
	if !ok {
		return smlERPLogStatus{}, errSMLConfigMissing
	}
	query := url.Values{}
	query.Set("trans_flag", strconv.Itoa(transFlag))
	query.Set("baseline", strconv.FormatInt(baseline, 10))

	var payload smlERPLogStatusResponse
	if err := s.smlERPLogRequest(ctx, tenant, "/erp-log-status", docNo, query, &payload); err != nil {
		return smlERPLogStatus{}, err
	}
	if !payload.Success {
		return smlERPLogStatus{}, fmt.Errorf("SML audit log status failed")
	}
	return payload.Data, nil
}

// fetchSMLERPLogHistory returns the decoded change list for display.
func (s *Server) fetchSMLERPLogHistory(ctx context.Context, docNo string, transFlag int, baseline int64) (smlERPLogHistoryResponse, error) {
	tenant, ok := s.hasSMLAPIConfig(ctx)
	if !ok {
		return smlERPLogHistoryResponse{}, errSMLConfigMissing
	}
	query := url.Values{}
	query.Set("trans_flag", strconv.Itoa(transFlag))
	query.Set("baseline", strconv.FormatInt(baseline, 10))

	var payload smlERPLogHistoryResponse
	if err := s.smlERPLogRequest(ctx, tenant, "/erp-log-history", docNo, query, &payload); err != nil {
		return smlERPLogHistoryResponse{}, err
	}
	if !payload.Success {
		return smlERPLogHistoryResponse{}, fmt.Errorf("SML audit log history failed")
	}
	return payload, nil
}

// captureSMLSourceBaseline records the audit-trail marker for a newly created
// signing job.
//
// A failure here is logged but never fails document creation: being unable to
// reach SML for a bookkeeping marker must not stop a user from starting work.
// The document keeps baseline -1, which verifySMLDocumentSourceViaERPLogs
// treats as "never measured" and repairs on its first successful check rather
// than silently passing an unverified document.
func (s *Server) captureSMLSourceBaseline(ctx context.Context, document models.SigningDocument) {
	if !requiresSMLFinalization(document) {
		return
	}
	checkCtx := store.WithSMLTenant(ctx, document.SMLTenant)
	baseline, err := s.fetchSMLERPLogBaseline(checkCtx, document.DocNo, document.TransFlag)
	if err != nil {
		if errors.Is(err, errSMLAuditLogUnsupported) {
			// Expected and permanent for shops without an audit database.
			// Logging a warning on every document creation there would be
			// pure noise that buries real failures.
			return
		}
		s.logger.Warn("capture SML audit baseline failed",
			"error", err, "documentID", document.ID, "docNo", document.DocNo)
		return
	}
	if err := s.store.RecordSMLSourceBaseline(ctx, document.ID, baseline.Roworder); err != nil {
		s.logger.Warn("record SML audit baseline failed",
			"error", err, "documentID", document.ID, "docNo", document.DocNo)
	}
}

// verifySMLDocumentSourceViaERPLogs answers "did anyone edit this document
// after we started?" from SML's audit trail.
//
// Called only after the caller has already confirmed the document still
// exists in SML, since the audit trail cannot report deletions (SML's cancel
// path writes no delete row).
//
// Three outcomes block the document:
//   - the document number was recreated (a different document now holds it)
//   - a delete row exists
//   - an edit changed at least one user-visible field
//
// An edit whose normalized diff is empty does NOT block. Over half of
// production edit rows are plain re-saves where nothing a person typed
// changed, and blocking on those is precisely what made the previous check
// untrustworthy to users.
func (s *Server) verifySMLDocumentSourceViaERPLogs(ctx, checkCtx context.Context, document models.SigningDocument) error {
	baseline := document.SMLSourceBaselineRow
	if baseline < 0 {
		// No baseline was captured — the document predates this feature, or
		// SML was unreachable at creation time. There is nothing to compare
		// against, so adopt the current position as the baseline rather than
		// comparing against 0, which would report the document's entire
		// history as new edits and block every legacy document at once.
		current, err := s.fetchSMLERPLogBaseline(checkCtx, document.DocNo, document.TransFlag)
		if err != nil {
			return err
		}
		s.logger.Info("adopting SML audit baseline for document without one",
			"documentID", document.ID, "docNo", document.DocNo, "baseline", current.Roworder)
		return s.store.RecordSMLSourceBaseline(ctx, document.ID, current.Roworder)
	}

	status, err := s.fetchSMLERPLogStatus(checkCtx, document.DocNo, document.TransFlag, baseline)
	if err != nil {
		return err
	}

	if status.HasMeaningfulChange {
		s.logger.Info("SML document blocked by audit trail",
			"documentID", document.ID,
			"docNo", document.DocNo,
			"transFlag", document.TransFlag,
			"baseline", baseline,
			"currentRoworder", status.CurrentRoworder,
			"recreated", status.CreatedAfterBaseline,
			"deleted", status.DeletedAfterBaseline,
			"edited", status.EditedAfterBaseline,
			"changeCount", status.ChangeCount,
			"diffUnavailable", status.DiffUnavailable,
			"latestEditor", status.LatestEditor,
		)
		state := "sml_source_changed"
		if status.DeletedAfterBaseline {
			state = "sml_source_missing"
		}
		return &smlSourceStateError{
			State:   state,
			Message: smlERPLogChangeSummary(status),
			Diff:    smlERPLogDiffs(status.Changes),
		}
	}

	// Nothing meaningful changed. Move the baseline forward so a harmless
	// re-save is not re-decoded on every later signature step.
	if status.CurrentRoworder > baseline {
		if err := s.store.AdvanceSMLSourceBaseline(ctx, document.ID, baseline, status.CurrentRoworder); err != nil {
			// Purely an optimization; the next check still reaches the same
			// verdict from the older baseline.
			s.logger.Warn("advance SML audit baseline failed",
				"error", err, "documentID", document.ID)
		}
		return nil
	}
	return s.store.RecordSMLSourceBaseline(ctx, document.ID, baseline)
}

// getSigningDocumentSMLEditHistory serves the change list behind a
// "document was edited" block, so a user can see exactly who changed what
// before deciding to cancel and re-import. The blocking message alone could
// not answer "แก้ตรงไหน", which is why users stopped trusting it.
func (s *Server) getSigningDocumentSMLEditHistory(w http.ResponseWriter, r *http.Request) {
	documentID := r.PathValue("id")
	document, err := s.store.FindSigningDocumentByID(r.Context(), documentID)
	if err != nil {
		if errors.Is(err, store.ErrSigningDocumentNotFound) {
			writeError(w, http.StatusNotFound, "signing_document_not_found", "ไม่พบเอกสารนี้")
			return
		}
		s.logger.Error("load signing document for SML edit history failed", "error", err, "documentID", documentID)
		writeError(w, http.StatusInternalServerError, "signing_document_load_failed", "ไม่สามารถโหลดเอกสารได้")
		return
	}
	if !requiresSMLFinalization(document) {
		writeError(w, http.StatusBadRequest, "not_sml_document", "เอกสารนี้ไม่ได้อ้างอิงจาก SML")
		return
	}

	baseline := document.SMLSourceBaselineRow
	if baseline < 0 {
		// Never measured, so there is no "since" to report against. Showing
		// the document's entire lifetime history here would imply those old
		// edits are what blocked it, which is misleading.
		baseline = 0
	}

	checkCtx := store.WithSMLTenant(r.Context(), document.SMLTenant)
	history, err := s.fetchSMLERPLogHistory(checkCtx, document.DocNo, document.TransFlag, baseline)
	if err != nil {
		s.logger.Warn("fetch SML edit history failed", "error", err, "documentID", documentID, "docNo", document.DocNo)
		writeError(w, http.StatusBadGateway, "sml_edit_history_failed", "ไม่สามารถอ่านประวัติการแก้ไขจาก SML ได้ กรุณาลองใหม่")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"docNo":     document.DocNo,
		"transFlag": document.TransFlag,
		"baseline":  baseline,
		"entries":   history.Data,
		"truncated": history.Truncated,
		"limit":     history.Limit,
	})
}

// smlERPLogChangeSummary renders the audit-trail result as the message a user
// sees, naming who changed what instead of only saying that something
// changed. That attribution is the reason for moving to the audit trail: the
// previous message could not answer "แก้ตรงไหน" and users did not believe it.
func smlERPLogChangeSummary(status smlERPLogStatus) string {
	switch {
	case status.CreatedAfterBaseline:
		return "เอกสารเลขที่นี้ถูกสร้างใหม่ใน SML หลังเริ่มงาน กรุณายกเลิกเอกสารและนำเข้า PDF ฉบับล่าสุดใหม่"
	case status.DeletedAfterBaseline:
		return "เอกสารนี้ถูกลบใน SML หลังเริ่มงาน กรุณายกเลิกเอกสารฉบับนี้"
	case status.DiffUnavailable:
		return "ข้อมูลเอกสารใน SML ถูกแก้ไขหลังเริ่มงาน กรุณายกเลิกเอกสารและนำเข้า PDF ฉบับล่าสุดใหม่"
	}

	who := status.LatestEditor
	if status.LatestComputer != "" {
		who = fmt.Sprintf("%s (%s)", who, status.LatestComputer)
	}
	if who == "" {
		who = "ผู้ใช้ใน SML"
	}
	when := status.LatestEditedAt
	if when != "" {
		when = " เมื่อ " + when
	}
	return fmt.Sprintf("ข้อมูลเอกสารใน SML ถูกแก้ไขโดย %s%s กรุณายกเลิกเอกสารและนำเข้า PDF ฉบับล่าสุดใหม่", who, when)
}

// smlERPLogDiffs converts audit-trail changes into the existing diff shape the
// API and frontend already render, so switching the verification source does
// not change the response contract.
func smlERPLogDiffs(changes []smlERPFieldChange) []smlSourceFieldDiff {
	if len(changes) == 0 {
		return nil
	}
	diffs := make([]smlSourceFieldDiff, 0, len(changes))
	for _, change := range changes {
		label := change.Label
		if label == "" {
			label = change.Field
		}
		// Detail-row changes are ambiguous without naming the line they
		// belong to — "จำนวน 16 -> 12" is unreadable when a document has
		// twenty lines.
		if change.RowLabel != "" {
			label = fmt.Sprintf("%s (%s)", label, change.RowLabel)
		} else if change.RowKey != "" {
			label = fmt.Sprintf("%s (%s)", label, change.RowKey)
		}
		diffs = append(diffs, smlSourceFieldDiff{
			Field:   label,
			Stored:  change.Old,
			Current: change.New,
		})
	}
	return diffs
}
