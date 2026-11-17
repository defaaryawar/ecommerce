package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/defaaryawar/ecommerce/pkg/auth"
	"github.com/defaaryawar/ecommerce/services/identity/internal/domain"
)

// ---------- Repository palsu (di memori) ----------

type fakeRepo struct {
	seq         int
	users       map[string]domain.User         // key: email
	tenants     map[string]domain.Tenant       // key: slug
	memberships map[string][]domain.Membership // key: user id
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		users:       map[string]domain.User{},
		tenants:     map[string]domain.Tenant{},
		memberships: map[string][]domain.Membership{},
	}
}

func (f *fakeRepo) CreateOwnerWithTenant(_ context.Context, u domain.User, t domain.Tenant) (domain.User, domain.Tenant, error) {
	if _, ok := f.tenants[t.Slug]; ok {
		return domain.User{}, domain.Tenant{}, domain.ErrSlugTaken
	}
	if _, ok := f.users[u.Email]; ok {
		return domain.User{}, domain.Tenant{}, domain.ErrEmailTaken
	}
	f.seq++
	t.ID = "tenant-" + strconv.Itoa(f.seq)
	t.Status = "active"
	u.ID = "user-" + strconv.Itoa(f.seq)
	f.tenants[t.Slug] = t
	f.users[u.Email] = u
	f.memberships[u.ID] = append(f.memberships[u.ID], domain.Membership{
		TenantID: t.ID, TenantSlug: t.Slug, TenantName: t.Name, Role: domain.RoleOwner,
	})
	return u, t, nil
}

func (f *fakeRepo) FindUserByEmail(_ context.Context, email string) (domain.User, error) {
	u, ok := f.users[email]
	if !ok {
		return domain.User{}, domain.ErrNotFound
	}
	return u, nil
}

func (f *fakeRepo) ListActiveMemberships(_ context.Context, userID string) ([]domain.Membership, error) {
	return append([]domain.Membership(nil), f.memberships[userID]...), nil
}

// ---------- Helper ----------

func newTestService(t *testing.T) (*Service, *fakeRepo, *auth.Issuer) {
	t.Helper()
	issuer, err := auth.NewIssuer("ecommerce-identity", time.Minute, map[auth.Audience][]byte{
		auth.AudienceMerchant: []byte(strings.Repeat("m", 32)),
		auth.AudiencePlatform: []byte(strings.Repeat("p", 32)),
	})
	if err != nil {
		t.Fatal(err)
	}
	repo := newFakeRepo()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewService(repo, issuer, log), repo, issuer
}

func validInput() RegisterOwnerInput {
	return RegisterOwnerInput{
		Email:     "budi@example.com",
		Password:  "rahasia123",
		FullName:  "Budi",
		StoreName: "Toko Budi",
		StoreSlug: "toko-budi",
	}
}

// ---------- RegisterOwner ----------

func TestRegisterOwner_Sukses(t *testing.T) {
	svc, repo, issuer := newTestService(t)

	out, err := svc.RegisterOwner(context.Background(), validInput())
	if err != nil {
		t.Fatalf("harusnya sukses: %v", err)
	}

	claims, err := issuer.Verify(auth.AudienceMerchant, out.AccessToken)
	if err != nil {
		t.Fatalf("token harus valid: %v", err)
	}
	if claims.TenantID != out.TenantID || claims.Role != "owner" || claims.Subject != out.UserID {
		t.Fatalf("isi token salah: %+v", claims)
	}

	stored := repo.users["budi@example.com"]
	if stored.PasswordHash == "" || stored.PasswordHash == "rahasia123" {
		t.Fatal("password harus disimpan dalam bentuk hash, bukan teks asli")
	}
}

func TestRegisterOwner_EmailDinormalisasi(t *testing.T) {
	svc, repo, _ := newTestService(t)
	in := validInput()
	in.Email = "  Budi@Example.COM "

	if _, err := svc.RegisterOwner(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if _, ok := repo.users["budi@example.com"]; !ok {
		t.Fatal("email harus disimpan dalam huruf kecil dan tanpa spasi")
	}
}

func TestRegisterOwner_InputTidakValid(t *testing.T) {
	cases := []struct {
		nama  string
		ubah  func(*RegisterOwnerInput)
		field string
	}{
		{"email jelek", func(i *RegisterOwnerInput) { i.Email = "bukan-email" }, "email"},
		{"password pendek", func(i *RegisterOwnerInput) { i.Password = "abc" }, "password"},
		{"password kepanjangan", func(i *RegisterOwnerInput) { i.Password = strings.Repeat("a", 73) }, "password"},
		{"nama kosong", func(i *RegisterOwnerInput) { i.FullName = "   " }, "full_name"},
		{"nama toko kosong", func(i *RegisterOwnerInput) { i.StoreName = "" }, "store_name"},
		{"slug ada spasi", func(i *RegisterOwnerInput) { i.StoreSlug = "toko budi" }, "store_slug"},
		{"slug diawali strip", func(i *RegisterOwnerInput) { i.StoreSlug = "-toko" }, "store_slug"},
		{"slug terlalu pendek", func(i *RegisterOwnerInput) { i.StoreSlug = "ab" }, "store_slug"},
		{"slug milik sistem", func(i *RegisterOwnerInput) { i.StoreSlug = "admin" }, "store_slug"},
	}

	for _, c := range cases {
		t.Run(c.nama, func(t *testing.T) {
			svc, repo, _ := newTestService(t)
			in := validInput()
			c.ubah(&in)

			_, err := svc.RegisterOwner(context.Background(), in)

			var ve *domain.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("harusnya ValidationError, dapat: %v", err)
			}
			if ve.Field != c.field {
				t.Fatalf("field salah: dapat %q, harusnya %q", ve.Field, c.field)
			}
			if len(repo.users) != 0 || len(repo.tenants) != 0 {
				t.Fatal("input tidak valid tidak boleh sampai menyimpan data")
			}
		})
	}
}

func TestRegisterOwner_EmailSudahDipakai(t *testing.T) {
	svc, _, _ := newTestService(t)
	if _, err := svc.RegisterOwner(context.Background(), validInput()); err != nil {
		t.Fatal(err)
	}

	in := validInput()
	in.StoreSlug = "toko-lain" // slug beda, email sama
	_, err := svc.RegisterOwner(context.Background(), in)
	if !errors.Is(err, domain.ErrEmailTaken) {
		t.Fatalf("harusnya ErrEmailTaken, dapat: %v", err)
	}
}

func TestRegisterOwner_SlugSudahDipakai(t *testing.T) {
	svc, _, _ := newTestService(t)
	if _, err := svc.RegisterOwner(context.Background(), validInput()); err != nil {
		t.Fatal(err)
	}

	in := validInput()
	in.Email = "siti@example.com" // email beda, slug sama
	_, err := svc.RegisterOwner(context.Background(), in)
	if !errors.Is(err, domain.ErrSlugTaken) {
		t.Fatalf("harusnya ErrSlugTaken, dapat: %v", err)
	}
}

// ---------- Login ----------

func TestLogin_Sukses_SatuToko(t *testing.T) {
	svc, _, issuer := newTestService(t)
	reg, err := svc.RegisterOwner(context.Background(), validInput())
	if err != nil {
		t.Fatal(err)
	}

	// Email beda huruf besar/kecil tetap harus bisa login.
	out, err := svc.Login(context.Background(), LoginInput{Email: "BUDI@example.com", Password: "rahasia123"})
	if err != nil {
		t.Fatalf("login harusnya sukses: %v", err)
	}
	if out.AccessToken == "" || out.TenantID != reg.TenantID {
		t.Fatalf("output salah: %+v", out)
	}
	claims, err := issuer.Verify(auth.AudienceMerchant, out.AccessToken)
	if err != nil || claims.TenantID != reg.TenantID || claims.Role != "owner" {
		t.Fatalf("token salah: %+v, err: %v", claims, err)
	}
}

func TestLogin_PasswordSalah(t *testing.T) {
	svc, _, _ := newTestService(t)
	if _, err := svc.RegisterOwner(context.Background(), validInput()); err != nil {
		t.Fatal(err)
	}

	_, err := svc.Login(context.Background(), LoginInput{Email: "budi@example.com", Password: "salah-total"})
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("harusnya ErrInvalidCredentials, dapat: %v", err)
	}
}

func TestLogin_EmailTidakAda_ErrorSamaDenganPasswordSalah(t *testing.T) {
	svc, _, _ := newTestService(t)

	_, err := svc.Login(context.Background(), LoginInput{Email: "hantu@example.com", Password: "apa-saja-123"})
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("harus ErrInvalidCredentials (bukan 'email tidak ditemukan'), dapat: %v", err)
	}
}

func TestLogin_BanyakToko_HarusMemilihDulu(t *testing.T) {
	svc, repo, _ := newTestService(t)
	reg, _ := svc.RegisterOwner(context.Background(), validInput())

	// Budi juga jadi admin di toko kedua.
	repo.memberships[reg.UserID] = append(repo.memberships[reg.UserID], domain.Membership{
		TenantID: "tenant-b", TenantSlug: "toko-b", TenantName: "Toko B", Role: domain.RoleAdmin,
	})

	out, err := svc.Login(context.Background(), LoginInput{Email: "budi@example.com", Password: "rahasia123"})
	if err != nil {
		t.Fatal(err)
	}
	if out.AccessToken != "" {
		t.Fatal("punya 2 toko dan belum memilih: token belum boleh diterbitkan")
	}
	if len(out.Memberships) != 2 {
		t.Fatalf("harusnya 2 pilihan toko, dapat %d", len(out.Memberships))
	}
}

func TestLogin_MemilihTokoLewatSlug(t *testing.T) {
	svc, repo, issuer := newTestService(t)
	reg, _ := svc.RegisterOwner(context.Background(), validInput())
	repo.memberships[reg.UserID] = append(repo.memberships[reg.UserID], domain.Membership{
		TenantID: "tenant-b", TenantSlug: "toko-b", TenantName: "Toko B", Role: domain.RoleAdmin,
	})

	out, err := svc.Login(context.Background(), LoginInput{
		Email: "budi@example.com", Password: "rahasia123", TenantSlug: "toko-b",
	})
	if err != nil {
		t.Fatal(err)
	}
	claims, err := issuer.Verify(auth.AudienceMerchant, out.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if claims.TenantID != "tenant-b" || claims.Role != "admin" {
		t.Fatalf("token harus untuk toko B sebagai admin, dapat: %+v", claims)
	}
}

func TestLogin_BukanAnggotaTokoItu(t *testing.T) {
	svc, _, _ := newTestService(t)
	if _, err := svc.RegisterOwner(context.Background(), validInput()); err != nil {
		t.Fatal(err)
	}

	_, err := svc.Login(context.Background(), LoginInput{
		Email: "budi@example.com", Password: "rahasia123", TenantSlug: "toko-orang-lain",
	})
	if !errors.Is(err, domain.ErrNotMember) {
		t.Fatalf("harusnya ErrNotMember, dapat: %v", err)
	}
}

func TestLogin_TidakPunyaToko(t *testing.T) {
	svc, repo, _ := newTestService(t)
	reg, _ := svc.RegisterOwner(context.Background(), validInput())
	repo.memberships[reg.UserID] = nil // keanggotaan dicabut

	_, err := svc.Login(context.Background(), LoginInput{Email: "budi@example.com", Password: "rahasia123"})
	if !errors.Is(err, domain.ErrNoMembership) {
		t.Fatalf("harusnya ErrNoMembership, dapat: %v", err)
	}
}
