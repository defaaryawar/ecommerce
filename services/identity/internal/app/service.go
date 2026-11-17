// Package app: usecase identity. Bergantung ke interface (port), bukan implementasi.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/defaaryawar/ecommerce/pkg/auth"
	"github.com/defaaryawar/ecommerce/services/identity/internal/domain"
)

// Repository: port ke penyimpanan. Implementasinya di adapter/postgres.
type Repository interface {
	// CreateOwnerWithTenant membuat tenant + user + membership owner + event outbox secara atomik.
	CreateOwnerWithTenant(ctx context.Context, u domain.User, t domain.Tenant) (domain.User, domain.Tenant, error)
	FindUserByEmail(ctx context.Context, email string) (domain.User, error)
	ListActiveMemberships(ctx context.Context, userID string) ([]domain.Membership, error)
}

// TokenIssuer: port ke penerbit JWT (dipenuhi oleh *auth.Issuer).
type TokenIssuer interface {
	Issue(aud auth.Audience, subject string, c auth.Claims) (string, time.Time, error)
}

type Service struct {
	repo   Repository
	tokens TokenIssuer
	log    *slog.Logger
}

func NewService(repo Repository, tokens TokenIssuer, log *slog.Logger) *Service {
	return &Service{repo: repo, tokens: tokens, log: log}
}

// Dipakai saat email tidak ditemukan, supaya waktu respons sama dengan password salah
// (mencegah orang menebak email mana yang terdaftar).
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("dummy-password-for-timing"), bcrypt.DefaultCost)

type RegisterOwnerInput struct {
	Email, Password, FullName, StoreName, StoreSlug string
}

type RegisterOwnerOutput struct {
	UserID, TenantID, AccessToken string
	ExpiresAt                     time.Time
}

func (s *Service) RegisterOwner(ctx context.Context, in RegisterOwnerInput) (RegisterOwnerOutput, error) {
	email := domain.NormalizeEmail(in.Email)
	if _, err := mail.ParseAddress(email); err != nil {
		return RegisterOwnerOutput{}, &domain.ValidationError{Field: "email", Msg: "format email tidak valid"}
	}
	if n := len(in.Password); n < 8 || n > 72 {
		return RegisterOwnerOutput{}, &domain.ValidationError{Field: "password", Msg: "8-72 karakter"}
	}
	fullName := strings.TrimSpace(in.FullName)
	if fullName == "" {
		return RegisterOwnerOutput{}, &domain.ValidationError{Field: "full_name", Msg: "wajib diisi"}
	}
	storeName := strings.TrimSpace(in.StoreName)
	if storeName == "" {
		return RegisterOwnerOutput{}, &domain.ValidationError{Field: "store_name", Msg: "wajib diisi"}
	}
	slug := strings.ToLower(strings.TrimSpace(in.StoreSlug))
	if err := domain.ValidateSlug(slug); err != nil {
		return RegisterOwnerOutput{}, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return RegisterOwnerOutput{}, fmt.Errorf("hash password: %w", err)
	}

	u, t, err := s.repo.CreateOwnerWithTenant(ctx,
		domain.User{Email: email, PasswordHash: string(hash), FullName: fullName},
		domain.Tenant{Slug: slug, Name: storeName},
	)
	if err != nil {
		return RegisterOwnerOutput{}, err
	}

	tok, exp, err := s.tokens.Issue(auth.AudienceMerchant, u.ID, auth.Claims{
		TenantID: t.ID,
		Role:     string(domain.RoleOwner),
	})
	if err != nil {
		return RegisterOwnerOutput{}, fmt.Errorf("issue token: %w", err)
	}

	s.log.InfoContext(ctx, "toko baru terdaftar", "tenant_id", t.ID, "slug", t.Slug, "user_id", u.ID)
	return RegisterOwnerOutput{UserID: u.ID, TenantID: t.ID, AccessToken: tok, ExpiresAt: exp}, nil
}

type LoginInput struct {
	Email, Password, TenantSlug string
}

// LoginOutput: kalau AccessToken kosong, user harus memilih toko dari Memberships.
type LoginOutput struct {
	AccessToken string
	ExpiresAt   time.Time
	TenantID    string
	Memberships []domain.Membership
}

func (s *Service) Login(ctx context.Context, in LoginInput) (LoginOutput, error) {
	u, err := s.repo.FindUserByEmail(ctx, domain.NormalizeEmail(in.Email))
	if errors.Is(err, domain.ErrNotFound) {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(in.Password))
		return LoginOutput{}, domain.ErrInvalidCredentials
	}
	if err != nil {
		return LoginOutput{}, err
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(in.Password)) != nil {
		return LoginOutput{}, domain.ErrInvalidCredentials
	}

	ms, err := s.repo.ListActiveMemberships(ctx, u.ID)
	if err != nil {
		return LoginOutput{}, err
	}
	if len(ms) == 0 {
		return LoginOutput{}, domain.ErrNoMembership
	}

	var chosen *domain.Membership
	slug := strings.ToLower(strings.TrimSpace(in.TenantSlug))
	switch {
	case slug != "":
		for i := range ms {
			if ms[i].TenantSlug == slug {
				chosen = &ms[i]
				break
			}
		}
		if chosen == nil {
			return LoginOutput{}, domain.ErrNotMember
		}
	case len(ms) == 1:
		chosen = &ms[0]
	default:
		// Punya beberapa toko: frontend tampilkan pilihan, lalu login ulang dengan tenant_slug.
		return LoginOutput{Memberships: ms}, nil
	}

	tok, exp, err := s.tokens.Issue(auth.AudienceMerchant, u.ID, auth.Claims{
		TenantID: chosen.TenantID,
		Role:     string(chosen.Role),
	})
	if err != nil {
		return LoginOutput{}, fmt.Errorf("issue token: %w", err)
	}
	return LoginOutput{AccessToken: tok, ExpiresAt: exp, TenantID: chosen.TenantID, Memberships: ms}, nil
}
