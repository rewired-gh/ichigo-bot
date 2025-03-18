package app

import (
	"database/sql"
	"os"
	"path/filepath"

	"log/slog"

	_ "modernc.org/sqlite"
)

const (
	dataDbName = "data.db"
)

// New schema: sessions table holds session_id, model and temperature.
// chat_records table holds a record id, session_id, role (int) and content.

func OpenSessionDB(dataDir string) *sql.DB {
	dbPath := filepath.Join(dataDir, dataDbName)
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		slog.Error("failed to create data directory", "error", err)
	}
	// Updated DSN for modernc.org/sqlite driver.
	dsn := "file:" + dbPath + "?mode=rwc"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		slog.Error("failed to open session DB", "error", err)
		return nil
	}
	// Create tables if not exist.
	schema := `
	CREATE TABLE IF NOT EXISTS sessions (
		session_id INTEGER PRIMARY KEY,
		model TEXT,
		temperature REAL,
		prompt TEXT
	);
	CREATE TABLE IF NOT EXISTS chat_records (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id INTEGER,
		role INTEGER,
		content TEXT,
		FOREIGN KEY(session_id) REFERENCES sessions(session_id)
	);
	CREATE INDEX IF NOT EXISTS idx_chat_records_session_id ON chat_records(session_id);
	`
	if _, err := db.Exec(schema); err != nil {
		slog.Error("failed to create tables", "error", err)
	}

	// Check if prompt column exists in sessions table, add it if not
	var hasPromptColumn bool
	err = db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('sessions') WHERE name='prompt'").Scan(&hasPromptColumn)
	if err != nil {
		slog.Error("failed to check for prompt column", "error", err)
	} else if !hasPromptColumn {
		// Add the prompt column to existing sessions table
		_, err := db.Exec("ALTER TABLE sessions ADD COLUMN prompt TEXT")
		if err != nil {
			slog.Error("failed to add prompt column to sessions table", "error", err)
		} else {
			slog.Info("added prompt column to existing sessions table")
		}
	}

	return db
}

func UpdateSessionMetadata(db *sql.DB, sessionID int64, model string, temperature float32, prompt string) {
	// Upsert sessions row.
	stmt := `
	INSERT INTO sessions(session_id, model, temperature, prompt)
	VALUES(?, ?, ?, ?)
	ON CONFLICT(session_id) DO UPDATE SET model=excluded.model, temperature=excluded.temperature, prompt=excluded.prompt;
	`
	if _, err := db.Exec(stmt, sessionID, model, temperature, prompt); err != nil {
		slog.Error("failed to update session metadata", "userID", sessionID, "error", err)
	}
}

func ClearAllMetadata(db *sql.DB) {
	stmt := `DELETE FROM sessions;`
	if _, err := db.Exec(stmt); err != nil {
		slog.Error("failed to clear all metadata", "error", err)
	}
}

func AppendChatRecord(db *sql.DB, sessionID int64, role int, content string) {
	stmt := `
	INSERT INTO chat_records(session_id, role, content)
	VALUES(?, ?, ?);
	`
	if _, err := db.Exec(stmt, sessionID, role, content); err != nil {
		slog.Error("failed to append chat record", "userID", sessionID, "error", err)
	}
}

func DeleteLastChatRecord(db *sql.DB, sessionID int64) {
	// Delete the record with the highest id for the user.
	stmt := `
	DELETE FROM chat_records
	WHERE id = (SELECT id FROM chat_records
	            WHERE session_id = ?
	            ORDER BY id DESC LIMIT 1);
	`
	if _, err := db.Exec(stmt, sessionID); err != nil {
		slog.Error("failed to delete last chat record", "userID", sessionID, "error", err)
	}
}

type StoredSession struct {
	Model       string
	Temperature float32
	Prompt      string
	ChatRecords []ChatRecord
}

func LoadSession(db *sql.DB, sessionID int64) (StoredSession, error) {
	var ss StoredSession
	row := db.QueryRow("SELECT model, temperature, prompt FROM sessions WHERE session_id = ?", sessionID)
	var prompt sql.NullString
	err := row.Scan(&ss.Model, &ss.Temperature, &prompt)
	if err != nil {
		return ss, err
	}
	if prompt.Valid {
		ss.Prompt = prompt.String
	} else {
		ss.Prompt = ""
	}
	rows, err := db.Query("SELECT id, role, content FROM chat_records WHERE session_id = ? ORDER BY id ASC", sessionID)
	if err != nil {
		return ss, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var roleInt int
		var content string
		if err := rows.Scan(&id, &roleInt, &content); err != nil {
			continue
		}
		ss.ChatRecords = append(ss.ChatRecords, ChatRecord{DBID: id, Role: ChatRole(roleInt), Content: content})
	}
	return ss, nil
}

func ClearChatRecords(db *sql.DB, sessionID int64) {
	stmt := `DELETE FROM chat_records WHERE session_id = ?;`
	if _, err := db.Exec(stmt, sessionID); err != nil {
		slog.Error("failed to clear chat records", "userID", sessionID, "error", err)
	}
}

func ClearAllChatRecords(db *sql.DB) {
	stmt := `DELETE FROM chat_records;`
	if _, err := db.Exec(stmt); err != nil {
		slog.Error("failed to clear all chat records", "error", err)
	}
}

func TrimOldChatRecords(db *sql.DB, sessionID int64, keepCount int) {
	// Delete chat records except the most recent keepCount by id.
	stmt := `
	DELETE FROM chat_records
	WHERE session_id = ?
	    AND id NOT IN (
	        SELECT id FROM chat_records
	        WHERE session_id = ?
	        ORDER BY id DESC
	        LIMIT ?
	    );
	`
	if _, err := db.Exec(stmt, sessionID, sessionID, keepCount); err != nil {
		slog.Error("failed to trim chat records", "sessionID", sessionID, "error", err)
	}
}

func TidyObsoleteSessions(db *sql.DB, validIDs []int64) (int, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var affected int64
	if len(validIDs) == 0 {
		if _, err := tx.Exec("DELETE FROM chat_records"); err != nil {
			return 0, err
		}
		res, err := tx.Exec("DELETE FROM sessions")
		if err != nil {
			return 0, err
		}
		affected, _ = res.RowsAffected()
	} else {
		placeholders := ""
		args := make([]interface{}, len(validIDs))
		for i, id := range validIDs {
			placeholders += "?,"
			args[i] = id
		}
		placeholders = placeholders[:len(placeholders)-1]

		chatSQL := "DELETE FROM chat_records WHERE session_id NOT IN (" + placeholders + ")"
		if _, err := tx.Exec(chatSQL, args...); err != nil {
			return 0, err
		}

		sessSQL := "DELETE FROM sessions WHERE session_id NOT IN (" + placeholders + ")"
		res, err := tx.Exec(sessSQL, args...)
		if err != nil {
			return 0, err
		}
		affected, _ = res.RowsAffected()
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}

	return int(affected), nil
}
