package store

import (
	"context"
	"strings"
)

const termCols = `id, user_id, source_lang, target_lang, term, translation, note, match_case, created_at`

func scanTerm(sc interface{ Scan(...any) error }) (*Term, error) {
	t := &Term{}
	var mc int
	err := sc.Scan(&t.ID, &t.UserID, &t.SourceLang, &t.TargetLang, &t.Term, &t.Translation, &t.Note, &mc, &t.CreatedAt)
	t.MatchCase = mc == 1
	return t, err
}

// AddTerm adds a glossary entry; the same term for the same pair is a conflict.
func (s *Store) AddTerm(ctx context.Context, uid int64, t *Term) error {
	t.Term, t.Translation = strings.TrimSpace(t.Term), strings.TrimSpace(t.Translation)
	res, err := s.DB.ExecContext(ctx, `INSERT INTO glossary (user_id, source_lang, target_lang, term, translation, note, match_case) VALUES (?,?,?,?,?,?,?)`,
		uid, t.SourceLang, t.TargetLang, t.Term, t.Translation, t.Note, boolInt(t.MatchCase))
	if err != nil {
		return conflict(err)
	}
	t.ID, _ = res.LastInsertId()
	t.UserID = uid
	t.CreatedAt = Now()
	return nil
}

// UpsertTerm adds a term or, when the pair already has it, sets its
// translation (imports). It reports whether the term was new.
func (s *Store) UpsertTerm(ctx context.Context, uid int64, t *Term) (bool, error) {
	var id int64
	err := s.DB.QueryRowContext(ctx, `SELECT id FROM glossary WHERE user_id = ? AND source_lang = ? AND target_lang = ? AND term = ? COLLATE NOCASE`,
		uid, t.SourceLang, t.TargetLang, strings.TrimSpace(t.Term)).Scan(&id)
	if err != nil {
		return true, s.AddTerm(ctx, uid, t)
	}
	t.ID = id
	_, err = s.DB.ExecContext(ctx, `UPDATE glossary SET translation = ?, note = CASE WHEN ? <> '' THEN ? ELSE note END WHERE id = ? AND user_id = ?`,
		strings.TrimSpace(t.Translation), t.Note, t.Note, id, uid)
	return false, err
}

// GetTerm returns one glossary entry.
func (s *Store) GetTerm(ctx context.Context, uid, id int64) (*Term, error) {
	t, err := scanTerm(s.DB.QueryRowContext(ctx, `SELECT `+termCols+` FROM glossary WHERE id = ? AND user_id = ?`, id, uid))
	return t, notFound(err)
}

// UpdateTerm saves a glossary entry.
func (s *Store) UpdateTerm(ctx context.Context, uid int64, t *Term) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE glossary SET source_lang = ?, target_lang = ?, term = ?, translation = ?, note = ?, match_case = ? WHERE id = ? AND user_id = ?`,
		t.SourceLang, t.TargetLang, strings.TrimSpace(t.Term), strings.TrimSpace(t.Translation), t.Note, boolInt(t.MatchCase), t.ID, uid)
	if err != nil {
		return conflict(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteTerm removes a glossary entry.
func (s *Store) DeleteTerm(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM glossary WHERE id = ? AND user_id = ?`, id, uid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListTerms lists a person's glossary; with from/to only that pair
// (a term written for "any" language, "*", matches every pair).
func (s *Store) ListTerms(ctx context.Context, uid int64, from, to, q string) ([]*Term, error) {
	where := "user_id = ?"
	args := []any{uid}
	if from != "" {
		where += " AND source_lang IN (?, '*')"
		args = append(args, from)
	}
	if to != "" {
		where += " AND target_lang IN (?, '*')"
		args = append(args, to)
	}
	if q = strings.TrimSpace(q); q != "" {
		where += " AND (term LIKE ? OR translation LIKE ? OR note LIKE ?)"
		like := "%" + q + "%"
		args = append(args, like, like, like)
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+termCols+` FROM glossary WHERE `+where+` ORDER BY source_lang, target_lang, term COLLATE NOCASE`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Term{}
	for rows.Next() {
		t, err := scanTerm(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
