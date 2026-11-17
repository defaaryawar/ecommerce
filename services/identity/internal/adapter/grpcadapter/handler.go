// Package grpcadapter: handler Connect/gRPC. Tugasnya cuma terjemahin proto <-> usecase
// dan error domain -> kode gRPC. Logika bisnis tidak boleh ada di sini.
package grpcadapter

import (
	"context"
	"errors"
	"log/slog"

	"connectrpc.com/connect/v2"

	identityv1 "github.com/defaaryawar/ecommerce/gen/go/identity/v1"
	"github.com/defaaryawar/ecommerce/gen/go/identity/v1/identityv1connect"
	"github.com/defaaryawar/ecommerce/services/identity/internal/app"
	"github.com/defaaryawar/ecommerce/services/identity/internal/domain"
)

type Handler struct {
	svc *app.Service
	log *slog.Logger
}

// Compile-time check: kalau ada method yang kurang atau salah tipe, compile langsung gagal.
var _ identityv1connect.MerchantAuthServiceHandler = (*Handler)(nil)

func NewHandler(svc *app.Service, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

func (h *Handler) RegisterOwner(
	ctx context.Context, req *identityv1.RegisterOwnerRequest,
) (*identityv1.RegisterOwnerResponse, error) {
	out, err := h.svc.RegisterOwner(ctx, app.RegisterOwnerInput{
		Email:     req.GetEmail(),
		Password:  req.GetPassword(),
		FullName:  req.GetFullName(),
		StoreName: req.GetStoreName(),
		StoreSlug: req.GetStoreSlug(),
	})
	if err != nil {
		return nil, h.toConnectErr(ctx, err)
	}
	return &identityv1.RegisterOwnerResponse{
		UserId:        out.UserID,
		TenantId:      out.TenantID,
		AccessToken:   out.AccessToken,
		ExpiresAtUnix: out.ExpiresAt.Unix(),
	}, nil
}

func (h *Handler) Login(
	ctx context.Context, req *identityv1.LoginRequest,
) (*identityv1.LoginResponse, error) {
	out, err := h.svc.Login(ctx, app.LoginInput{
		Email:      req.GetEmail(),
		Password:   req.GetPassword(),
		TenantSlug: req.GetTenantSlug(),
	})
	if err != nil {
		return nil, h.toConnectErr(ctx, err)
	}

	res := &identityv1.LoginResponse{
		AccessToken: out.AccessToken,
		TenantId:    out.TenantID,
	}
	if !out.ExpiresAt.IsZero() {
		res.ExpiresAtUnix = out.ExpiresAt.Unix()
	}
	for _, m := range out.Memberships {
		res.Memberships = append(res.Memberships, &identityv1.Membership{
			TenantId:   m.TenantID,
			TenantSlug: m.TenantSlug,
			TenantName: m.TenantName,
			Role:       string(m.Role),
		})
	}
	return res, nil
}

// toConnectErr: error domain -> error Connect. Di v2, hanya pesan dari connect.NewError
// yang sampai ke client, jadi error domain harus diubah eksplisit.
func (h *Handler) toConnectErr(ctx context.Context, err error) error {
	var ve *domain.ValidationError
	switch {
	case errors.As(err, &ve):
		return connect.NewError(connect.CodeInvalidArgument, ve.Error())
	case errors.Is(err, domain.ErrEmailTaken), errors.Is(err, domain.ErrSlugTaken):
		return connect.NewError(connect.CodeAlreadyExists, err.Error())
	case errors.Is(err, domain.ErrInvalidCredentials):
		return connect.NewError(connect.CodeUnauthenticated, err.Error())
	case errors.Is(err, domain.ErrNoMembership), errors.Is(err, domain.ErrNotMember):
		return connect.NewError(connect.CodePermissionDenied, err.Error())
	case errors.Is(err, context.Canceled):
		return connect.NewError(connect.CodeCanceled, "request dibatalkan")
	case errors.Is(err, context.DeadlineExceeded):
		return connect.NewError(connect.CodeDeadlineExceeded, "request melewati batas waktu")
	default:
		// Pesan generik ke client. Error aslinya disimpan sebagai cause + masuk log.
		h.log.ErrorContext(ctx, "internal error", "err", err)
		return connect.NewError(connect.CodeInternal, "terjadi kesalahan internal").WithCause(err)
	}
}
