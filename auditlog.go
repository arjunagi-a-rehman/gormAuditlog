package auditlog

import (
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"log"
	"strings"

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
	return al.DB.AutoMigrate(&AuditLog{})
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
	// INSERT trigger
	insertTrigger := fmt.Sprintf(`
		CREATE TRIGGER %s_insert_trigger
		AFTER INSERT ON %s
		FOR EACH ROW
		BEGIN
			INSERT INTO audit_logs (record_id, table_name, action, timestamp, current_value, performed_by)
			VALUES (
				NEW.id,
				'%s',
				'INSERT',
				NOW(),
				JSON_OBJECT(
					'id', NEW.id,
					'name', NEW.name,
					'email', NEW.email,
					'created_at', NEW.created_at,
					'updated_at', NEW.updated_at
				),
				IFNULL(@performed_by, 'system')
			);
		END;
	`, tableName, tableName, tableName)

	// UPDATE trigger
	updateTrigger := fmt.Sprintf(`
		CREATE TRIGGER %s_update_trigger
		AFTER UPDATE ON %s
		FOR EACH ROW
		BEGIN
			INSERT INTO audit_logs (record_id, table_name, action, timestamp, current_value, performed_by)
			VALUES (
				NEW.id,
				'%s',
				'UPDATE',
				NOW(),
				JSON_OBJECT(
					'id', NEW.id,
					'name', NEW.name,
					'email', NEW.email,
					'created_at', NEW.created_at,
					'updated_at', NEW.updated_at
				),
				IFNULL(@performed_by, 'system')
			);
		END;
	`, tableName, tableName, tableName)

	// DELETE trigger
	deleteTrigger := fmt.Sprintf(`
		CREATE TRIGGER %s_delete_trigger
		BEFORE DELETE ON %s
		FOR EACH ROW
		BEGIN
			INSERT INTO audit_logs (record_id, table_name, action, timestamp, current_value, performed_by)
			VALUES (
				OLD.id,
				'%s',
				'DELETE',
				NOW(),
				JSON_OBJECT(
					'id', OLD.id,
					'name', OLD.name,
					'email', OLD.email,
					'created_at', OLD.created_at,
					'updated_at', OLD.updated_at
				),
				IFNULL(@performed_by, 'system')
			);
		END;
	`, tableName, tableName, tableName)

	// Execute each trigger creation
	if err := al.DB.Exec(insertTrigger).Error; err != nil {
		return fmt.Errorf("failed to create INSERT trigger: %w", err)
	}
	if err := al.DB.Exec(updateTrigger).Error; err != nil {
		return fmt.Errorf("failed to create UPDATE trigger: %w", err)
	}
	if err := al.DB.Exec(deleteTrigger).Error; err != nil {
		return fmt.Errorf("failed to create DELETE trigger: %w", err)
	}

	return nil
}

func (al *AuditLogger) createPostgresTriggers(tableName string) error {
	sql := fmt.Sprintf(`
		CREATE OR REPLACE FUNCTION %s_audit() RETURNS TRIGGER AS $$
		DECLARE
			audit_performed_by TEXT;
			record_id TEXT;
		BEGIN
			-- Try to get the performed_by value, default to 'system' if not set
			audit_performed_by := COALESCE(current_setting('audit.performed_by', true), 'system');

			IF (TG_OP = 'DELETE') THEN
				record_id := OLD.id::text;
			ELSE
				record_id := NEW.id::text;
			END IF;

			-- Log the values for debugging
			RAISE NOTICE 'Audit trigger called: table=%%, op=%%, record_id=%%, performed_by=%%', 
				TG_TABLE_NAME, TG_OP, record_id, audit_performed_by;

			INSERT INTO audit_logs (record_id, table_name, action, timestamp, current_value, performed_by)
			VALUES (
				record_id,
				TG_TABLE_NAME,
				TG_OP,
				NOW(),
				CASE
					WHEN TG_OP = 'DELETE' THEN row_to_json(OLD)::text
					WHEN TG_OP = 'UPDATE' THEN row_to_json(NEW)::text
					ELSE row_to_json(NEW)::text
				END,
				audit_performed_by
			);

			-- Log the inserted audit log for debugging
			RAISE NOTICE 'Audit log inserted: %%', (SELECT row_to_json(audit_logs.*) FROM audit_logs WHERE id = lastval());

			IF TG_OP = 'DELETE' THEN
				RETURN OLD;
			ELSE
				RETURN NEW;
			END IF;
		END;
		$$ LANGUAGE plpgsql;

		DROP TRIGGER IF EXISTS %s_audit_trigger ON %s;
		CREATE TRIGGER %s_audit_trigger
		AFTER INSERT OR UPDATE OR DELETE ON %s
		FOR EACH ROW EXECUTE FUNCTION %s_audit();
	`, tableName, tableName, tableName, tableName, tableName, tableName)

	result := al.DB.Exec(sql)
	if result.Error != nil {
		log.Printf("Error creating PostgreSQL trigger for table %s: %v", tableName, result.Error)
	} else {
		log.Printf("Successfully created PostgreSQL trigger for table %s", tableName)
	}
	return result.Error
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
	recordID := fmt.Sprintf("%v", pkValue)
	action := getAction(tx)

	auditLog := AuditLog{
		RecordID:     recordID,
		TableName:    tableName,
		Action:       action,
		Timestamp:    time.Now(),
		CurrentValue: string(currentValues),
		PerformedBy:  getPerformedBy(tx),
	}

	result := al.DB.Create(&auditLog)
	if result.Error != nil {
		fmt.Printf("Error creating audit log: %v\n", result.Error)
	}
}

func getPrimaryKeyValue(tx *gorm.DB, record interface{}) interface{} {
	if field := tx.Statement.Schema.PrioritizedPrimaryField; field != nil {
		value, _ := field.ValueOf(tx.Statement.Context, reflect.ValueOf(record))
		return value
	}
	return nil
}

func getAction(tx *gorm.DB) string {
	if tx.Statement.Schema == nil {
		return "UNKNOWN"
	}

	if tx.Statement.SQL.String() != "" && strings.HasPrefix(strings.ToUpper(tx.Statement.SQL.String()), "INSERT") {
		return "INSERT"
	}

	if tx.Statement.SQL.String() != "" && strings.HasPrefix(strings.ToUpper(tx.Statement.SQL.String()), "UPDATE") {
		return "UPDATE"
	}

	if tx.Statement.SQL.String() != "" && strings.HasPrefix(strings.ToUpper(tx.Statement.SQL.String()), "DELETE") {
		return "DELETE"
	}
	switch tx.Statement.ReflectValue.Kind() {
	case reflect.Slice, reflect.Array:
		if tx.Statement.Changed() {
			return "UPDATE"
		}
		return "INSERT"
	default:
		if tx.Statement.Changed() {
			return "UPDATE"
		}
	}

	// Check for delete operation
	if tx.Statement.SQL.String() != "" && strings.HasPrefix(strings.ToUpper(tx.Statement.SQL.String()), "DELETE") {
		return "DELETE"
	}

	// If it's a new record, it's an insert
	if tx.Statement.Schema.PrioritizedPrimaryField != nil {
		_, isZero := tx.Statement.Schema.PrioritizedPrimaryField.ValueOf(tx.Statement.Context, tx.Statement.ReflectValue)
		if isZero {
			return "INSERT"
		}
	}

	// Default to UPDATE if we can't determine otherwise
	return "UPDATE"
}

func getPerformedBy(tx *gorm.DB) string {
	value, ok := tx.Get("performed_by")
	if ok {
		return value.(string)
	}
	if PerformedBy, ok := tx.Get("performed_by"); ok {
		return PerformedBy.(string)
	}
	return "system"
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
	var err error
	switch al.DBType {
	case "mysql":
		err = tx.Exec("SET @performed_by = ?", performedBy).Error
	case "postgres":
		// Use the correct syntax for PostgreSQL
		err = tx.Exec("SELECT set_config('audit.performed_by', $1, true)", performedBy).Error
	default:
		err = fmt.Errorf("SetPerformedBy not implemented for database type: %s", al.DBType)
	}

	if err != nil {
		log.Printf("Error in SetPerformedBy: %v", err)
		tx.AddError(err)
	} else {
		log.Printf("Successfully set performed_by to: %s", performedBy)
	}

	// Set the value in GORM's statement
	return tx.Set("performed_by", performedBy)
}
