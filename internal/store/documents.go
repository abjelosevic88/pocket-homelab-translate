package store

import (
	"context"
	"encoding/json"
)

const docCols = `id, user_id, name, format, mime, size, source_lang, detected_lang, target_lang, engine, used, status, done, total,
	error, result_name, origin, saved, created_at, finished_at`

func scanDoc(sc interface{ Scan(...any) error }) (*Document, error) {
	d := &Document{}
	var saved string
	err := sc.Scan(&d.ID, &d.UserID, &d.Name, &d.Format, &d.Mime, &d.Size, &d.SourceLang, &d.DetectedLang, &d.TargetLang, &d.Engine, &d.Used,
		&d.Status, &d.Done, &d.Total, &d.Error, &d.ResultName, &d.Origin, &saved, &d.CreatedAt, &d.FinishedAt)
	_ = json.Unmarshal([]byte(saved), &d.Saved)
	if d.Saved == nil {
		d.Saved = []Saved{}
	}
	return d, err
}

// AddDocument stores a document to translate (queued) with its file.
func (s *Store) AddDocument(ctx context.Context, uid int64, d *Document, data []byte) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if d.Saved == nil {
		d.Saved = []Saved{}
	}
	if d.Status == "" {
		d.Status = DocQueued
	}
	d.CreatedAt = Now()
	res, err := tx.ExecContext(ctx, `INSERT INTO documents (user_id, name, format, mime, size, source_lang, target_lang, engine, status, origin, saved, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, uid, d.Name, d.Format, d.Mime, len(data), d.SourceLang, d.TargetLang, d.Engine, d.Status, d.Origin, toJSON(d.Saved), d.CreatedAt)
	if err != nil {
		return err
	}
	d.ID, _ = res.LastInsertId()
	d.UserID, d.Size = uid, int64(len(data))
	if _, err := tx.ExecContext(ctx, `INSERT INTO document_files (document_id, which, data) VALUES (?, 'original', ?)`, d.ID, data); err != nil {
		return err
	}
	return tx.Commit()
}

// GetDocument returns one of a person's documents (uid 0 = any, for the worker).
func (s *Store) GetDocument(ctx context.Context, uid, id int64) (*Document, error) {
	q, args := `SELECT `+docCols+` FROM documents WHERE id = ?`, []any{id}
	if uid > 0 {
		q += ` AND user_id = ?`
		args = append(args, uid)
	}
	d, err := scanDoc(s.DB.QueryRowContext(ctx, q, args...))
	return d, notFound(err)
}

// ListDocuments lists a person's documents, newest first.
func (s *Store) ListDocuments(ctx context.Context, uid int64, limit int) ([]*Document, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+docCols+` FROM documents WHERE user_id = ? ORDER BY created_at DESC, id DESC LIMIT ?`, uid, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Document{}
	for rows.Next() {
		d, err := scanDoc(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DocumentFile returns a document's original or translated file.
func (s *Store) DocumentFile(ctx context.Context, uid, id int64, which string) ([]byte, error) {
	var b []byte
	err := s.DB.QueryRowContext(ctx, `SELECT f.data FROM document_files f JOIN documents d ON d.id = f.document_id WHERE f.document_id = ? AND f.which = ? AND (? = 0 OR d.user_id = ?)`,
		id, which, uid, uid).Scan(&b)
	return b, notFound(err)
}

// NextDocument claims the oldest queued document for the worker.
func (s *Store) NextDocument(ctx context.Context) (*Document, error) {
	d, err := scanDoc(s.DB.QueryRowContext(ctx, `SELECT `+docCols+` FROM documents WHERE status = 'queued' ORDER BY id LIMIT 1`))
	if err != nil {
		return nil, notFound(err)
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE documents SET status = 'working', done = 0, error = '' WHERE id = ?`, d.ID)
	d.Status = DocWorking
	return d, err
}

// RequeueWorking puts documents that were being translated when the app
// stopped back in the queue.
func (s *Store) RequeueWorking(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE documents SET status = 'queued', done = 0 WHERE status = 'working'`)
	return err
}

// DocumentProgress records how far the worker is.
func (s *Store) DocumentProgress(ctx context.Context, id int64, done, total int, detected string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE documents SET done = ?, total = ?, detected_lang = CASE WHEN ? <> '' THEN ? ELSE detected_lang END WHERE id = ?`, done, total, detected, detected, id)
	return err
}

// FinishDocument stores the translated file (or the error).
func (s *Store) FinishDocument(ctx context.Context, d *Document, result []byte) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	d.FinishedAt = Now()
	if _, err := tx.ExecContext(ctx, `UPDATE documents SET status = ?, error = ?, used = ?, result_name = ?, done = ?, total = ?, detected_lang = ?, finished_at = ? WHERE id = ?`,
		d.Status, d.Error, d.Used, d.ResultName, d.Done, d.Total, d.DetectedLang, d.FinishedAt, d.ID); err != nil {
		return err
	}
	if result != nil {
		if _, err := tx.ExecContext(ctx, `INSERT INTO document_files (document_id, which, data) VALUES (?, 'result', ?) ON CONFLICT(document_id, which) DO UPDATE SET data = excluded.data`, d.ID, result); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// RetryDocument queues a failed (or finished) document again, maybe in another language.
func (s *Store) RetryDocument(ctx context.Context, uid, id int64, target, engine string) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE documents SET status = 'queued', done = 0, total = 0, error = '', finished_at = '',
		target_lang = CASE WHEN ? <> '' THEN ? ELSE target_lang END, engine = CASE WHEN ? <> '' THEN ? ELSE engine END
		WHERE id = ? AND user_id = ? AND status IN ('done', 'failed')`, target, target, engine, engine, id, uid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	_, err = s.DB.ExecContext(ctx, `DELETE FROM document_files WHERE document_id = ? AND which = 'result'`, id)
	return err
}

// AddSaved notes where a translated document was put.
func (s *Store) AddSaved(ctx context.Context, uid, id int64, sv Saved) error {
	d, err := s.GetDocument(ctx, uid, id)
	if err != nil {
		return err
	}
	d.Saved = append(d.Saved, sv)
	_, err = s.DB.ExecContext(ctx, `UPDATE documents SET saved = ? WHERE id = ? AND user_id = ?`, toJSON(d.Saved), id, uid)
	return err
}

// DeleteDocument removes a document and its files.
func (s *Store) DeleteDocument(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM documents WHERE id = ? AND user_id = ? AND status <> 'working'`, id, uid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
