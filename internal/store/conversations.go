package store

import (
	"context"
	"strings"
)

const convCols = `c.id, c.user_id, c.title, c.lang_a, c.lang_b, c.created_at, c.updated_at,
	(SELECT COUNT(1) FROM turns t WHERE t.conversation_id = c.id),
	COALESCE((SELECT t.source_text FROM turns t WHERE t.conversation_id = c.id ORDER BY t.id DESC LIMIT 1), '')`

func scanConv(sc interface{ Scan(...any) error }) (*Conversation, error) {
	c := &Conversation{}
	err := sc.Scan(&c.ID, &c.UserID, &c.Title, &c.LangA, &c.LangB, &c.CreatedAt, &c.UpdatedAt, &c.Turns, &c.Last)
	return c, err
}

// AddConversation starts a conversation.
func (s *Store) AddConversation(ctx context.Context, uid int64, c *Conversation) error {
	now := Now()
	c.CreatedAt, c.UpdatedAt = firstSet(c.CreatedAt, now), firstSet(c.UpdatedAt, now)
	res, err := s.DB.ExecContext(ctx, `INSERT INTO conversations (user_id, title, lang_a, lang_b, created_at, updated_at) VALUES (?,?,?,?,?,?)`,
		uid, strings.TrimSpace(c.Title), c.LangA, c.LangB, c.CreatedAt, c.UpdatedAt)
	if err != nil {
		return err
	}
	c.ID, _ = res.LastInsertId()
	c.UserID = uid
	return nil
}

// GetConversation returns one of a person's conversations.
func (s *Store) GetConversation(ctx context.Context, uid, id int64) (*Conversation, error) {
	c, err := scanConv(s.DB.QueryRowContext(ctx, `SELECT `+convCols+` FROM conversations c WHERE c.id = ? AND c.user_id = ?`, id, uid))
	return c, notFound(err)
}

// UpdateConversation saves the title and the two languages.
func (s *Store) UpdateConversation(ctx context.Context, uid int64, c *Conversation) error {
	c.UpdatedAt = Now()
	res, err := s.DB.ExecContext(ctx, `UPDATE conversations SET title = ?, lang_a = ?, lang_b = ?, updated_at = ? WHERE id = ? AND user_id = ?`,
		strings.TrimSpace(c.Title), c.LangA, c.LangB, c.UpdatedAt, c.ID, uid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteConversation removes a conversation and what was said.
func (s *Store) DeleteConversation(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM conversations WHERE id = ? AND user_id = ?`, id, uid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListConversations lists a person's conversations, most recent first.
func (s *Store) ListConversations(ctx context.Context, uid int64, limit int) ([]*Conversation, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+convCols+` FROM conversations c WHERE c.user_id = ? ORDER BY c.updated_at DESC, c.id DESC LIMIT ?`, uid, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Conversation{}
	for rows.Next() {
		c, err := scanConv(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

const turnCols = `id, user_id, conversation_id, side, spoken, source_lang, target_lang, source_text, result_text, engine, created_at`

func scanTurn(sc interface{ Scan(...any) error }) (*Turn, error) {
	t := &Turn{}
	var spoken int
	err := sc.Scan(&t.ID, &t.UserID, &t.ConversationID, &t.Side, &spoken, &t.SourceLang, &t.TargetLang, &t.SourceText, &t.ResultText, &t.Engine, &t.CreatedAt)
	t.Spoken = spoken == 1
	return t, err
}

// AddTurn adds what one side said to a conversation.
func (s *Store) AddTurn(ctx context.Context, uid int64, t *Turn) error {
	now := Now()
	t.CreatedAt = firstSet(t.CreatedAt, now)
	res, err := s.DB.ExecContext(ctx, `INSERT INTO turns (user_id, conversation_id, side, spoken, source_lang, target_lang, source_text, result_text, engine, created_at)
		SELECT ?,?,?,?,?,?,?,?,?,? WHERE EXISTS (SELECT 1 FROM conversations WHERE id = ? AND user_id = ?)`,
		uid, t.ConversationID, t.Side, boolInt(t.Spoken), t.SourceLang, t.TargetLang, t.SourceText, t.ResultText, t.Engine, t.CreatedAt, t.ConversationID, uid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	t.ID, _ = res.LastInsertId()
	t.UserID = uid
	_, _ = s.DB.ExecContext(ctx, `UPDATE conversations SET updated_at = ? WHERE id = ?`, t.CreatedAt, t.ConversationID)
	return nil
}

// Turns lists what was said in a conversation, in order.
func (s *Store) Turns(ctx context.Context, uid, convID int64) ([]*Turn, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+turnCols+` FROM turns WHERE conversation_id = ? AND user_id = ? ORDER BY id`, convID, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Turn{}
	for rows.Next() {
		t, err := scanTurn(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// GetTurn returns one turn of a person's.
func (s *Store) GetTurn(ctx context.Context, uid, id int64) (*Turn, error) {
	t, err := scanTurn(s.DB.QueryRowContext(ctx, `SELECT `+turnCols+` FROM turns WHERE id = ? AND user_id = ?`, id, uid))
	return t, notFound(err)
}

// DeleteTurn removes one thing said.
func (s *Store) DeleteTurn(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM turns WHERE id = ? AND user_id = ?`, id, uid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
