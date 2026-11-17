// Package domain: entity dan aturan bisnis identity. Tidak boleh import DB, gRPC, dll.
package domain

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

type Role string

const (
	RoleOwner Role = "owner"
	RoleAdmin Role = "admin"
	RoleStaff Role = "staff"
)

type Tenant struct {
	ID     string
	Slug   string
	Name   string
	Status string
}

type User struct {
	ID           string
	Email        string
	PasswordHash string
	FullName     string
}

type Membership struct {
	TenantID   string
	TenantSlug string
	TenantName string
	Role       Role
}

var (
	ErrNotFound           = errors.New("data tidak ditemukan")
	ErrEmailTaken         = errors.New("email sudah terdaftar")
	ErrSlugTaken          = errors.New("slug toko sudah dipakai")
	ErrInvalidCredentials = errors.New("email atau password salah")
	ErrNoMembership       = errors.New("akun ini belum terdaftar di toko mana pun")
	ErrNotMember          = errors.New("akun ini bukan anggota toko tersebut")
)

// ValidationError: input tidak valid (nanti dipetakan ke InvalidArgument).
type ValidationError struct {
	Field string
	Msg   string
}

func (e *ValidationError) Error() string { return fmt.Sprintf("%s: %s", e.Field, e.Msg) }

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,38}[a-z0-9]$`)

// Slug yang tidak boleh dipakai toko karena bentrok dengan subdomain sistem.
var reservedSlugs = map[string]bool{
	"www": true, "api": true, "admin": true, "console": true, "app": true,
	"mail": true, "static": true, "cdn": true, "auth": true, "status": true,
}

// ValidateSlug: 3-40 karakter, huruf kecil/angka/strip, bukan slug sistem.
func ValidateSlug(s string) error {
	if !slugRe.MatchString(s) {
		return &ValidationError{Field: "store_slug", Msg: "3-40 karakter, huruf kecil, angka, atau strip"}
	}
	if reservedSlugs[s] {
		return &ValidationError{Field: "store_slug", Msg: "slug ini dipakai sistem"}
	}
	return nil
}

// NormalizeEmail: trim + lowercase.
func NormalizeEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }
