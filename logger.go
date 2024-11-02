package auditlog

import (
	"fmt"
	"reflect"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// NewAuditLogger creates a new AuditLogger
func NewAuditLogger(db *gorm.DB, models ...interface{}) (*AuditLogger, error) {
	trackedTables := make(map[string]bool)
	namer := schema.NamingStrategy{}

	for _, model := range models {
		modelType := reflect.TypeOf(model)
		if modelType.Kind() == reflect.Ptr {
			modelType = modelType.Elem()
		}
		tableName := namer.TableName(modelType.Name())
		trackedTables[tableName] = true
	}

	dbType := ""
	dialectName := db.Dialector.Name()

	switch dialectName {
	case "mysql":
		dbType = "mysql"
	case "postgres", "postgresql":
		dbType = "postgres"
	default:
		return nil, fmt.Errorf("unsupported database type: %s", dialectName)
	}

	return &AuditLogger{DB: db, TrackedTables: trackedTables, DBType: dbType}, nil
}

// CreateAuditLogTable creates the audit_logs table
func (al *AuditLogger) CreateAuditLogTable() error {
	return al.DB.AutoMigrate(&AuditLog{})
}

// AddTrackedTable adds a table to be tracked for audit logging
func (al *AuditLogger) AddTrackedTable(tableName string) {
	al.TrackedTables[tableName] = true
}

// RemoveTrackedTable removes a table from being tracked for audit logging
func (al *AuditLogger) RemoveTrackedTable(tableName string) {
	delete(al.TrackedTables, tableName)
}

// GetTrackedTables returns a list of currently tracked tables
func (al *AuditLogger) GetTrackedTables() []string {
	tables := make([]string, 0, len(al.TrackedTables))
	for table := range al.TrackedTables {
		tables = append(tables, table)
	}
	return tables
}

// SetPerformedBy sets the performed_by value for the current transaction
func (al *AuditLogger) SetPerformedBy(tx *gorm.DB, performedBy string) *gorm.DB {
	switch al.DBType {
	case "mysql":
		if err := tx.Exec("SET @performed_by = ?", performedBy).Error; err != nil {
			tx.AddError(err)
		}
	case "postgres":
		if err := tx.Exec("SELECT set_config('audit.performed_by', $1, true)", performedBy).Error; err != nil {
			tx.AddError(err)
		}
	default:
		tx.AddError(fmt.Errorf("SetPerformedBy not implemented for database type: %s", al.DBType))
	}
	return tx.Set("performed_by", performedBy)
}