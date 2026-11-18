-- +goose Up
alter table outbox add column topic text;
update outbox set topic = 'identity.tenant.events.v1' where topic is null;
alter table outbox alter column topic set not null;

-- +goose Down
alter table outbox drop column topic;