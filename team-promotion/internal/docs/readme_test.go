package docs

import (
	"os"
	"strings"
	"testing"
)

// Scenario: Shop voucher audit: admin creates a shop voucher (task 3, docs).
// The README audited-actions description must list voucher.admin_create.
func TestReadmeListsVoucherAdminCreateAuditedAction_ShopVoucherAuditAdminCreates(t *testing.T) {
	b, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "voucher.admin_create") {
		t.Fatal("README.md does not list the audited action voucher.admin_create")
	}
}
