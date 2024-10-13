package auditlog

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	dsn := "host=localhost user=zander password=1234 dbname=test port=5432 sslmode=disable TimeZone=UTC"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	assert.NoError(t, err, "Failed to connect to database")

	// Test the connection
	sqlDB, err := db.DB()
	assert.NoError(t, err, "Failed to get database instance")

	err = sqlDB.Ping()
	assert.NoError(t, err, "Failed to ping database")

	// Drop existing tables if they exist
	db.Exec("DROP TABLE IF EXISTS test_users")
	db.Exec("DROP TABLE IF EXISTS audit_logs")

	// Migrate the schema
	err = db.AutoMigrate(&TestUser{})
	assert.NoError(t, err, "Failed to migrate TestUser schema")

	return db
}

func TestAuditLogger(t *testing.T) {
	db := setupTestDB(t)

	// Initialize AuditLogger
	auditLogger, err := NewAuditLogger(db, []string{"test_users"})
	assert.NoError(t, err, "Failed to create Audit Logger")

	// Create audit log table
	err = auditLogger.CreateAuditLogTable()
	assert.NoError(t, err, "Failed to create audit log table")

	// Create triggers
	err = auditLogger.CreateTriggers()
	assert.NoError(t, err, "Failed to create triggers")

	// Register hooks
	// auditLogger.RegisterHooks()

	// Test Create
	t.Run("Create", func(t *testing.T) {
		user := TestUser{ID: 1, Name: "John Doe", Email: "john@example.com"}
		result := db.Create(&user)
		assert.NoError(t, result.Error, "Failed to create user")
		assert.NotZero(t, user.ID, "User ID should not be zero")

		var auditLog AuditLog
		recordId := fmt.Sprintf("%v", user.ID)
		err := db.Where("table_name = ? AND action = ?  AND record_id = ?", "test_users", "INSERT", recordId).First(&auditLog).Error
		fmt.Println(auditLog, "auditLog")
		assert.NoError(t, err, "Failed to find audit log for create operation")
		assert.Equal(t, "INSERT", auditLog.Action, "Audit log action should be INSERT")
		assert.Equal(t, "john@example.com", user.Email, "User email should match")
	})

	// Test Update
	t.Run("Update", func(t *testing.T) {
		var user TestUser
		err := db.First(&user).Error
		assert.NoError(t, err, "Failed to fetch user")

		user.Name = "Jane Doe"
		result := db.Save(&user)
		assert.NoError(t, result.Error, "Failed to update user")

		var auditLog AuditLog
		recordId := fmt.Sprintf("%v", user.ID)
		err = db.Where("table_name = ? AND action = ? AND record_id = ?", "test_users", "UPDATE", recordId).First(&auditLog).Error
		assert.NoError(t, err, "Failed to find audit log for update operation")
		assert.Equal(t, "UPDATE", auditLog.Action, "Audit log action should be UPDATE")
	})

	// Test Delete
	t.Run("Delete", func(t *testing.T) {
		var user TestUser
		err := db.First(&user).Error
		assert.NoError(t, err, "Failed to fetch user")

		result := db.Delete(&user)
		assert.NoError(t, result.Error, "Failed to delete user")

		var auditLog AuditLog
		recordId := fmt.Sprintf("%v", user.ID)

		err = db.Where("table_name = ? AND action = ? AND record_id = ?", "test_users", "DELETE", recordId).First(&auditLog).Error
		assert.NoError(t, err, "Failed to find audit log for delete operation")
		assert.Equal(t, "DELETE", auditLog.Action, "Audit log action should be DELETE")
	})

	// Test SetPerformedBy
	t.Run("SetPerformedBy", func(t *testing.T) {
		user := TestUser{Name: "Alice", Email: "alice@example.com"}
		tx := db.Begin()
		tx = auditLogger.SetPerformedBy(tx, "admin")
		result := tx.Create(&user)
		assert.NoError(t, result.Error, "Failed to create user with SetPerformedBy")
		tx.Commit()

		var auditLog AuditLog
		recordId := fmt.Sprintf("%v", user.ID)

		err := db.Where("table_name = ? AND action = ? AND record_id = ? AND performed_by = ? ", "test_users", "INSERT", recordId, "admin").First(&auditLog).Error
		fmt.Println(auditLog, "auditLog")
		fmt.Println(auditLog.PerformedBy, "auditLog.chnageBy")
		assert.NoError(t, err, "Failed to find audit log for SetPerformedBy operation")
		assert.Equal(t, "admin", auditLog.PerformedBy, "PerformedBy should be set to admin")
	})

	t.Run("SetPerformedBy", func(t *testing.T) {
		user := TestUser{Name: "kallu", Email: "kallu@example.com"}
		tx := db.Begin()
		// tx = auditLogger.SetPerformedBy(tx, "admin")
		result := tx.Create(&user)
		assert.NoError(t, result.Error, "Failed to create user with SetPerformedBy")
		tx.Commit()

		var auditLog AuditLog
		recordId := fmt.Sprintf("%v", user.ID)

		err := db.Where("table_name = ? AND action = ? AND record_id = ? AND performed_by = ? ", "test_users", "INSERT", recordId, "system").First(&auditLog).Error
		fmt.Println(auditLog, "auditLog")
		fmt.Println(auditLog.PerformedBy, "auditLog.chnageBy")
		assert.NoError(t, err, "Failed to find audit log for SetPerformedBy operation")
		assert.Equal(t, "admin", auditLog.PerformedBy, "PerformedBy should be set to admin")
	})

	// Clean up
	 db.Exec("DROP TABLE IF EXISTS test_users")
	 db.Exec("DROP TABLE IF EXISTS audit_logs")
}
