package migrations

import "gorm.io/gorm"

// upCustomerAccount renames transactions.customer_phone to customer_account.
//
// The column holds whatever identifier a payment moves through, which is a mobile
// number only for mobile-money rails; a bank account, card token, or wallet
// address is equally valid. AutoMigrate cannot express a rename — it would add
// the new column and leave the old one behind holding every historical value —
// so the change is made here, before convergence widens the column.
//
// The guards make this a no-op on a database that never had the old column,
// which is what lets a fresh install and an upgrade run the same code path.
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
