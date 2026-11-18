// Package postgres: implementasi app.Repository pakai pgx.
package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/defaaryawar/ecommerce/services/identity/internal/domain"
)

type Repo struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

func (r *Repo) CreateOwnerWithTenant(ctx context.Context, u domain.User, t domain.Tenant) (domain.User, domain.Tenant, error) {
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx,
			`insert into tenants (slug, name) values ($1, $2) returning id, status`,
			t.Slug, t.Name,
		).Scan(&t.ID, &t.Status); err != nil {
			return mapUniqueErr(err)
		}

		if err := tx.QueryRow(ctx,
			`insert into users (email, password_hash, full_name) values ($1, $2, $3) returning id`,
			u.Email, u.PasswordHash, u.FullName,
		).Scan(&u.ID); err != nil {
			return mapUniqueErr(err)
		}

		if _, err := tx.Exec(ctx,
			`insert into memberships (user_id, tenant_id, role) values ($1, $2, $3)`,
			u.ID, t.ID, string(domain.RoleOwner),
		); err != nil {
			return err
		}

		// Outbox: event masuk di transaksi yang sama. Kalau commit gagal, event ikut batal.
		payload, err := json.Marshal(map[string]string{
			"tenant_id":     t.ID,
			"slug":          t.Slug,
			"name":          t.Name,
			"owner_user_id": u.ID,
		})
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx,
			`insert into outbox (aggregate_type, aggregate_id, event_type, topic, tenant_id, payload)
			 values ('tenant', $1, 'identity.tenant.registered.v1', 'identity.tenant.events.v1', $2, $3)`,
			t.ID, t.ID, string(payload),
		)
		return err
	})
	if err != nil {
		return domain.User{}, domain.Tenant{}, err
	}
	return u, t, nil
}

func (r *Repo) FindUserByEmail(ctx context.Context, email string) (domain.User, error) {
	var u domain.User
	err := r.pool.QueryRow(ctx,
		`select id, email, password_hash, full_name from users where lower(email) = lower($1)`,
		email,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.FullName)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrNotFound
	}
	return u, err
}

func (r *Repo) ListActiveMemberships(ctx context.Context, userID string) ([]domain.Membership, error) {
	rows, err := r.pool.Query(ctx, `
		select t.id, t.slug, t.name, m.role
		from memberships m
		join tenants t on t.id = m.tenant_id
		where m.user_id = $1 and m.status = 'active' and t.status = 'active'
		order by t.name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.Membership
	for rows.Next() {
		var m domain.Membership
		var role string
		if err := rows.Scan(&m.TenantID, &m.TenantSlug, &m.TenantName, &role); err != nil {
			return nil, err
		}
		m.Role = domain.Role(role)
		out = append(out, m)
	}
	return out, rows.Err()
}

// mapUniqueErr: error "unique violation" (kode 23505) dari Postgres -> error domain.
func mapUniqueErr(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		switch pgErr.ConstraintName {
		case "tenants_slug_key":
			return domain.ErrSlugTaken
		case "users_email_key":
			return domain.ErrEmailTaken
		}
	}
	return err
}
