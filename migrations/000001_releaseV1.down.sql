DROP INDEX IF EXISTS idx_service_versions_service_id;
DROP INDEX IF EXISTS idx_services_name_trgm;
DROP INDEX IF EXISTS idx_services_search_vector;
DROP TABLE IF EXISTS service_versions;
DROP TABLE IF EXISTS services;
DROP TABLE IF EXISTS users;
