-- +goose Up

-- ============ Realm merchant ============
-- Catatan: tabel tenants di sini sementara. Nanti dipindah ke tenant service,
-- identity cukup dengar event "tenant.registered".
create table tenants (
    id         uuid primary key default gen_random_uuid(),
    slug       text not null unique,
    name       text not null,
    status     text not null default 'active' check (status in ('active', 'suspended')),
    created_at timestamptz not null default now()
);

-- Identitas global: 1 orang = 1 akun, bisa jadi anggota banyak toko.
create table users (
    id            uuid primary key default gen_random_uuid(),
    email         text not null,
    password_hash text not null,
    full_name     text not null,
    created_at    timestamptz not null default now()
);
create unique index users_email_key on users (lower(email));

create table memberships (
    user_id    uuid not null references users (id) on delete cascade,
    tenant_id  uuid not null references tenants (id) on delete cascade,
    role       text not null check (role in ('owner', 'admin', 'staff')),
    status     text not null default 'active' check (status in ('active', 'invited', 'disabled')),
    created_at timestamptz not null default now(),
    primary key (user_id, tenant_id)
);
create index memberships_tenant_idx on memberships (tenant_id);

-- ============ Realm platform (tim internal) ============
-- Sengaja tabel terpisah dari users, supaya bug di cek role merchant
-- tidak bisa bikin staff toko jadi admin platform.
create table platform_users (
    id            uuid primary key default gen_random_uuid(),
    email         text not null,
    password_hash text not null,
    full_name     text not null,
    mfa_secret    text,
    status        text not null default 'active' check (status in ('active', 'disabled')),
    created_at    timestamptz not null default now()
);
create unique index platform_users_email_key on platform_users (lower(email));

create table platform_roles (
    platform_user_id uuid not null references platform_users (id) on delete cascade,
    role             text not null check (role in ('super_admin', 'support', 'finance', 'ops')),
    primary key (platform_user_id, role)
);

-- ============ Outbox ============
-- Event ditulis di transaksi yang sama dengan datanya.
-- Relay (nanti) yang publish ke Kafka lalu isi published_at.
create table outbox (
    id             uuid primary key default gen_random_uuid(),
    aggregate_type text not null,
    aggregate_id   text not null,
    event_type     text not null,
    tenant_id      uuid,
    payload        jsonb not null,
    created_at     timestamptz not null default now(),
    published_at   timestamptz
);
create index outbox_unpublished_idx on outbox (created_at) where published_at is null;

-- +goose Down
drop table outbox;
drop table platform_roles;
drop table platform_users;
drop table memberships;
drop table users;
drop table tenants;