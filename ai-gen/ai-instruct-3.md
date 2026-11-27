# ai-instruct-3

## PostgreSQL support

For now no migration has been played:
* Prepare equivalent migration for PostgreSQL `cwmerch-db/mysql` and `cwmerch-db/postgresql`
* CWMerch can be configured for new project on PostgreSQL has well
* I want generic environment variables: `DB_TYPE`, `DB_HOST`, `DB_NAME`, `DB_PORT` (default: 5432 if `DB_TYPE` is `postgres` or `postgresql` or `pgsql` or `pg` or `psql`, 3306 if it's `mysql` or `mariadb` or `my`)

## Clone database

On admin I want a feature that allow to export data to the main database to a target database with a button "clone data" (the database is supposed to be up to date in term of migrations and can already contain data so handle duplicates).

It can be cloned from one type to another or same type of database.

Update also the documentation to say that cwmerch allows also to change to PostgreSQL (something we've dreamed of during years).
