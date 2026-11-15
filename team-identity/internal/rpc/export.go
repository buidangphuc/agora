package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/store"
)

const (
	exportMaxBytes      = 1 << 20
	exportSchemaVersion = 1
	exportContentType   = "application/json; charset=utf-8"
)

type ExportRepository interface {
	ExportUserData(ctx context.Context, userID string) (store.UserExport, error)
}

func (h *PrivacyHandler) WithExport(repo ExportRepository) *PrivacyHandler {
	h.export = repo
	return h
}

type exportAccountDoc struct {
	UserID                string   `json:"user_id"`
	Username              string   `json:"username"`
	Roles                 []string `json:"roles"`
	Status                string   `json:"status"`
	CreatedAt             string   `json:"created_at"`
	SellerTermsVersion    string   `json:"seller_terms_version"`
	SellerTermsAcceptedAt *string  `json:"seller_terms_accepted_at"`
}

type exportAddressDoc struct {
	ID            string `json:"id"`
	RecipientName string `json:"recipient_name"`
	Phone         string `json:"phone"`
	Street        string `json:"street"`
	Ward          string `json:"ward"`
	District      string `json:"district"`
	City          string `json:"city"`
	IsDefault     bool   `json:"is_default"`
}

type exportSessionDoc struct {
	ID        string `json:"id"`
	Device    string `json:"device"`
	IP        string `json:"ip"`
	CreatedAt string `json:"created_at"`
	LastSeen  string `json:"last_seen"`
	Revoked   bool   `json:"revoked"`
}

type exportLoginDoc struct {
	ID        string `json:"id"`
	IP        string `json:"ip"`
	UserAgent string `json:"user_agent"`
	Success   bool   `json:"success"`
	CreatedAt string `json:"created_at"`
}

type exportConsentDoc struct {
	Purpose    string `json:"purpose"`
	Granted    bool   `json:"granted"`
	Version    string `json:"version"`
	Source     string `json:"source"`
	RecordedAt string `json:"recorded_at"`
}

type exportDeletionDoc struct {
	Pending    bool    `json:"pending"`
	EraseAfter *string `json:"erase_after"`
}

type exportTruncatedDoc struct {
	LoginHistory bool `json:"login_history"`
	Sessions     bool `json:"sessions"`
}

type exportDoc struct {
	SchemaVersion int                `json:"schema_version"`
	GeneratedAt   string             `json:"generated_at"`
	Account       exportAccountDoc   `json:"account"`
	Addresses     []exportAddressDoc `json:"addresses"`
	Sessions      []exportSessionDoc `json:"sessions"`
	LoginHistory  []exportLoginDoc   `json:"login_history"`
	Consents      []exportConsentDoc `json:"consents"`
	Deletion      exportDeletionDoc  `json:"deletion"`
	Truncated     exportTruncatedDoc `json:"truncated"`
}

func exportTime(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func exportTimePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := exportTime(*t)
	return &s
}

func buildExportDoc(e store.UserExport, now time.Time) exportDoc {
	roles := e.Account.Roles
	if roles == nil {
		roles = []string{}
	}
	d := exportDoc{
		SchemaVersion: exportSchemaVersion,
		GeneratedAt:   exportTime(now),
		Account: exportAccountDoc{
			UserID: e.Account.UserID, Username: e.Account.Username, Roles: roles, Status: e.Account.Status,
			CreatedAt: exportTime(e.Account.CreatedAt), SellerTermsVersion: e.Account.SellerTermsVersion,
			SellerTermsAcceptedAt: exportTimePtr(e.Account.SellerTermsAcceptedAt),
		},
		Addresses:    make([]exportAddressDoc, 0, len(e.Addresses)),
		Sessions:     make([]exportSessionDoc, 0, len(e.Sessions)),
		LoginHistory: make([]exportLoginDoc, 0, len(e.LoginHistory)),
		Consents:     make([]exportConsentDoc, 0, len(e.Consents)),
		Deletion:     exportDeletionDoc{Pending: e.Account.DeletionRequestedAt != nil, EraseAfter: exportTimePtr(e.Account.EraseAfter)},
		Truncated:    exportTruncatedDoc{LoginHistory: e.LoginHistoryTruncated, Sessions: e.SessionsTruncated},
	}
	for _, a := range e.Addresses {
		d.Addresses = append(d.Addresses, exportAddressDoc{ID: a.ID, RecipientName: a.RecipientName, Phone: a.Phone, Street: a.Street, Ward: a.Ward, District: a.District, City: a.City, IsDefault: a.IsDefault})
	}
	for _, s := range e.Sessions {
		d.Sessions = append(d.Sessions, exportSessionDoc{ID: s.ID, Device: s.Device, IP: s.IP, CreatedAt: exportTime(s.CreatedAt), LastSeen: exportTime(s.LastSeen), Revoked: s.Revoked})
	}
	for _, l := range e.LoginHistory {
		d.LoginHistory = append(d.LoginHistory, exportLoginDoc{ID: l.ID, IP: l.IP, UserAgent: l.UserAgent, Success: l.Success, CreatedAt: exportTime(l.CreatedAt)})
	}
	for _, c := range e.Consents {
		d.Consents = append(d.Consents, exportConsentDoc{Purpose: c.Purpose, Granted: c.Granted, Version: c.Version, Source: c.Source, RecordedAt: exportTime(c.RecordedAt)})
	}
	return d
}

func (h *PrivacyHandler) ExportMyData(ctx context.Context, _ *identityv1.ExportMyDataRequest) (*identityv1.ExportMyDataResponse, error) {
	p, err := auth.RequireUser(ctx)
	if err != nil {
		return nil, err
	}
	if h.export == nil {
		return nil, status.Error(codes.Unimplemented, "data export not enabled")
	}
	e, err := h.export.ExportUserData(ctx, p.ID)
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) {
			h.logger.WarnContext(ctx, "export data unavailable", "op", "export my data", "error", err)
			return nil, status.Error(codes.Unavailable, "service unavailable")
		}
		h.logger.ErrorContext(ctx, "export data failed", "op", "export my data", "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}
	now := time.Now().UTC()
	doc := buildExportDoc(e, now)
	data, err := json.Marshal(doc)
	for err == nil && len(data) > exportMaxBytes {
		switch {
		case len(doc.LoginHistory) > 0:
			doc.LoginHistory = doc.LoginHistory[:len(doc.LoginHistory)/2]
			doc.Truncated.LoginHistory = true
		case len(doc.Sessions) > 0:
			doc.Sessions = doc.Sessions[:len(doc.Sessions)/2]
			doc.Truncated.Sessions = true
		default:
			h.logger.ErrorContext(ctx, "export data too large", "op", "export my data", "bytes", len(data))
			return nil, status.Error(codes.Internal, "internal error")
		}
		data, err = json.Marshal(doc)
	}
	if err != nil {
		h.logger.ErrorContext(ctx, "export data encode failed", "op", "export my data", "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}
	h.logger.InfoContext(ctx, "data exported", "op", "export my data",
		"addresses", len(doc.Addresses), "sessions", len(doc.Sessions), "login_history", len(doc.LoginHistory),
		"consents", len(doc.Consents), "bytes", len(data))
	return &identityv1.ExportMyDataResponse{Data: data, ContentType: exportContentType, GeneratedAt: timestamppb.New(now)}, nil
}
