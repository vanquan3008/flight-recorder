-- schema_migrations is created and owned by golang-migrate, so it is not in
-- db/migrations. It is declared here only so sqlc can type-check queries on it;
-- this directory is never applied to a database.
CREATE TABLE schema_migrations (
    version bigint  NOT NULL PRIMARY KEY,
    dirty   boolean NOT NULL
);
