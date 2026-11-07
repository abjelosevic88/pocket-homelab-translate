# Backup

The words live in one SQLite file, `DATA_DIR/pocket-translate.db` (WAL mode); the pictures are files in `DATA_DIR/media/`. Take a consistent copy of the database with `pocket-translate backup` (writes `DATA_DIR/backups/pocket-translate-<time>.db` with `VACUUM INTO`) or Settings → Backup & data → Download database (admin), and back up `media/` as it is. Back up that copy, not the live file. `DATA_DIR/core.token*` is the Pocket Core registration.

Restore: stop the app, replace `pocket-translate.db` (remove `-wal`/`-shm` files) and `media/`, start.

Every person can also export their own forms with every answer as JSON (`/api/v1/export.json`, back in with Settings → Backup & data), and one form's answers as CSV (`/api/v1/forms/<id>/export.csv`).
