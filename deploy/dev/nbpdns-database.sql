-- nbpdns's role and database in the lab's PostgreSQL 18, postgres-47
-- (ADR-0046), which make lab-up runs with psql, as the superuser, once the
-- lab is healthy. Each is created only if it's missing, so that a volume
-- from before them gets them too, and a second run does nothing. The
-- password is for this lab only.
SELECT 'CREATE ROLE nbpdns LOGIN PASSWORD ''nbpdns-lab-nbpdns-db''' -- gitleaks:allow
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'nbpdns') \gexec
SELECT 'CREATE DATABASE nbpdns OWNER nbpdns'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'nbpdns') \gexec
