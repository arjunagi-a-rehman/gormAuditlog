package auditlog

import (
	"time"

	"gorm.io/gorm"
)

// AuditLog represents the structure of our audit log
type AuditLog struct {
	ID           uint      `gorm:"primaryKey"`
	RecordID     string    `gorm:"index"`
	TableName    string    `gorm:"index"`
	Action       string    `gorm:"index"`
	Timestamp    time.Time `gorm:"index"`
	CurrentValue string    `gorm:"type:text"`
	PerformedBy  string    `gorm:"index"`
}

// AuditLogger is a struct that holds the database connection and tracked tables
type AuditLogger struct {
	DB            *gorm.DB
	TrackedTables map[string]bool
	DBType        string
}
