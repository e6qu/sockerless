-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Shauth and Ory Hydra share one PostgreSQL instance but require isolated
-- databases because they own independent migration histories. The
-- POSTGRES_DB/POSTGRES_USER environment on the postgres service creates and
-- owns "shauth"; this script adds the second database Hydra's own migration
-- needs.
CREATE DATABASE hydra;
