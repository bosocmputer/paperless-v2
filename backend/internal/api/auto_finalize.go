package api

import (
	"context"
	"time"
)

const (
	autoFinalizeSweepInterval = time.Minute
	autoFinalizeStaleAfter    = 5 * time.Minute
)

func (s *Server) StartAutoFinalizeSweeper(ctx context.Context) {
	go func() {
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				s.runAutoFinalizeSweep(ctx)
				timer.Reset(autoFinalizeSweepInterval)
			}
		}
	}()
}

func (s *Server) enqueueAutoFinalize(documentID, ipAddress, userAgent string) {
	if documentID == "" {
		return
	}
	go s.autoFinalizeDocument(context.Background(), documentID, ipAddress, userAgent)
}

func (s *Server) runAutoFinalizeSweep(ctx context.Context) {
	listCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	ids, err := s.store.ListSigningDocumentsForAutoFinalize(listCtx, 10, time.Now().Add(-autoFinalizeStaleAfter))
	if err != nil {
		s.logger.Warn("auto finalize sweep list failed", "error", err)
		return
	}
	for _, id := range ids {
		s.autoFinalizeDocument(ctx, id, "", "")
	}
}

func (s *Server) autoFinalizeDocument(parent context.Context, documentID, ipAddress, userAgent string) {
	claimCtx, claimCancel := context.WithTimeout(parent, 10*time.Second)
	claimed, err := s.store.ClaimSigningDocumentAutoFinalize(claimCtx, documentID, ipAddress, userAgent, time.Now().Add(-autoFinalizeStaleAfter))
	claimCancel()
	if err != nil {
		s.logger.Warn("auto finalize claim failed", "error", err, "documentID", documentID)
		return
	}
	if !claimed {
		return
	}

	timeout := 3 * s.cfg.SMLPaperlessTimeout
	if timeout < 2*time.Minute {
		timeout = 2 * time.Minute
	}
	finalizeCtx, finalizeCancel := context.WithTimeout(context.Background(), timeout)
	result := s.finalizeCompletedDocument(finalizeCtx, documentID, ipAddress, userAgent)
	finalizeCancel()
	if result.LockOK {
		_ = s.store.AddSigningEvent(context.Background(), documentID, "", "", "document_auto_confirmed", "ระบบส่งเอกสารเข้า SML เรียบร้อยแล้ว", ipAddress, userAgent, nil)
	}
}
