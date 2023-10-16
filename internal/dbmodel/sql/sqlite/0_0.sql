-- 0_0.sql initialises an empty database.

CREATE TABLE audit_log (
	id INTEGER PRIMARY KEY,
	created_at DATETIME,
	updated_at DATETIME,
	deleted_at DATETIME,
	time DATETIME,
	tag TEXT,
	user_tag TEXT,
	action TEXT,
	success INTEGER,
	params BLOB
);
CREATE INDEX idx_audit_log_deleted_at ON audit_log (deleted_at);
CREATE INDEX idx_audit_log_time ON audit_log (time);
CREATE INDEX idx_audit_log_tag ON audit_log (tag);
CREATE INDEX idx_audit_log_user_tag ON audit_log (user_tag);
CREATE INDEX idx_audit_log_action ON audit_log (action);

CREATE TABLE clouds (
	id INTEGER PRIMARY KEY,
	created_at DATETIME,
	updated_at DATETIME,
	name TEXT NOT NULL UNIQUE,
	type TEXT NOT NULL,
	host_cloud_region TEXT NOT NULL,
	auth_types BLOB,
	endpoint TEXT NOT NULL,
	identity_endpoint TEXT NOT NULL,
	storage_endpoint TEXT NOT NULL,
	ca_certificates BLOB,
	config BLOB
);

CREATE TABLE cloud_regions (
	id INTEGER PRIMARY KEY,
	created_at DATETIME,
	updated_at DATETIME,
	deleted_at DATETIME,
	cloud_name TEXT NOT NULL REFERENCES clouds (name) ON DELETE CASCADE,
	name TEXT NOT NULL,
	endpoint TEXT NOT NULL,
	identity_endpoint TEXT NOT NULL,
	storage_endpoint TEXT NOT NULL,
	config BLOB,
	UNIQUE(cloud_name, name)
);
CREATE INDEX idx_cloud_regions_deleted_at ON cloud_regions (deleted_at);

CREATE TABLE controllers (
	id INTEGER PRIMARY KEY,
	created_at DATETIME,
	updated_at DATETIME,
	deleted_at DATETIME,
	name TEXT NOT NULL UNIQUE,
	uuid TEXT NOT NULL,
	admin_user TEXT NOT NULL,
	admin_password TEXT NOT NULL,
	ca_certificate TEXT NOT NULL,
	public_address TEXT NOT NULL,
	cloud_name TEXT NOT NULL REFERENCES clouds (name) ON DELETE CASCADE,
	cloud_region TEXT NOT NULL,
	deprecated INTEGER NOT NULL DEFAULT 0,
	agent_version TEXT NOT NULL,
	addresses BLOB,
	unavailable_since DATETIME
);
CREATE INDEX idx_controllers_deleted_at ON controllers (deleted_at);

CREATE TABLE cloud_region_controller_priorities (
	id INTEGER PRIMARY KEY,
	created_at DATETIME,
	updated_at DATETIME,
	deleted_at DATETIME,
	cloud_region_id INTEGER NOT NULL REFERENCES cloud_regions (id) ON DELETE CASCADE,
	controller_id INTEGER NOT NULL REFERENCES controllers (id) ON DELETE CASCADE,
	priority INTEGER NOT NULL
);
CREATE INDEX idx_cloud_region_controller_priorities_deleted_at ON cloud_region_controller_priorities (deleted_at);

CREATE TABLE users (
	id INTEGER PRIMARY KEY,
	created_at DATETIME,
	updated_at DATETIME,
	deleted_at DATETIME,
	username TEXT NOT NULL UNIQUE,
	display_name TEXT NOT NULL,
	last_login DATETIME,
	disabled INTEGER,
	controller_access TEXT NOT NULL DEFAULT 'login',
	audit_log_access TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_users_deleted_at ON users (deleted_at);

CREATE TABLE cloud_credentials (
	id INTEGER PRIMARY KEY,
	created_at DATETIME,
	updated_at DATETIME,
	deleted_at DATETIME,
	cloud_name TEXT NOT NULL REFERENCES clouds (name),
	owner_username TEXT NOT NULL REFERENCES users (username),
	name TEXT NOT NULL,
	auth_type TEXT NOT NULL,
	label TEXT NOT NULL,
	attributes_in_vault INTEGER NOT NULL,
	attributes BLOB,
	valid INTEGER,
	UNIQUE(cloud_name, owner_username, name)
);
CREATE INDEX idx_cloud_credentials_deleted_at ON cloud_credentials (deleted_at);

CREATE TABLE cloud_defaults (
	id INTEGER PRIMARY KEY,
	created_at DATETIME,
	updated_at DATETIME,
	deleted_at DATETIME,
	username TEXT NOT NULL REFERENCES users (username),
	cloud_id INTEGER NOT NULL REFERENCES clouds (id),
	region TEXT NOT NULL,
	defaults BLOB,
	UNIQUE (username, cloud_id, region)
);
CREATE INDEX idx_cloud_defaults_deleted_at ON cloud_defaults (deleted_at);

CREATE TABLE models (
	id INTEGER PRIMARY KEY,
	created_at DATETIME,
	updated_at DATETIME,
	name TEXT NOT NULL,
	uuid TEXT UNIQUE,
	owner_username TEXT NOT NULL REFERENCES users (username),
	controller_id INTEGER REFERENCES controllers (id) ON DELETE CASCADE,
	cloud_region_id INTEGER REFERENCES cloud_regions (id),
	cloud_credential_id INTEGER REFERENCES cloud_credentials (id) ON DELETE RESTRICT,
	type TEXT NOT NULL,
	is_controller INTEGER NOT NULL,
	default_series TEXT NOT NULL,
	life TEXT NOT NULL,
	status_status TEXT NOT NULL,
	status_info TEXT NOT NULL,
	status_data BLOB,
	status_since DATETIME,
	status_version TEXT NOT NULL,
	sla_level TEXT NOT NULL,
	sla_owner TEXT NOT NULL,
	cores BIGINT NOT NULL DEFAULT 0,
	machines BIGINT NOT NULL DEFAULT 0,
	units BIGINT NOT NULL DEFAULT 0,
	UNIQUE(owner_username, name)
);

CREATE TABLE application_offers (
	id INTEGER PRIMARY KEY,
	created_at DATETIME,
	updated_at DATETIME,
	model_id INTEGER NOT NULL REFERENCES models (id) ON DELETE CASCADE,
	application_name TEXT NOT NULL,
	application_description TEXT NOT NULL,
	name TEXT NOT NULL,
	uuid TEXT NOT NULL UNIQUE,
	url TEXT NOT NULl,
	bindings BLOB,
	charm_url TEXT NOT NULL,
	UNIQUE (model_id, application_name, name)
);

CREATE TABLE application_offer_connections (
	id INTEGER PRIMARY KEY,
	created_at DATETIME,
	updated_at DATETIME,
	deleted_at DATETIME,
	application_offer_id INTEGER NOT NULL REFERENCES application_offers (id) ON DELETE CASCADE,
	source_model_tag TEXT NOT NULL,
	relation_id INTEGER NOT NULL,
	username TEXT NOT NULL,
	endpoint TEXT NOT NULL,
	ingress_subnets BLOB
);
CREATE INDEX idx_application_offer_connections_deleted_at ON application_offer_connections (deleted_at);

CREATE TABLE application_offer_remote_endpoints (
	id INTEGER PRIMARY KEY,
	created_at DATETIME,
	updated_at DATETIME,
	deleted_at DATETIME,
	application_offer_id INTEGER NOT NULL REFERENCES application_offers (id) ON DELETE CASCADE,
	name TEXT NOT NULL,
	role TEXT NOT NULL,
	interface TEXT NOT NULL,
	"limit" INTEGER NOT NULL
);
CREATE INDEX idx_application_offer_remote_endpoints_deleted_at ON application_offer_remote_endpoints (deleted_at);

CREATE TABLE application_offer_remote_spaces (
	id INTEGER PRIMARY KEY,
	created_at DATETIME,
	updated_at DATETIME,
	deleted_at DATETIME,
	application_offer_id INTEGER NOT NULL REFERENCES application_offers (id) ON DELETE CASCADE,
	cloud_type TEXT NOT NULL,
	name TEXT NOT NULL,
	provider_id TEXT NOT NULL,
	provider_attributes BLOB
);
CREATE INDEX idx_application_offer_remote_spaces_deleted_at ON application_offer_remote_spaces (deleted_at);

CREATE TABLE user_application_offer_access (
	id INTEGER PRIMARY KEY,
	created_at DATETIME,
	updated_at DATETIME,
	deleted_at DATETIME,
	username TEXT NOT NULL REFERENCES users (username),
	application_offer_id INTEGER NOT NULL REFERENCES application_offers (id) ON DELETE CASCADE,
	access TEXT NOT NULL,
	UNIQUE (username, application_offer_id)
);
CREATE INDEX idx_user_application_offer_access_deleted_at ON user_application_offer_access (deleted_at);

CREATE TABLE user_cloud_access (
	id INTEGER PRIMARY KEY,
	created_at DATETIME,
	updated_at DATETIME,
	deleted_at DATETIME,
	username TEXT NOT NULL REFERENCES users (username),
	cloud_name TEXT NOT NULL REFERENCES clouds (name) ON DELETE CASCADE,
	access TEXT NOT NULL,
	UNIQUE (username, cloud_name)
);
CREATE INDEX idx_user_cloud_access_deleted_at ON user_cloud_access (deleted_at);

CREATE TABLE user_model_access (
	id INTEGER PRIMARY KEY,
	created_at DATETIME,
	updated_at DATETIME,
	deleted_at DATETIME,
	username TEXT NOT NULL REFERENCES users (username),
	model_id INTEGER NOT NULL REFERENCES models (id) ON DELETE CASCADE,
	access TEXT NOT NULl,
	last_connection DATETIME,
	UNIQUE (username, model_id)
);
CREATE INDEX idx_user_model_access_deleted_at ON user_model_access (deleted_at);

CREATE TABLE user_model_defaults (
	id INTEGER PRIMARY KEY,
	created_at DATETIME,
	updated_at DATETIME,
	deleted_at DATETIME,
	username TEXT NOT NULL UNIQUE REFERENCES users (username),
	defaults BLOB
);
CREATE INDEX idx_user_model_defaults_deleted_at ON user_model_defaults (deleted_at);

CREATE TABLE controller_configs (
	id INTEGER PRIMARY KEY,
	created_at DATETIME,
	updated_at DATETIME,
	deleted_at DATETIME,
	name TEXT NOT NULL,
	config BYTEA,
	UNIQUE(name)
);

CREATE TABLE root_keys (
	id BLOB NOT NULL PRIMARY KEY,
	created_at DATETIME NOT NULL,
	expires DATETIME NOT NULL,
	root_key BLOB NOT NULL
);

CREATE INDEX idx_root_keys_created_at ON root_keys (created_at);
CREATE INDEX idx_root_keys_expires ON root_keys (expires);

UPDATE versions SET major=1, minor=0 WHERE component='jimmdb';
