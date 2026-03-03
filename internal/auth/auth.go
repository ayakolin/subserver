package auth

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUserNotFound     = errors.New("用户不存在")
	ErrInvalidPassword  = errors.New("密码错误")
	ErrUserExists       = errors.New("用户名已存在")
	ErrInvalidToken     = errors.New("无效的 token")
	ErrTokenExpired     = errors.New("token 已过期")
)

// User 用户
type User struct {
	ID        int64
	Username  string
	Password  string // 加密后的密码
	CreatedAt time.Time
}

// Auth 认证管理器
type Auth struct {
	db         *sql.DB
	jwtSecret  []byte
	tokenTTL   time.Duration
}

// NewAuth 创建新的认证管理器
func NewAuth(db *sql.DB, jwtSecret string) *Auth {
	return &Auth{
		db:        db,
		jwtSecret: []byte(jwtSecret),
		tokenTTL:  24 * time.Hour, // token 有效期 24 小时
	}
}

// Register 用户注册
func (a *Auth) Register(username, password string) (*User, error) {
	// 检查用户名是否存在
	exists, err := a.userExists(username)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrUserExists
	}

	// 加密密码
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	// 插入数据库
	query := `INSERT INTO users (username, password, created_at) VALUES (?, ?, ?)`
	result, err := a.db.Exec(query, username, string(hashedPassword), time.Now())
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &User{
		ID:        id,
		Username:  username,
		Password:  string(hashedPassword),
		CreatedAt: time.Now(),
	}, nil
}

// Login 用户登录
func (a *Auth) Login(username, password string) (*User, error) {
	// 查询用户
	query := `SELECT id, username, password, created_at FROM users WHERE username = ?`
	var user User
	var hashedPassword string
	err := a.db.QueryRow(query, username).Scan(&user.ID, &user.Username, &hashedPassword, &user.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}

	user.Password = hashedPassword

	// 验证密码
	if err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password)); err != nil {
		return nil, ErrInvalidPassword
	}

	return &user, nil
}

// GenerateToken 生成 JWT token
func (a *Auth) GenerateToken(userID int64, username string) (string, error) {
	claims := jwt.MapClaims{
		"user_id":  userID,
		"username": username,
		"exp":      time.Now().Add(a.tokenTTL).Unix(),
		"iat":      time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(a.jwtSecret)
}

// ValidateToken 验证 JWT token
func (a *Auth) ValidateToken(tokenString string) (*User, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return a.jwtSecret, nil
	})

	if err != nil || !token.Valid {
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, ErrInvalidToken
	}

	userIDFloat, ok := claims["user_id"].(float64)
	if !ok {
		return nil, ErrInvalidToken
	}
	userID := int64(userIDFloat)

	username, ok := claims["username"].(string)
	if !ok {
		return nil, ErrInvalidToken
	}

	return &User{
		ID:       userID,
		Username: username,
	}, nil
}

// userExists 检查用户是否存在
func (a *Auth) userExists(username string) (bool, error) {
	query := `SELECT COUNT(*) FROM users WHERE username = ?`
	var count int
	err := a.db.QueryRow(query, username).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetUserByID 根据 ID 获取用户
func (a *Auth) GetUserByID(userID int64) (*User, error) {
	query := `SELECT id, username, created_at FROM users WHERE id = ?`
	var user User
	err := a.db.QueryRow(query, userID).Scan(&user.ID, &user.Username, &user.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GenerateSecureSecret 生成安全的随机密钥
func GenerateSecureSecret() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
