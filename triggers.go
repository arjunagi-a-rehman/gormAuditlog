package auditlog

import (
	"fmt"
)
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
	switch al.DBType {
	case "mysql":
		return createMySQLTriggers(al.DB, tableName)
	case "postgres":
		return createPostgresTriggers(al.DB, tableName)
	}
	return fmt.Errorf("unsupported database type: %s", al.DBType)
}

