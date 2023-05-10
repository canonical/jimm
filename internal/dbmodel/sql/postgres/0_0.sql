-- 0_0.sql initialises an empty database.

CREATE TABLE IF NOT EXISTS audit_log (
	id BIGSERIAL PRIMARY KEY,
	time TIMESTAMP WITH TIME ZONE,
	model TEXT,
	conversation_id TEXT,
	message_id INTEGER,
	facade_name TEXT, 
   	facade_method TEXT,
   	facade_version INTEGER,
	object_id TEXT,
	user_tag TEXT,
	is_response BOOLEAN,
	params JSON,
	errors JSON
);
CREATE INDEX IF NOT EXISTS idx_audit_log_time ON audit_log (created_at);
CREATE INDEX IF NOT EXISTS idx_audit_log_user_tag ON audit_log (user_tag);
CREATE INDEX IF NOT EXISTS idx_audit_log_method ON audit_log (facade_method);
CREATE INDEX IF NOT EXISTS idx_audit_log_model ON audit_log (model);

CREATE TABLE IF NOT EXISTS clouds (
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMP WITH TIME ZONE,
	updated_at TIMESTAMP WITH TIME ZONE,
	name TEXT NOT NULL UNIQUE,
	type TEXT NOT NULL,
	host_cloud_region TEXT NOT NULL,
	auth_types BYTEA,
	endpoint TEXT NOT NULL,
	identity_endpoint TEXT NOT NULL,
	storage_endpoint TEXT NOT NULL,
	ca_certificates BYTEA,
	config BYTEA
);

CREATE TABLE IF NOT EXISTS cloud_regions (
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMP WITH TIME ZONE,
	updated_at TIMESTAMP WITH TIME ZONE,
	deleted_at TIMESTAMP WITH TIME ZONE,
	cloud_name TEXT NOT NULL REFERENCES clouds (name) ON DELETE CASCADE,
	name TEXT NOT NULL,
	endpoint TEXT NOT NULL,
	identity_endpoint TEXT NOT NULL,
	storage_endpoint TEXT NOT NULL,
	config BYTEA,
	UNIQUE(cloud_name, name)
);
CREATE INDEX IF NOT EXISTS idx_cloud_regions_deleted_at ON cloud_regions (deleted_at);

CREATE TABLE IF NOT EXISTS controllers (
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMP WITH TIME ZONE,
	updated_at TIMESTAMP WITH TIME ZONE,
	deleted_at TIMESTAMP WITH TIME ZONE,
	name TEXT NOT NULL UNIQUE,
	uuid TEXT NOT NULL,
	admin_user TEXT NOT NULL,
	admin_password TEXT NOT NULL,
	ca_certificate TEXT NOT NULL,
	public_address TEXT NOT NULL,
	cloud_name TEXT NOT NULL REFERENCES clouds (name),
	cloud_region TEXT NOT NULL,
	deprecated BOOLEAN NOT NULL DEFAULT false,
	agent_version TEXT NOT NULL,
	addresses BYTEA,
	unavailable_since TIMESTAMP WITH TIME ZONE
);
CREATE INDEX IF NOT EXISTS idx_controllers_deleted_at ON controllers (deleted_at);

CREATE TABLE IF NOT EXISTS cloud_region_controller_priorities (
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMP WITH TIME ZONE,
	updated_at TIMESTAMP WITH TIME ZONE,
	deleted_at TIMESTAMP WITH TIME ZONE,
	cloud_region_id INTEGER NOT NULL REFERENCES cloud_regions (id) ON DELETE CASCADE,
	controller_id INTEGER NOT NULL REFERENCES controllers (id) ON DELETE CASCADE,
	priority INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_cloud_region_controller_priorities_deleted_at ON cloud_region_controller_priorities (deleted_at);

CREATE TABLE IF NOT EXISTS users (
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMP WITH TIME ZONE,
	updated_at TIMESTAMP WITH TIME ZONE,
	deleted_at TIMESTAMP WITH TIME ZONE,
	username TEXT NOT NULL UNIQUE,
	display_name TEXT NOT NULL,
	last_login TIMESTAMP WITH TIME ZONE,
	disabled BOOLEAN,
	controller_access TEXT NOT NULL DEFAULT 'login',
	audit_log_access TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_users_deleted_at ON users (deleted_at);

CREATE TABLE IF NOT EXISTS cloud_credentials (
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMP WITH TIME ZONE,
	updated_at TIMESTAMP WITH TIME ZONE,
	deleted_at TIMESTAMP WITH TIME ZONE,
	cloud_name TEXT NOT NULL REFERENCES clouds (name),
	owner_username TEXT NOT NULL REFERENCES users (username),
	name TEXT NOT NULL,
	auth_type TEXT NOT NULL,
	label TEXT NOT NULL,
	attributes_in_vault BOOLEAN NOT NULL,
	attributes BYTEA,
	valid BOOLEAN,
	UNIQUE(cloud_name, owner_username, name)
);
CREATE INDEX IF NOT EXISTS idx_cloud_credentials_deleted_at ON cloud_credentials (deleted_at);

CREATE TABLE IF NOT EXISTS cloud_defaults (
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMP WITH TIME ZONE,
	updated_at TIMESTAMP WITH TIME ZONE,
	deleted_at TIMESTAMP WITH TIME ZONE,
	username TEXT NOT NULL REFERENCES users (username),
	cloud_id INTEGER NOT NULL REFERENCES clouds (id),
	region TEXT NOT NULL,
	defaults BYTEA,
	UNIQUE (username, cloud_id, region)
);
CREATE INDEX IF NOT EXISTS idx_cloud_defaults_deleted_at ON cloud_defaults (deleted_at);

CREATE TABLE IF NOT EXISTS models (
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMP WITH TIME ZONE,
	updated_at TIMESTAMP WITH TIME ZONE,
	name TEXT NOT NULL,
	uuid TEXT UNIQUE,
	owner_username TEXT NOT NULL REFERENCES users (username),
	controller_id INTEGER REFERENCES controllers (id),
	cloud_region_id INTEGER REFERENCES cloud_regions (id),
	cloud_credential_id BIGINT REFERENCES cloud_credentials (id),
	type TEXT NOT NULL,
	is_controller BOOLEAN NOT NULL,
	default_series TEXT NOT NULL,
	life TEXT NOT NULL,
	status_status TEXT NOT NULL,
	status_info TEXT NOT NULL,
	status_data BYTEA,
	status_since TIMESTAMP WITH TIME ZONE,
	status_version TEXT NOT NULL,
	sla_level TEXT NOT NULL,
	sla_owner TEXT NOT NULL,
	cores BIGINT NOT NULL DEFAULT 0,
	machines BIGINT NOT NULL DEFAULT 0,
	units BIGINT NOT NULL DEFAULT 0,
	UNIQUE(controller_id, owner_username, name)
);

CREATE TABLE IF NOT EXISTS application_offers (
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMP WITH TIME ZONE,
	updated_at TIMESTAMP WITH TIME ZONE,
	model_id BIGINT NOT NULL REFERENCES models (id) ON DELETE CASCADE,
	application_name TEXT NOT NULL,
	application_description TEXT NOT NULL,
	name TEXT NOT NULL,
	uuid TEXT NOT NULL UNIQUE,
	url TEXT NOT NULl,
	bindings BYTEA,
	charm_url TEXT NOT NULL,
	UNIQUE (model_id, application_name, name)
);

CREATE TABLE IF NOT EXISTS application_offer_connections (
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMP WITH TIME ZONE,
	updated_at TIMESTAMP WITH TIME ZONE,
	deleted_at TIMESTAMP WITH TIME ZONE,
	application_offer_id BIGINT NOT NULL REFERENCES application_offers (id) ON DELETE CASCADE,
	source_model_tag TEXT NOT NULL,
	relation_id INTEGER NOT NULL,
	username TEXT NOT NULL,
	endpoint TEXT NOT NULL,
	ingress_subnets BYTEA
);
CREATE INDEX IF NOT EXISTS idx_application_offer_connections_deleted_at ON application_offer_connections (deleted_at);


CREATE TABLE IF NOT EXISTS application_offer_remote_endpoints (
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMP WITH TIME ZONE,
	updated_at TIMESTAMP WITH TIME ZONE,
	deleted_at TIMESTAMP WITH TIME ZONE,
	application_offer_id BIGINT NOT NULL REFERENCES application_offers (id) ON DELETE CASCADE,
	name TEXT NOT NULL,
	role TEXT NOT NULL,
	interface TEXT NOT NULL,
	"limit" INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_application_offer_remote_endpoints_deleted_at ON application_offer_remote_endpoints (deleted_at);

CREATE TABLE IF NOT EXISTS application_offer_remote_spaces (
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMP WITH TIME ZONE,
	updated_at TIMESTAMP WITH TIME ZONE,
	deleted_at TIMESTAMP WITH TIME ZONE,
	application_offer_id BIGINT NOT NULL REFERENCES application_offers (id) ON DELETE CASCADE,
	cloud_type TEXT NOT NULL,
	name TEXT NOT NULL,
	provider_id TEXT NOT NULL,
	provider_attributes BYTEA
);
CREATE INDEX IF NOT EXISTS idx_application_offer_remote_spaces_deleted_at ON application_offer_remote_spaces (deleted_at);

CREATE TABLE IF NOT EXISTS user_application_offer_access (
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMP WITH TIME ZONE,
	updated_at TIMESTAMP WITH TIME ZONE,
	deleted_at TIMESTAMP WITH TIME ZONE,
	username TEXT NOT NULL REFERENCES users (username),
	application_offer_id BIGINT NOT NULL REFERENCES application_offers (id) ON DELETE CASCADE,
	access TEXT NOT NULL,
	UNIQUE (username, application_offer_id)
);
CREATE INDEX IF NOT EXISTS idx_user_application_offer_access_deleted_at ON user_application_offer_access (deleted_at);

CREATE TABLE IF NOT EXISTS user_cloud_access (
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMP WITH TIME ZONE,
	updated_at TIMESTAMP WITH TIME ZONE,
	deleted_at TIMESTAMP WITH TIME ZONE,
	username TEXT NOT NULL REFERENCES users (username),
	cloud_name TEXT NOT NULL REFERENCES clouds (name) ON DELETE CASCADE,
	access TEXT NOT NULL,
	UNIQUE (username, cloud_name)
);
CREATE INDEX IF NOT EXISTS idx_user_cloud_access_deleted_at ON user_cloud_access (deleted_at);

CREATE TABLE IF NOT EXISTS user_model_access (
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMP WITH TIME ZONE,
	updated_at TIMESTAMP WITH TIME ZONE,
	deleted_at TIMESTAMP WITH TIME ZONE,
	username TEXT NOT NULL REFERENCES users (username),
	model_id BIGINT NOT NULL REFERENCES models (id) ON DELETE CASCADE,
	access TEXT NOT NULl,
	last_connection TIMESTAMP WITH TIME ZONE,
	UNIQUE (username, model_id)
);
CREATE INDEX IF NOT EXISTS idx_user_model_access_deleted_at ON user_model_access (deleted_at);

CREATE TABLE IF NOT EXISTS user_model_defaults (
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMP WITH TIME ZONE,
	updated_at TIMESTAMP WITH TIME ZONE,
	deleted_at TIMESTAMP WITH TIME ZONE,
	username TEXT NOT NULL UNIQUE REFERENCES users (username),
	defaults BYTEA
);
CREATE INDEX IF NOT EXISTS idx_user_model_defaults_deleted_at ON user_model_defaults (deleted_at);

CREATE TABLE IF NOT EXISTS controller_configs (
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMP WITH TIME ZONE,
	updated_at TIMESTAMP WITH TIME ZONE,
	deleted_at TIMESTAMP WITH TIME ZONE,
	name TEXT NOT NULL,
	config BYTEA,
	UNIQUE(name)
);

CREATE TABLE IF NOT EXISTS root_keys (
	id BYTEA NOT NULL PRIMARY KEY,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	expires TIMESTAMP WITH TIME ZONE  NOT NULL,
	root_key BYTEA NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_root_keys_created_at ON root_keys (created_at);
CREATE INDEX IF NOT EXISTS idx_root_keys_expires ON root_keys (expires);

CREATE TABLE IF NOT EXISTS groups (
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMP WITH TIME ZONE,
	updated_at TIMESTAMP WITH TIME ZONE,
	deleted_at TIMESTAMP WITH TIME ZONE,
	name TEXT NOT NULL UNIQUE
);
CREATE INDEX IF NOT EXISTS idx_group_deleted_at ON groups (deleted_at);
CREATE INDEX IF NOT EXISTS idx_group_name ON groups (name);

UPDATE versions SET major=1, minor=0 WHERE component='jimmdb';

