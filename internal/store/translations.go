package store

import (
	"context"

	"strings"
)

const translationCols = `id, user_id, kind, source, source_lang, detected_lang, target_lang, source_text, result_text,
	engine, model, tone, starred, collection, note, learn_ref, external_id, created_at, updated_at`

func scanTranslation(sc interface{ Scan(...any) error }) (*Translation, error) {
	t := &Translation{}
	var starred int
	err := sc.Scan(&t.ID, &t.UserID, &t.Kind, &t.Source, &t.SourceLang, &t.DetectedLang, &t.TargetLang, &t.SourceText, &t.ResultText,
		&t.Engine, &t.Model, &t.Tone, &starred, &t.Collection, &t.Note, &t.LearnRef, &t.ExternalID, &t.CreatedAt, &t.UpdatedAt)
	t.Starred = starred == 1
	return t, err
}

// AddTranslation saves one translation for a person. With an external id
// already there (an import run twice), the existing row is returned and
// dup is true.
func (s *Store) AddTranslation(ctx context.Context, uid int64, t *Translation) (dup bool, err error) {
	if t.ExternalID != "" {
		row := s.DB.QueryRowContext(ctx, `SELECT `+translationCols+` FROM translations WHERE user_id = ? AND source = ? AND external_id = ?`, uid, t.Source, t.ExternalID)
		if old, err := scanTranslation(row); err == nil {
			*t = *old
			return true, nil
		}
	}
	if t.Kind == "" {
		t.Kind = "text"
	}
	now := Now()
	res, err := s.DB.ExecContext(ctx, `INSERT INTO translations (user_id, kind, source, source_lang, detected_lang, target_lang, source_text, result_text,
		engine, model, tone, starred, collection, note, learn_ref, external_id, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		uid, t.Kind, t.Source, t.SourceLang, t.DetectedLang, t.TargetLang, t.SourceText, t.ResultText,
		t.Engine, t.Model, t.Tone, boolInt(t.Starred), strings.TrimSpace(t.Collection), t.Note, t.LearnRef, t.ExternalID, firstSet(t.CreatedAt, now), now)
	if err != nil {
		return false, conflict(err)
	}
	t.ID, _ = res.LastInsertId()
	t.UserID = uid
	if t.CreatedAt == "" {
		t.CreatedAt = now
	}
	t.UpdatedAt = now
	return false, nil
}

func firstSet(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// GetTranslation returns one of a person's translations.
func (s *Store) GetTranslation(ctx context.Context, uid, id int64) (*Translation, error) {
	t, err := scanTranslation(s.DB.QueryRowContext(ctx, `SELECT `+translationCols+` FROM translations WHERE id = ? AND user_id = ?`, id, uid))
	return t, notFound(err)
}

// UpdateTranslation saves the editable parts of a translation: the result
// (a person's own correction), star, collection, note and Learn card.
func (s *Store) UpdateTranslation(ctx context.Context, uid int64, t *Translation) error {
	t.UpdatedAt = Now()
	res, err := s.DB.ExecContext(ctx, `UPDATE translations SET result_text = ?, starred = ?, collection = ?, note = ?, learn_ref = ?, updated_at = ? WHERE id = ? AND user_id = ?`,
		t.ResultText, boolInt(t.Starred), strings.TrimSpace(t.Collection), t.Note, t.LearnRef, t.UpdatedAt, t.ID, uid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteTranslation removes one translation.
func (s *Store) DeleteTranslation(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM translations WHERE id = ? AND user_id = ?`, id, uid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ClearHistory removes a person's history; starred translations (the
// phrasebook) stay unless all is true. It returns how many went.
func (s *Store) ClearHistory(ctx context.Context, uid int64, all bool) (int64, error) {
	q := `DELETE FROM translations WHERE user_id = ?`
	if !all {
		q += ` AND starred = 0`
	}
	res, err := s.DB.ExecContext(ctx, q, uid)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// PruneHistory removes a person's unstarred translations older than a date.
func (s *Store) PruneHistory(ctx context.Context, uid int64, before string) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM translations WHERE user_id = ? AND starred = 0 AND created_at < ?`, uid, before)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// TranslationFilter narrows a list.
type TranslationFilter struct {
	Q          string
	Starred    bool
	Collection *string
	Kind       string
	Lang       string // either side
	Source     string // only rows this linked app added
	Day        string // YYYY-MM-DD, local
	Limit      int
	Offset     int
}

// ListTranslations lists a person's translations, newest first.
func (s *Store) ListTranslations(ctx context.Context, uid int64, f TranslationFilter) ([]*Translation, int, error) {
	where := []string{"user_id = ?"}
	args := []any{uid}
	if f.Starred {
		where = append(where, "starred = 1")
	}
	if f.Collection != nil {
		where = append(where, "collection = ?")
		args = append(args, *f.Collection)
	}
	if f.Kind != "" {
		where = append(where, "kind = ?")
		args = append(args, f.Kind)
	}
	if f.Source != "" {
		where = append(where, "source = ?")
		args = append(args, f.Source)
	}
	if f.Lang != "" {
		where = append(where, "(target_lang = ? OR detected_lang = ? OR source_lang = ?)")
		args = append(args, f.Lang, f.Lang, f.Lang)
	}
	if f.Day != "" {
		where = append(where, "date(created_at, 'localtime') = ?")
		args = append(args, f.Day)
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		like := "%" + strings.NewReplacer("%", `\%`, "_", `\_`, `\`, `\\`).Replace(q) + "%"
		where = append(where, `(source_text LIKE ? ESCAPE '\' OR result_text LIKE ? ESCAPE '\' OR note LIKE ? ESCAPE '\' OR collection LIKE ? ESCAPE '\')`)
		args = append(args, like, like, like, like)
	}
	w := strings.Join(where, " AND ")
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM translations WHERE `+w, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	order := "created_at DESC, id DESC"
	if f.Starred {
		order = "collection, created_at DESC, id DESC"
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+translationCols+` FROM translations WHERE `+w+` ORDER BY `+order+` LIMIT ? OFFSET ?`, append(args, limit, f.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []*Translation{}
	for rows.Next() {
		t, err := scanTranslation(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, t)
	}
	return out, total, rows.Err()
}

// AllTranslations is every translation of a person, oldest first (export).
func (s *Store) AllTranslations(ctx context.Context, uid int64) ([]*Translation, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+translationCols+` FROM translations WHERE user_id = ? ORDER BY id`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Translation{}
	for rows.Next() {
		t, err := scanTranslation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Collection is a phrasebook group and how many phrases it has.
type Collection struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// Collections lists a person's phrasebook groups ("" = not in one).
func (s *Store) Collections(ctx context.Context, uid int64) ([]Collection, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT collection, COUNT(1) FROM translations WHERE user_id = ? AND starred = 1 GROUP BY collection ORDER BY collection = '', collection COLLATE NOCASE`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Collection{}
	for rows.Next() {
		var c Collection
		if err := rows.Scan(&c.Name, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// RenameCollection moves a whole phrasebook group to another name.
func (s *Store) RenameCollection(ctx context.Context, uid int64, from, to string) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `UPDATE translations SET collection = ?, updated_at = ? WHERE user_id = ? AND starred = 1 AND collection = ?`, strings.TrimSpace(to), Now(), uid, from)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Stats are a person's totals for the overview.
type Stats struct {
	Total     int            `json:"total"`
	Starred   int            `json:"starred"`
	Today     int            `json:"today"`
	Week      int            `json:"week"`
	Documents int            `json:"documents"`
	Talks     int            `json:"conversations"`
	Terms     int            `json:"terms"`
	Pairs     []PairCount    `json:"pairs"`
	Kinds     map[string]int `json:"kinds"`
}

// PairCount is how often a person translated from one language to another.
type PairCount struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Count int    `json:"count"`
}

// StatsFor counts a person's translations.
func (s *Store) StatsFor(ctx context.Context, uid int64, today, weekStart string) (Stats, error) {
	st := Stats{Pairs: []PairCount{}, Kinds: map[string]int{}}
	q := func(dst *int, sql string, args ...any) {
		_ = s.DB.QueryRowContext(ctx, sql, args...).Scan(dst)
	}
	q(&st.Total, `SELECT COUNT(1) FROM translations WHERE user_id = ?`, uid)
	q(&st.Starred, `SELECT COUNT(1) FROM translations WHERE user_id = ? AND starred = 1`, uid)
	q(&st.Today, `SELECT COUNT(1) FROM translations WHERE user_id = ? AND date(created_at, 'localtime') = ?`, uid, today)
	q(&st.Week, `SELECT COUNT(1) FROM translations WHERE user_id = ? AND date(created_at, 'localtime') >= ?`, uid, weekStart)
	q(&st.Documents, `SELECT COUNT(1) FROM documents WHERE user_id = ?`, uid)
	q(&st.Talks, `SELECT COUNT(1) FROM conversations WHERE user_id = ?`, uid)
	q(&st.Terms, `SELECT COUNT(1) FROM glossary WHERE user_id = ?`, uid)
	rows, err := s.DB.QueryContext(ctx, `SELECT CASE WHEN detected_lang <> '' THEN detected_lang ELSE source_lang END AS f, target_lang, COUNT(1) n
		FROM translations WHERE user_id = ? GROUP BY f, target_lang ORDER BY n DESC LIMIT 8`, uid)
	if err != nil {
		return st, err
	}
	for rows.Next() {
		var p PairCount
		if err := rows.Scan(&p.From, &p.To, &p.Count); err == nil {
			st.Pairs = append(st.Pairs, p)
		}
	}
	rows.Close()
	rows, err = s.DB.QueryContext(ctx, `SELECT kind, COUNT(1) FROM translations WHERE user_id = ? GROUP BY kind`, uid)
	if err != nil {
		return st, err
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		var n int
		if err := rows.Scan(&k, &n); err == nil {
			st.Kinds[k] = n
		}
	}
	return st, rows.Err()
}

// HistoryUsers lists people whose history is kept only for some days.
func (s *Store) HistoryUsers(ctx context.Context) ([]int64, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT DISTINCT user_id FROM user_settings WHERE key = 'history_days' AND value NOT IN ('0', '')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err == nil {
			out = append(out, id)
		}
	}
	return out, rows.Err()
}
