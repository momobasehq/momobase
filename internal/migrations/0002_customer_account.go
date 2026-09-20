package migrations

import "gorm.io/gorm"

// upCustomerAccount renames transactions.customer_phone to customer_account, which
// AutoMigrate cannot express. The guards make it a no-op where the old column never was.
func upCustomerAccount(db *gorm.DB) error {
	migrator := db.Migrator()
	if !migrator.HasTable("transactions") || migrator.HasColumn("transactions", "customer_account") {
		return nil
	}
	if !migrator.HasColumn("transactions", "customer_phone") {
		return nil
	}
	return migrator.RenameColumn("transactions", "customer_phone", "customer_account")
}
