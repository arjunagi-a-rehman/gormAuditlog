package auditlog

import (
	"fmt"
	"log"

	"gorm.io/gorm"
)

func  createPostgresTriggers(DB *gorm.DB,tableName string) error {

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

	result := DB.Exec(sql)
	if result.Error != nil {
		log.Printf("Error creating PostgreSQL trigger for table %s: %v", tableName, result.Error)
	} else {
		log.Printf("Successfully created PostgreSQL trigger for table %s", tableName)
	}
	return result.Error
}
