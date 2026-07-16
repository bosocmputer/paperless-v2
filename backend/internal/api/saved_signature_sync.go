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
	"strings"

	"github.com/bosocmputer/paperless-v2/backend/internal/models"
	"github.com/bosocmputer/paperless-v2/backend/internal/store"
)

type smlSavedSignatureSyncResult struct {
	Available int
	New       int
	Changed   int
	Unchanged int
	Missing   int
	Invalid   int
	Synced    int
	Failed    int
	Items     []models.SMLSignatureSyncItem
}

type smlSavedSignatureRequest struct {
	Provider        string `json:"provider"`
	DataGroup       string `json:"dataGroup"`
	DatabaseName    string `json:"databaseName"`
	UserCode        string `json:"userCode"`
	ExpectedVersion string `json:"expectedVersion"`
}

type plannedSMLSignature struct {
	Candidate models.SMLUserSyncCandidate
	Status    string
	Previous  bool
}

func (s *Server) syncSMLSavedSignatures(ctx context.Context, candidates []models.SMLUserSyncCandidate, session models.AuthSession, tenant, actorID string, dryRun bool) (smlSavedSignatureSyncResult, error) {
	tenant = store.NormalizeSMLTenant(tenant)
	usernames := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		usernames = append(usernames, candidate.Username)
	}
	existing, err := s.store.ListSavedSignaturesByUsernames(ctx, tenant, usernames)
	if err != nil {
		return smlSavedSignatureSyncResult{}, err
	}
	plans, result := planSMLSavedSignatures(candidates, existing)
	if dryRun {
		return result, nil
	}

	users, err := s.store.FindUsersByUsernames(ctx, usernames)
	if err != nil {
		return smlSavedSignatureSyncResult{}, err
	}
	for index := range plans {
		plan := &plans[index]
		key := strings.ToLower(strings.TrimSpace(plan.Candidate.Username))
		user, userExists := users[key]
		item := models.SMLSignatureSyncItem{
			Username:       plan.Candidate.Username,
			DisplayName:    plan.Candidate.DisplayName,
			Status:         plan.Status,
			PreviousExists: plan.Previous,
		}
		if !userExists {
			item.Status = "failed"
			item.Issue = "paperless_user_missing"
			result.Failed++
			result.Items[index] = item
			continue
		}

		switch plan.Status {
		case "missing", "invalid":
			issue := firstNonEmpty(plan.Candidate.SignatureIssue, "signature_"+plan.Status)
			if err := s.store.RecordUserSavedSignatureError(ctx, user.ID, tenant, plan.Candidate.Username, issue); err != nil {
				item.Status = "failed"
				item.Issue = "signature_state_update_failed"
				result.Failed++
			}
			result.Items[index] = item
			continue
		case "unchanged":
			result.Items[index] = item
			continue
		}

		data, version, err := s.fetchSMLSavedSignature(ctx, session, plan.Candidate)
		if err != nil {
			item.Status = "failed"
			item.Issue = savedSignatureSyncErrorCode(err)
			result.Failed++
			_ = s.store.RecordUserSavedSignatureError(ctx, user.ID, tenant, plan.Candidate.Username, item.Issue)
			result.Items[index] = item
			continue
		}
		normalized, err := normalizeSavedSignatureImage(data)
		if err != nil {
			item.Status = "failed"
			item.Issue = "signature_normalize_failed"
			result.Failed++
			_ = s.store.RecordUserSavedSignatureError(ctx, user.ID, tenant, plan.Candidate.Username, item.Issue)
			result.Items[index] = item
			continue
		}
		uploaded, err := s.storeUploadedBytes(ctx, normalized, "sml-signature-"+plan.Candidate.Username+".png", "sml-signature.png", "image/png", ".png", 0, actorID)
		if err != nil {
			item.Status = "failed"
			item.Issue = "signature_store_failed"
			result.Failed++
			_ = s.store.RecordUserSavedSignatureError(ctx, user.ID, tenant, plan.Candidate.Username, item.Issue)
			result.Items[index] = item
			continue
		}
		oldFileID, err := s.store.UpsertUserSavedSignature(ctx, store.SavedSignatureUpsertInput{
			UserID:        user.ID,
			SMLTenant:     tenant,
			FileID:        uploaded.ID,
			SMLUserCode:   plan.Candidate.Username,
			SourceVersion: version,
		})
		if err != nil {
			s.cleanupUploadedFileBestEffort(uploaded, "saved_signature_upsert_failed")
			item.Status = "failed"
			item.Issue = "signature_pointer_update_failed"
			result.Failed++
			result.Items[index] = item
			continue
		}
		if oldFileID != "" && oldFileID != uploaded.ID {
			s.cleanupUploadedFileBestEffort(models.UploadedFile{ID: oldFileID}, "saved_signature_replaced")
		}
		item.Status = "synced"
		result.Synced++
		result.Items[index] = item
	}
	return result, nil
}

func planSMLSavedSignatures(candidates []models.SMLUserSyncCandidate, existing map[string]models.UserSavedSignature) ([]plannedSMLSignature, smlSavedSignatureSyncResult) {
	plans := make([]plannedSMLSignature, 0, len(candidates))
	result := smlSavedSignatureSyncResult{Items: make([]models.SMLSignatureSyncItem, 0, len(candidates))}
	for _, candidate := range candidates {
		current, hasCurrent := existing[strings.ToLower(strings.TrimSpace(candidate.Username))]
		previous := hasCurrent && current.FileID != "" && current.File.ID != ""
		status := "missing"
		switch {
		case candidate.SignatureAvailable && strings.TrimSpace(candidate.SignatureVersion) != "":
			result.Available++
			switch {
			case !previous:
				status = "new"
				result.New++
			case current.SourceVersion == candidate.SignatureVersion:
				status = "unchanged"
				result.Unchanged++
			default:
				status = "changed"
				result.Changed++
			}
		case strings.EqualFold(candidate.SignatureIssue, "signature_missing") || strings.TrimSpace(candidate.SignatureIssue) == "":
			status = "missing"
			result.Missing++
		default:
			status = "invalid"
			result.Invalid++
		}
		plans = append(plans, plannedSMLSignature{Candidate: candidate, Status: status, Previous: previous})
		result.Items = append(result.Items, models.SMLSignatureSyncItem{
			Username:       candidate.Username,
			DisplayName:    candidate.DisplayName,
			Status:         status,
			Issue:          candidate.SignatureIssue,
			PreviousExists: previous,
		})
	}
	return plans, result
}

func (s *Server) fetchSMLSavedSignature(ctx context.Context, session models.AuthSession, candidate models.SMLUserSyncCandidate) ([]byte, string, error) {
	endpoint, err := url.Parse(s.cfg.SMLPaperlessBaseURL + "/api/v1/auth/sml/users/signature")
	if err != nil {
		return nil, "", fmt.Errorf("invalid_sml_endpoint")
	}
	databaseName := firstNonEmpty(session.SMLDataCode, session.SMLTenant)
	body, err := json.Marshal(smlSavedSignatureRequest{
		Provider:        firstNonEmpty(session.SMLProvider, s.cfg.SMLAuthProvider),
		DataGroup:       firstNonEmpty(session.SMLDataGroup, s.cfg.SMLAuthDataGroup),
		DatabaseName:    databaseName,
		UserCode:        candidate.Username,
		ExpectedVersion: candidate.SignatureVersion,
	})
	if err != nil {
		return nil, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Accept", "image/png, image/jpeg")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", s.cfg.SMLPaperlessAPIKey)
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		var payload struct {
			Error   *smlAPIError `json:"error"`
			Message string       `json:"message"`
		}
		_ = json.NewDecoder(io.LimitReader(resp.Body, 256<<10)).Decode(&payload)
		return nil, "", newSMLRequestError(payload.Error, payload.Message, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxSavedSignatureSourceBytes+1))
	if err != nil || len(data) == 0 || len(data) > maxSavedSignatureSourceBytes {
		return nil, "", fmt.Errorf("signature_payload_invalid")
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.SplitN(resp.Header.Get("Content-Type"), ";", 2)[0]))
	if contentType != "image/png" && contentType != "image/jpeg" {
		return nil, "", fmt.Errorf("signature_content_type_invalid")
	}
	version := strings.TrimSpace(resp.Header.Get("X-Signature-Version"))
	if version == "" || version != candidate.SignatureVersion {
		return nil, "", fmt.Errorf("signature_version_changed")
	}
	return data, version, nil
}

func savedSignatureSyncErrorCode(err error) string {
	if err == nil {
		return ""
	}
	var smlErr *smlRequestError
	if ok := errors.As(err, &smlErr); ok && strings.TrimSpace(smlErr.Code) != "" {
		return truncateForMetadata(smlErr.Code, 80)
	}
	text := strings.ToLower(err.Error())
	if strings.Contains(text, "version") {
		return "signature_version_changed"
	}
	return "signature_fetch_failed"
}
