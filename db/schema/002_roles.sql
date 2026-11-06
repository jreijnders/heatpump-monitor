-- Roles and grants. Passwords come in as psql variables (see db/migrate.sh):
--   :ingester_password, :grafana_ro_password, :claude_ro_password
--
-- analytics_ro is a NOLOGIN group role holding read access to readings and every
-- analytics view/function; grafana_ro and claude_ro are members of it.
-- Later schema files GRANT SELECT on their views to analytics_ro.

SELECT 'CREATE ROLE analytics_ro NOLOGIN'
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'analytics_ro') \gexec

SELECT format('CREATE ROLE %I LOGIN', r)
FROM unnest(ARRAY['ingester', 'grafana_ro', 'claude_ro']) AS r
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = r) \gexec

ALTER ROLE ingester   WITH LOGIN PASSWORD :'ingester_password';
ALTER ROLE grafana_ro WITH LOGIN PASSWORD :'grafana_ro_password';
ALTER ROLE claude_ro  WITH LOGIN PASSWORD :'claude_ro_password';

-- Read-only roles: refuse writes at the session level too, and cap runaway queries.
ALTER ROLE grafana_ro SET default_transaction_read_only = on;
ALTER ROLE claude_ro  SET default_transaction_read_only = on;
ALTER ROLE claude_ro  SET statement_timeout = '60s';

GRANT analytics_ro TO grafana_ro, claude_ro;

GRANT USAGE ON SCHEMA public TO ingester, analytics_ro;
GRANT INSERT, SELECT ON readings TO ingester;
GRANT SELECT ON readings TO analytics_ro;
