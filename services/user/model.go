package user

import "time"

// User 是用户表。
type User struct {
	ID        int64 `gorm:"primaryKey"`
	Username  string `gorm:"uniqueIndex;size:64;not null"`
	PassHash  string `gorm:"not null"`
	Salt      string `gorm:"not null"`
	CreatedAt time.Time
}

// TableName 显式指定表名。
func (User) TableName() string { return "users" }

// Session 是登录会话(登出即删除)。
type Session struct {
	Token     string `gorm:"primaryKey;size:64"`
	UserID    int64  `gorm:"index;not null"`
	CreatedAt time.Time
}

// TableName 显式指定表名。
func (Session) TableName() string { return "sessions" }
