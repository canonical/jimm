-- 0_0.sql initialises an empty database.

CREATE TABLE audit_log (
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMP WITH TIME ZONE,
	updated_at TIMESTAMP WITH TIME ZONE,
	deleted_at TIMESTAMP WITH TIME ZONE,
	time TIMESTAMP WITH TIME ZONE,
	tag TEXT,
	user_tag TEXT,
	action TEXT,
	success BOOLEAN,
	params BYTEA
);
CREATE INDEX idx_audit_log_deleted_at ON audit_log (deleted_at);
CREATE INDEX idx_audit_log_time ON audit_log (time);
CREATE INDEX idx_audit_log_tag ON audit_log (tag);
CREATE INDEX idx_audit_log_user_tag ON audit_log (user_tag);
CREATE INDEX idx_audit_log_action ON audit_log (action);

CREATE TABLE clouds (
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

CREATE TABLE cloud_regions (
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
CREATE INDEX idx_cloud_regions_deleted_at ON cloud_regions (deleted_at);

CREATE TABLE controllers (
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
CREATE INDEX idx_controllers_deleted_at ON controllers (deleted_at);

CREATE TABLE cloud_region_controller_priorities (
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMP WITH TIME ZONE,
	updated_at TIMESTAMP WITH TIME ZONE,
	deleted_at TIMESTAMP WITH TIME ZONE,
	cloud_region_id INTEGER NOT NULL REFERENCES cloud_regions (id) ON DELETE CASCADE,
	controller_id INTEGER NOT NULL REFERENCES controllers (id) ON DELETE CASCADE,
	priority INTEGER NOT NULL
);
CREATE INDEX idx_cloud_region_controller_priorities_deleted_at ON cloud_region_controller_priorities (deleted_at);

CREATE TABLE users (
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMP WITH TIME ZONE,
	updated_at TIMESTAMP WITH TIME ZONE,
	deleted_at TIMESTAMP WITH TIME ZONE,
	username TEXT NOT NULL UNIQUE,
	display_name TEXT NOT NULL,
	last_login TIMESTAMP WITH TIME ZONE,
	disabled BOOLEAN,
	controller_access TEXT NOT NULL DEFAULT 'add-model',
	audit_log_access TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_users_deleted_at ON users (deleted_at);

CREATE TABLE cloud_credentials (
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
CREATE INDEX idx_cloud_credentials_deleted_at ON cloud_credentials (deleted_at);

CREATE TABLE cloud_defaults (
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
CREATE INDEX idx_cloud_defaults_deleted_at ON cloud_defaults (deleted_at);

CREATE TABLE models (
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
	UNIQUE(owner_username, name)
);

CREATE TABLE applications (
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMP WITH TIME ZONE,
	updated_at TIMESTAMP WITH TIME ZONE,
	model_id BIGINT NOT NULL REFERENCES models (id) ON DELETE CASCADE,
	name TEXT NOT NULL,
	exposed BOOLEAN NOT NULL,
	charm_url TEXT NOT NULL,
	life TEXT NOT NULL,
	min_units INTEGER,
	constraint_arch TEXT,
	constraint_container TEXT,
	constraint_mem INTEGER,
	constraint_root_disk INTEGER,
	constraint_root_disk_source TEXT,
	constraint_cpu_cores INTEGER,
	constraint_cpu_power INTEGER,
	constraint_tags BYTEA,
	constraint_availability_zone TEXT,
	constraint_zones BYTEA,
	constraint_instance_type TEXT,
	constraint_spaces BYTEA,
	constraint_virt_type TEXT,
	constraint_allocate_public_ip BOOLEAN,
	config BYTEA,
	subordinate BOOLEAN NOT NULL,
	status_status TEXT NOT NULL,
	status_info TEXT NOT NULL,
	status_data BYTEA,
	status_since TIMESTAMP WITH TIME ZONE,
	status_version TEXT NOT NULL,
	workload_version TEXT NOT NULL,
	UNIQUE (model_id, name)
);

CREATE TABLE application_offers (
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
	FOREIGN KEY (model_id, application_name) REFERENCES applications (model_id, name) ON DELETE CASCADE,
	UNIQUE (model_id, application_name, name)
);

CREATE TABLE application_offer_connections (
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
CREATE INDEX idx_application_offer_connections_deleted_at ON application_offer_connections (deleted_at);


CREATE TABLE application_offer_remote_endpoints (
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
CREATE INDEX idx_application_offer_remote_endpoints_deleted_at ON application_offer_remote_endpoints (deleted_at);

CREATE TABLE application_offer_remote_spaces (
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
CREATE INDEX idx_application_offer_remote_spaces_deleted_at ON application_offer_remote_spaces (deleted_at);


CREATE TABLE machines (
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMP WITH TIME ZONE,
	updated_at TIMESTAMP WITH TIME ZONE,
	model_id BIGINT NOT NULL REFERENCES models (id) ON DELETE CASCADE,
	machine_id TEXT NOT NULL,
	hw_arch TEXT,
	hw_container TEXT,
	hw_mem INTEGER,
	hw_root_disk INTEGER,
	hw_root_disk_source TEXT,
	hw_cpu_cores INTEGER,
	hw_cpu_power INTEGER,
	hw_tags BYTEA,
	hw_availability_zone TEXT,
	hw_zones BYTEA,
	hw_instance_type TEXT,
	hw_spaces BYTEA,
	hw_virt_type TEXT,
	hw_allocate_public_ip BOOLEAN,
	instance_id TEXT NOT NULL,
	display_name TEXT NOT NULL,
	agent_status_status TEXT NOT NULL,
	agent_status_info TEXT NOT NULL,
	agent_status_data BYTEA,
	agent_status_since TIMESTAMP WITH TIME ZONE,
	agent_status_version TEXT NOT NULL,
	instance_status_status TEXT NOT NULL,
	instance_status_info TEXT NOT NULL,
	instance_status_data BYTEA,
	instance_status_since TIMESTAMP WITH TIME ZONE,
	instance_status_version TEXT NOT NULL,
	life TEXT NOT NULL,
	has_vote BOOLEAN NOT NULL,
	wants_vote BOOLEAN NOT NULL,
	series TEXT NOT NULL,
	UNIQUE (model_id, machine_id)
);

CREATE TABLE units (
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMP WITH TIME ZONE,
	updated_at TIMESTAMP WITH TIME ZONE,
	model_id BIGINT NOT NULL REFERENCES models (id) ON DELETE CASCADE,
	application_name TEXT NOT NULL,
	machine_id TEXT NOT NULL,
	name TEXT NOT NULL,
	life TEXT NOT NULL,
	public_address TEXT NOT NULL,
	private_address TEXT NOT NULL,
	ports BYTEA,
	port_ranges BYTEA,
	principal TEXT NOT NULL,
	workload_status_status TEXT NOT NULL,
	workload_status_info TEXT NOT NULL,
	workload_status_data BYTEA,
	workload_status_since TIMESTAMP WITH TIME ZONE,
	workload_status_version TEXT NOT NULL,
	agent_status_status TEXT NOT NULL,
	agent_status_info TEXT NOT NULL,
	agent_status_data BYTEA,
	agent_status_since TIMESTAMP WITH TIME ZONE,
	agent_status_version TEXT NOT NULL,
	UNIQUE (model_id, name),
	FOREIGN KEY (model_id, application_name) REFERENCES applications (model_id, name) ON DELETE CASCADE,
	FOREIGN KEY (model_id, machine_id) REFERENCES machines (model_id, machine_id) ON DELETE CASCADE
);

CREATE TABLE user_application_offer_access (
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMP WITH TIME ZONE,
	updated_at TIMESTAMP WITH TIME ZONE,
	deleted_at TIMESTAMP WITH TIME ZONE,
	username TEXT NOT NULL REFERENCES users (username),
	application_offer_id BIGINT NOT NULL REFERENCES application_offers (id) ON DELETE CASCADE,
	access TEXT NOT NULL,
	UNIQUE (username, application_offer_id)
);
CREATE INDEX idx_user_application_offer_access_deleted_at ON user_application_offer_access (deleted_at);

CREATE TABLE user_cloud_access (
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMP WITH TIME ZONE,
	updated_at TIMESTAMP WITH TIME ZONE,
	deleted_at TIMESTAMP WITH TIME ZONE,
	username TEXT NOT NULL REFERENCES users (username),
	cloud_name TEXT NOT NULL REFERENCES clouds (name) ON DELETE CASCADE,
	access TEXT NOT NULL,
	UNIQUE (username, cloud_name)
);
CREATE INDEX idx_user_cloud_access_deleted_at ON user_cloud_access (deleted_at);

CREATE TABLE user_model_access (
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
CREATE INDEX idx_user_model_access_deleted_at ON user_model_access (deleted_at);

CREATE TABLE user_model_defaults (
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMP WITH TIME ZONE,
	updated_at TIMESTAMP WITH TIME ZONE,
	deleted_at TIMESTAMP WITH TIME ZONE,
	username TEXT NOT NULL UNIQUE REFERENCES users (username),
	defaults BYTEA
);
CREATE INDEX idx_user_model_defaults_deleted_at ON user_model_defaults (deleted_at);

CREATE TABLE controller_configs (
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMP WITH TIME ZONE,
	updated_at TIMESTAMP WITH TIME ZONE,
	deleted_at TIMESTAMP WITH TIME ZONE,
	name TEXT NOT NULL,
	config BYTEA,
	UNIQUE(name)
);

UPDATE versions SET major=1, minor=0 WHERE component='jimmdb';
