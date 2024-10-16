# gormAuditlog

gormAuditlog is a Go module that provides audit logging functionality for GORM-based applications. It automatically tracks changes to your database tables, recording inserts, updates, and deletes in a separate audit log table.

## Features

- Automatic audit logging for GORM models
- Supports MySQL and PostgreSQL databases
- Tracks INSERT, UPDATE, and DELETE operations
- Records the user who performed each action
- Easy integration with existing GORM-based applications
- Customizable tracked tables

## Installation

To install gormAuditlog, use `go get`:

```bash
go get github.com/arjunagi-a-rehman/gormAuditlog
```

## Usage

Here's a quick example of how to use gormAuditlog in your application:

```go
import (
    "github.com/arjunagi-a-rehman/gormAuditlog"
    "gorm.io/gorm"
)

// Your GORM model
type User struct {
    gorm.Model
    Name  string
    Email string
}

func main() {
    // Initialize your GORM database connection
    db, err := gorm.Open(mysql.Open("dsn"), &gorm.Config{})
    if err != nil {
        panic("failed to connect database")
    }

    // Create a new AuditLogger
    auditLogger, err := auditlog.NewAuditLogger(db, &User{})
    if err != nil {
        panic(err)
    }

    // Create the audit log table
    err = auditLogger.CreateAuditLogTable()
    if err != nil {
        panic(err)
    }

    // Create database triggers
    err = auditLogger.CreateTriggers()
    if err != nil {
        panic(err)
    }

    // Use the audit logger in your operations
    db.Create(&User{Name: "John Doe", Email: "john@example.com"})
}
```

### Setting the Performer of an Action

To record who performed an action:

```go
auditLogger.SetPerformedBy(db, "user123")
db.Create(&User{Name: "Jane Doe", Email: "jane@example.com"})
```

## Configuration

You can add or remove tracked tables dynamically:

```go
auditLogger.AddTrackedTable("new_table")
auditLogger.RemoveTrackedTable("old_table")
```

## Audit Log Structure

The audit log table has the following structure:

- `ID`: Unique identifier for the log entry
- `RecordID`: ID of the affected record
- `TableName`: Name of the table that was modified
- `Action`: The type of action (INSERT, UPDATE, DELETE)
- `Timestamp`: When the action occurred
- `CurrentValue`: JSON representation of the current state of the record
- `PerformedBy`: Identifier of the user who performed the action

## Contributing

Contributions to gormAuditlog are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- Thanks to the GORM team for their excellent ORM library.
- Inspired by the need for easy audit logging in Go applications.
