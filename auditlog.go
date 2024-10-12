package auditlog

import (
	"encoding/json"
	"fmt"
	"reflect"
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
	ChangedBy    string    `gorm:"index"`
}

// AuditLogger is a struct that holds the database connection and tracked tables
type AuditLogger struct {
	DB            *gorm.DB
	TrackedTables map[string]bool
	DBType        string
}

// NewAuditLogger creates a new AuditLogger
func NewAuditLogger(db *gorm.DB, tables []string) (*AuditLogger, error) {
	trackedTables := make(map[string]bool)
	for _, table := range tables {
		trackedTables[table] = true
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
	var sql string
	if al.DBType == "mysql" {
		sql = `
			CREATE TABLE IF NOT EXISTS audit_logs (
				id INT AUTO_INCREMENT PRIMARY KEY,
				record_id INT,
				table_name VARCHAR(255),
				action VARCHAR(50),
				timestamp DATETIME,
				current_value TEXT,
				changed_by VARCHAR(255),
				INDEX idx_audit_logs_record_id (record_id),
				INDEX idx_audit_logs_table_name (table_name),
				INDEX idx_audit_logs_action (action),
				INDEX idx_audit_logs_timestamp (timestamp),
				INDEX idx_audit_logs_changed_by (changed_by)
			);
		`
	} else if al.DBType == "postgres" {
		sql = `
			CREATE TABLE IF NOT EXISTS audit_logs (
				id SERIAL PRIMARY KEY,
				record_id INTEGER,
				table_name VARCHAR(255),
				action VARCHAR(50),
				timestamp TIMESTAMP,
				current_value TEXT,
				changed_by VARCHAR(255)
			);
			CREATE INDEX IF NOT EXISTS idx_audit_logs_record_id ON audit_logs(record_id);
			CREATE INDEX IF NOT EXISTS idx_audit_logs_table_name ON audit_logs(table_name);
			CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs(action);
			CREATE INDEX IF NOT EXISTS idx_audit_logs_timestamp ON audit_logs(timestamp);
			CREATE INDEX IF NOT EXISTS idx_audit_logs_changed_by ON audit_logs(changed_by);
		`
	}

	return al.DB.Exec(sql).Error
}

// CreateTriggers creates database triggers for the tracked tables
func (al *AuditLogger) CreateTriggers() error {
	for table := range al.TrackedTables {
		if err := al.createTableTriggers(table); err != nil {
			return err
		}
	}
	return nil
}

func (al *AuditLogger) createTableTriggers(tableName string) error {
	if al.DBType == "mysql" {
		return al.createMySQLTriggers(tableName)
	} else if al.DBType == "postgres" {
		return al.createPostgresTriggers(tableName)
	}
	return fmt.Errorf("unsupported database type: %s", al.DBType)
}

func (al *AuditLogger) createMySQLTriggers(tableName string) error {
	createTrigger := func(triggerName, timing, event string) error {
		sql := fmt.Sprintf(`
			CREATE TRIGGER %s
			%s %s ON %s
			FOR EACH ROW
			BEGIN
				INSERT INTO audit_logs (record_id, table_name, action, timestamp, current_value, changed_by)
				VALUES (
					CASE WHEN '%s' = 'DELETE' THEN OLD.id ELSE NEW.id END,
					'%s',
					'%s',
					NOW(),
					CASE
						WHEN '%s' = 'DELETE' THEN JSON_OBJECT('old', OLD)
						WHEN '%s' = 'UPDATE' THEN JSON_OBJECT('old', OLD, 'new', NEW)
						ELSE JSON_OBJECT('new', NEW)
					END,
					IFNULL(@changed_by, 'system')
				);
			END;
		`, triggerName, timing, event, tableName, event, tableName, event, event, event)

		return al.DB.Exec(sql).Error
	}

	if err := createTrigger(tableName+"_insert_trigger", "AFTER", "INSERT"); err != nil {
		return err
	}
	if err := createTrigger(tableName+"_update_trigger", "AFTER", "UPDATE"); err != nil {
		return err
	}
	if err := createTrigger(tableName+"_delete_trigger", "BEFORE", "DELETE"); err != nil {
		return err
	}

	return nil
}

func (al *AuditLogger) createPostgresTriggers(tableName string) error {
	sql := fmt.Sprintf(`
		CREATE OR REPLACE FUNCTION %s_audit() RETURNS TRIGGER AS $$
		BEGIN
			INSERT INTO audit_logs (record_id, table_name, action, timestamp, current_value, changed_by)
			VALUES (
				CASE WHEN TG_OP = 'DELETE' THEN OLD.id ELSE NEW.id END,
				TG_TABLE_NAME,
				TG_OP,
				NOW(),
				CASE
					WHEN TG_OP = 'DELETE' THEN row_to_json(OLD)::text
					WHEN TG_OP = 'UPDATE' THEN json_build_object('old', row_to_json(OLD), 'new', row_to_json(NEW))::text
					ELSE row_to_json(NEW)::text
				END,
				COALESCE(current_setting('audit.changed_by', true), 'system')
			);
			RETURN NULL;
		END;
		$$ LANGUAGE plpgsql;

		DROP TRIGGER IF EXISTS %s_insert_trigger ON %s;
		CREATE TRIGGER %s_insert_trigger
		AFTER INSERT ON %s
		FOR EACH ROW EXECUTE FUNCTION %s_audit();

		DROP TRIGGER IF EXISTS %s_update_trigger ON %s;
		CREATE TRIGGER %s_update_trigger
		AFTER UPDATE ON %s
		FOR EACH ROW EXECUTE FUNCTION %s_audit();

		DROP TRIGGER IF EXISTS %s_delete_trigger ON %s;
		CREATE TRIGGER %s_delete_trigger
		AFTER DELETE ON %s
		FOR EACH ROW EXECUTE FUNCTION %s_audit();
	`, tableName, tableName, tableName, tableName, tableName, tableName, tableName, tableName, tableName, tableName, tableName, tableName, tableName, tableName, tableName, tableName)

	return al.DB.Exec(sql).Error
}

// LogChanges is a method to be used as a GORM hook
func (al *AuditLogger) LogChanges(tx *gorm.DB) {
	if tx.Statement.Schema == nil {
		return
	}

	tableName := tx.Statement.Table
	if !al.TrackedTables[tableName] {
		return
	}

	switch tx.Statement.ReflectValue.Kind() {
	case reflect.Slice, reflect.Array:
		for i := 0; i < tx.Statement.ReflectValue.Len(); i++ {
			al.logSingleRecord(tx, tx.Statement.ReflectValue.Index(i).Interface(), tableName)
		}
	case reflect.Struct:
		al.logSingleRecord(tx, tx.Statement.ReflectValue.Interface(), tableName)
	}
}

func (al *AuditLogger) logSingleRecord(tx *gorm.DB, record interface{}, tableName string) {
	currentValues, _ := json.Marshal(record)

	pkValue := getPrimaryKeyValue(tx, record)
	var recordID string
	if pkValue != nil {
		recordID = fmt.Sprintf("%v", pkValue) // Convert any type to string
	}

	auditLog := AuditLog{
		RecordID:     recordID,
		TableName:    tableName,
		Action:       getAction(tx),
		Timestamp:    time.Now(),
		CurrentValue: string(currentValues),
		ChangedBy:    getChangedBy(tx),
	}

	al.DB.Create(&auditLog)
}

func getPrimaryKeyValue(tx *gorm.DB, record interface{}) interface{} {
	if field := tx.Statement.Schema.PrioritizedPrimaryField; field != nil {
		value, isZero := field.ValueOf(tx.Statement.Context, reflect.ValueOf(record))
		if !isZero {
			return value
		}
	}
	return nil
}

func getAction(tx *gorm.DB) string {
	switch tx.Statement.ReflectValue.Kind() {
	case reflect.Slice, reflect.Array:
		if tx.Statement.Changed() {
			return "BulkUpdate"
		}
		return "BulkCreate"
	default:
		if tx.Statement.Changed() {
			return "Update"
		}
		return "Create"
	}
}

func getChangedBy(tx *gorm.DB) string {
	if changedBy, ok := tx.Get("changed_by"); ok {
		return changedBy.(string)
	}
	return "system"
}

// RegisterHooks registers the audit log hooks with GORM
func (al *AuditLogger) RegisterHooks() {
	al.DB.Callback().Create().After("gorm:create").Register("audit_log:create", al.LogChanges)
	al.DB.Callback().Update().After("gorm:update").Register("audit_log:update", al.LogChanges)
	al.DB.Callback().Delete().After("gorm:delete").Register("audit_log:delete", al.LogChanges)
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

// SetChangedBy sets the changed_by value for the current transaction
func (al *AuditLogger) SetChangedBy(tx *gorm.DB, changedBy string) *gorm.DB {
	if al.DBType == "mysql" {
		return tx.Exec("SET @changed_by = ?", changedBy)
	} else if al.DBType == "postgres" {
		return tx.Exec("SET LOCAL audit.changed_by = ?", changedBy)
	}
	return tx
}