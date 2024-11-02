package auditlog

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

func  createMySQLTriggers(DB *gorm.DB, tableName string) error {
	// Get table columns
	var columns []struct {
		Field string
	}
	err := DB.Raw("SHOW COLUMNS FROM " + tableName).Scan(&columns).Error
	if err != nil {
		return fmt.Errorf("failed to get columns for table %s: %w", tableName, err)
	}

	// Generate JSON_OBJECT string for columns
	jsonObjectParts := make([]string, len(columns))
	for i, col := range columns {
		jsonObjectParts[i] = fmt.Sprintf("'%s', NEW.%s", col.Field, col.Field)
	}
	jsonObjectStr := strings.Join(jsonObjectParts, ", ")

	// INSERT trigger
	insertTrigger := fmt.Sprintf(`
		CREATE TRIGGER %s_insert_trigger
		AFTER INSERT ON %s
		FOR EACH ROW
		BEGIN
			INSERT INTO audit_logs (record_id, table_name, action, timestamp, current_value, performed_by)
			VALUES (
				CAST(NEW.id AS CHAR),
				'%s',
				'INSERT',
				NOW(),
				JSON_OBJECT(%s),
				IFNULL(@performed_by, 'system')
			);
		END;
	`, tableName, tableName, tableName, jsonObjectStr)

	// UPDATE trigger
	updateTrigger := fmt.Sprintf(`
		CREATE TRIGGER %s_update_trigger
		AFTER UPDATE ON %s
		FOR EACH ROW
		BEGIN
			INSERT INTO audit_logs (record_id, table_name, action, timestamp, current_value, performed_by)
			VALUES (
				CAST(NEW.id AS CHAR),
				'%s',
				'UPDATE',
				NOW(),
				JSON_OBJECT(%s),
				IFNULL(@performed_by, 'system')
			);
		END;
	`, tableName, tableName, tableName, jsonObjectStr)

	// DELETE trigger
	deleteTrigger := fmt.Sprintf(`
		CREATE TRIGGER %s_delete_trigger
		BEFORE DELETE ON %s
		FOR EACH ROW
		BEGIN
			INSERT INTO audit_logs (record_id, table_name, action, timestamp, current_value, performed_by)
			VALUES (
				CAST(OLD.id AS CHAR),
				'%s',
				'DELETE',
				NOW(),
				JSON_OBJECT(%s),
				IFNULL(@performed_by, 'system')
			);
		END;
	`, tableName, tableName, tableName, strings.ReplaceAll(jsonObjectStr, "NEW.", "OLD."))

	// Execute each trigger creation
	if err := DB.Exec(insertTrigger).Error; err != nil {
		return fmt.Errorf("failed to create INSERT trigger: %w", err)
	}
	if err := DB.Exec(updateTrigger).Error; err != nil {
		return fmt.Errorf("failed to create UPDATE trigger: %w", err)
	}
	if err := DB.Exec(deleteTrigger).Error; err != nil {
		return fmt.Errorf("failed to create DELETE trigger: %w", err)
	}

	return nil
}