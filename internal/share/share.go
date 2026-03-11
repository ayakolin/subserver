package share

import (
	"database/sql"
	"time"
)

// Share 分享
type Share struct {
	ID        int64
	UserID    int64
	FileID    string
	Name      string // 分享显示名称
	File      *ShareFile
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ShareFile 分享文件
type ShareFile struct {
	ID        string
	Name      string
	Content   []byte
	Size      int64
	MimeType  string
	ExpiresAt *time.Time
	Once      bool
	ReadCount int
}

// Manager 分享管理器
type Manager struct {
	db *sql.DB
}

// NewManager 创建分享管理器
func NewManager(db *sql.DB) *Manager {
	return &Manager{
		db: db,
	}
}

// CreateShare 创建分享
func (m *Manager) CreateShare(userID int64, fileID string, name string) (*Share, error) {
	query := `INSERT INTO shares (user_id, file_id, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`
	result, err := m.db.Exec(query, userID, fileID, name, time.Now(), time.Now())
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &Share{
		ID:        id,
		UserID:    userID,
		FileID:    fileID,
		Name:      name,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, nil
}

// GetUserShares 获取用户的所有分享
func (m *Manager) GetUserShares(userID int64) ([]*Share, error) {
	query := `
	SELECT s.id, s.user_id, s.file_id, s.name, s.created_at, s.updated_at,
	       f.name, f.size, f.mime_type, f.read_count
	FROM shares s
	LEFT JOIN files f ON s.file_id = f.id
	WHERE s.user_id = ?
	ORDER BY s.created_at DESC
	`

	rows, err := m.db.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var shares []*Share
	for rows.Next() {
		var share Share
		var shareName sql.NullString
		var fileName sql.NullString
		var fileSize sql.NullInt64
		var mimeType sql.NullString
		var readCount sql.NullInt64

		err := rows.Scan(
			&share.ID,
			&share.UserID,
			&share.FileID,
			&shareName,
			&share.CreatedAt,
			&share.UpdatedAt,
			&fileName,
			&fileSize,
			&mimeType,
			&readCount,
		)
		if err != nil {
			return nil, err
		}

		if shareName.Valid {
			share.Name = shareName.String
		}

		if fileName.Valid {
			share.File = &ShareFile{
				ID:        share.FileID,
				Name:      fileName.String,
				Size:      fileSize.Int64,
				MimeType:  mimeType.String,
				ReadCount: int(readCount.Int64),
			}
		}

		shares = append(shares, &share)
	}

	return shares, nil
}

// GetShareByID 根据 ID 获取分享
func (m *Manager) GetShareByID(id int64, userID int64) (*Share, error) {
	query := `
	SELECT s.id, s.user_id, s.file_id, s.created_at, s.updated_at,
	       f.name, f.content, f.size, f.mime_type, f.expires_at, f.once, f.read_count
	FROM shares s
	LEFT JOIN files f ON s.file_id = f.id
	WHERE s.id = ? AND s.user_id = ?
	`

	var share Share
	var fileName sql.NullString
	var content sql.NullString
	var fileSize sql.NullInt64
	var mimeType sql.NullString
	var expiresAt sql.NullTime
	var once bool
	var readCount sql.NullInt64

	err := m.db.QueryRow(query, id, userID).Scan(
		&share.ID,
		&share.UserID,
		&share.FileID,
		&share.CreatedAt,
		&share.UpdatedAt,
		&fileName,
		&content,
		&fileSize,
		&mimeType,
		&expiresAt,
		&once,
		&readCount,
	)

	if err == sql.ErrNoRows {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, err
	}

	if fileName.Valid {
		share.File = &ShareFile{
			ID:        share.FileID,
			Name:      fileName.String,
			Content:   []byte(content.String),
			Size:      fileSize.Int64,
			MimeType:  mimeType.String,
			Once:      once,
			ReadCount: int(readCount.Int64),
		}
		if expiresAt.Valid {
			share.File.ExpiresAt = &expiresAt.Time
		}
	}

	return &share, nil
}

// GetShareByFileID 根据文件 ID 和用户 ID 获取分享
func (m *Manager) GetShareByFileID(fileID string, userID int64) (*Share, error) {
	query := `
	SELECT s.id, s.user_id, s.file_id, s.created_at, s.updated_at,
	       f.name, f.content, f.size, f.mime_type, f.expires_at, f.once, f.read_count
	FROM shares s
	LEFT JOIN files f ON s.file_id = f.id
	WHERE s.file_id = ? AND s.user_id = ?
	`

	var share Share
	var fileName sql.NullString
	var content sql.NullString
	var fileSize sql.NullInt64
	var mimeType sql.NullString
	var expiresAt sql.NullTime
	var once bool
	var readCount sql.NullInt64

	err := m.db.QueryRow(query, fileID, userID).Scan(
		&share.ID,
		&share.UserID,
		&share.FileID,
		&share.CreatedAt,
		&share.UpdatedAt,
		&fileName,
		&content,
		&fileSize,
		&mimeType,
		&expiresAt,
		&once,
		&readCount,
	)

	if err == sql.ErrNoRows {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, err
	}

	if fileName.Valid {
		share.File = &ShareFile{
			ID:        share.FileID,
			Name:      fileName.String,
			Content:   []byte(content.String),
			Size:      fileSize.Int64,
			MimeType:  mimeType.String,
			Once:      once,
			ReadCount: int(readCount.Int64),
		}
		if expiresAt.Valid {
			share.File.ExpiresAt = &expiresAt.Time
		}
	}

	return &share, nil
}

// UpdateShareContent 更新分享内容
func (m *Manager) UpdateShareContent(fileID string, name string, content []byte, mimeType string) error {
	query := `UPDATE files SET name = ?, content = ?, mime_type = ? WHERE id = ?`
	_, err := m.db.Exec(query, name, content, mimeType, fileID)
	return err
}

// UpdateShareName 更新分享显示名称
func (m *Manager) UpdateShareName(shareID int64, userID int64, name string) error {
	query := `UPDATE shares SET name = ?, updated_at = ? WHERE id = ? AND user_id = ?`
	result, err := m.db.Exec(query, name, time.Now(), shareID, userID)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return sql.ErrNoRows
	}

	return nil
}

// DeleteShare 删除分享及关联文件
func (m *Manager) DeleteShare(id int64, userID int64) error {
	// 先获取分享关联的文件 ID
	var fileID string
	query := `SELECT file_id FROM shares WHERE id = ? AND user_id = ?`
	err := m.db.QueryRow(query, id, userID).Scan(&fileID)
	if err != nil {
		return err
	}

	// 删除分享记录
	query = `DELETE FROM shares WHERE id = ? AND user_id = ?`
	if _, err := m.db.Exec(query, id, userID); err != nil {
		return err
	}

	// 检查是否还有其他分享引用同一文件
	var count int
	query = `SELECT COUNT(*) FROM shares WHERE file_id = ?`
	if err := m.db.QueryRow(query, fileID).Scan(&count); err != nil {
		return err
	}

	// 如果没有其他分享引用该文件，删除文件
	if count == 0 {
		query = `DELETE FROM files WHERE id = ?`
		if _, err := m.db.Exec(query, fileID); err != nil {
			return err
		}
	}

	return nil
}

// DeleteFile 删除文件（级联删除分享）
func (m *Manager) DeleteFile(fileID string, userID int64) error {
	// 删除文件，shares 表会级联删除
	query := `DELETE FROM files WHERE id = ?`
	_, err := m.db.Exec(query, fileID)
	return err
}
