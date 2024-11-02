package auditlog

import (
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"strings"

	"gorm.io/gorm"
)




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

