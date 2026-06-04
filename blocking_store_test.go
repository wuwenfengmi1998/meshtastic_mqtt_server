package main

import (
	"errors"
	"testing"

	"gorm.io/gorm"
)

func TestNodeBlockingCRUD(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	nodeNum := int64(305419896)
	rule, err := st.CreateNodeBlocking(" !12345678 ", &nodeNum, " noisy node ", true)
	if err != nil {
		t.Fatalf("CreateNodeBlocking() error = %v", err)
	}
	if rule.NodeID != "!12345678" || rule.NodeNum == nil || *rule.NodeNum != nodeNum || rule.Reason != "noisy node" || !rule.Enabled {
		t.Fatalf("created node rule = %+v, want normalized fields", rule)
	}

	if _, err := st.CreateNodeBlocking("!12345678", nil, "duplicate", true); !errors.Is(err, errBlockingAlreadyExists) {
		t.Fatalf("duplicate CreateNodeBlocking() error = %v, want errBlockingAlreadyExists", err)
	}

	updatedNum := int64(7)
	updated, err := st.UpdateNodeBlocking(rule.ID, "!00000007", &updatedNum, "updated", false)
	if err != nil {
		t.Fatalf("UpdateNodeBlocking() error = %v", err)
	}
	if updated.NodeID != "!00000007" || updated.NodeNum == nil || *updated.NodeNum != updatedNum || updated.Reason != "updated" || updated.Enabled {
		t.Fatalf("updated node rule = %+v, want updated fields", updated)
	}

	rows, err := st.ListNodeBlocking(listOptions{})
	if err != nil {
		t.Fatalf("ListNodeBlocking() error = %v", err)
	}
	if len(rows) != 1 || rows[0].ID != rule.ID {
		t.Fatalf("ListNodeBlocking() = %+v, want one updated rule", rows)
	}
	total, err := st.CountNodeBlocking(listOptions{})
	if err != nil || total != 1 {
		t.Fatalf("CountNodeBlocking() = %d, %v, want 1, nil", total, err)
	}

	if err := st.DeleteNodeBlocking(rule.ID); err != nil {
		t.Fatalf("DeleteNodeBlocking() error = %v", err)
	}
	if err := st.DeleteNodeBlocking(rule.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("DeleteNodeBlocking(missing) error = %v, want record not found", err)
	}
}

func TestNodeBlockingValidation(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	if _, err := st.CreateNodeBlocking("   ", nil, "", true); err == nil {
		t.Fatal("CreateNodeBlocking(empty) error = nil, want error")
	}
	if _, err := st.UpdateNodeBlocking(1, "!missing", nil, "", true); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("UpdateNodeBlocking(missing) error = %v, want record not found", err)
	}
}

func TestIPBlockingCRUDAndValidation(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	rule, err := st.CreateIPBlocking(" 127.0.0.1 ", "local", true)
	if err != nil {
		t.Fatalf("CreateIPBlocking(ip) error = %v", err)
	}
	if rule.IPValue != "127.0.0.1" || rule.Reason != "local" || !rule.Enabled {
		t.Fatalf("created ip rule = %+v, want normalized IP", rule)
	}

	cidr, err := st.CreateIPBlocking("192.168.1.99/24", "cidr", true)
	if err != nil {
		t.Fatalf("CreateIPBlocking(cidr) error = %v", err)
	}
	if cidr.IPValue != "192.168.1.0/24" {
		t.Fatalf("cidr IPValue = %q, want 192.168.1.0/24", cidr.IPValue)
	}

	if _, err := st.CreateIPBlocking("127.0.0.1", "duplicate", true); !errors.Is(err, errBlockingAlreadyExists) {
		t.Fatalf("duplicate CreateIPBlocking() error = %v, want errBlockingAlreadyExists", err)
	}
	if _, err := st.CreateIPBlocking("not-an-ip", "invalid", true); err == nil {
		t.Fatal("CreateIPBlocking(invalid) error = nil, want error")
	}

	updated, err := st.UpdateIPBlocking(rule.ID, "10.0.0.0/8", "updated", false)
	if err != nil {
		t.Fatalf("UpdateIPBlocking() error = %v", err)
	}
	if updated.IPValue != "10.0.0.0/8" || updated.Reason != "updated" || updated.Enabled {
		t.Fatalf("updated ip rule = %+v, want updated fields", updated)
	}

	rows, err := st.ListIPBlocking(listOptions{})
	if err != nil {
		t.Fatalf("ListIPBlocking() error = %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("ListIPBlocking() length = %d, want 2", len(rows))
	}
	total, err := st.CountIPBlocking(listOptions{})
	if err != nil || total != 2 {
		t.Fatalf("CountIPBlocking() = %d, %v, want 2, nil", total, err)
	}

	if err := st.DeleteIPBlocking(rule.ID); err != nil {
		t.Fatalf("DeleteIPBlocking() error = %v", err)
	}
	if err := st.DeleteIPBlocking(rule.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("DeleteIPBlocking(missing) error = %v, want record not found", err)
	}
}

func TestForbiddenWordBlockingCRUDAndValidation(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	rule, err := st.CreateForbiddenWordBlocking(" spam ", "", false, "junk", true)
	if err != nil {
		t.Fatalf("CreateForbiddenWordBlocking() error = %v", err)
	}
	if rule.Word != "spam" || rule.MatchType != forbiddenWordMatchContains || rule.CaseSensitive || rule.Reason != "junk" || !rule.Enabled {
		t.Fatalf("created word rule = %+v, want normalized fields", rule)
	}

	if _, err := st.CreateForbiddenWordBlocking("spam", "contains", false, "duplicate", true); !errors.Is(err, errBlockingAlreadyExists) {
		t.Fatalf("duplicate CreateForbiddenWordBlocking() error = %v, want errBlockingAlreadyExists", err)
	}
	if _, err := st.CreateForbiddenWordBlocking("   ", "contains", false, "empty", true); err == nil {
		t.Fatal("CreateForbiddenWordBlocking(empty) error = nil, want error")
	}
	if _, err := st.CreateForbiddenWordBlocking("regex", "regex", false, "unsupported", true); err == nil {
		t.Fatal("CreateForbiddenWordBlocking(unsupported match type) error = nil, want error")
	}

	updated, err := st.UpdateForbiddenWordBlocking(rule.ID, "Spam", "contains", true, "updated", false)
	if err != nil {
		t.Fatalf("UpdateForbiddenWordBlocking() error = %v", err)
	}
	if updated.Word != "Spam" || updated.MatchType != "contains" || !updated.CaseSensitive || updated.Reason != "updated" || updated.Enabled {
		t.Fatalf("updated word rule = %+v, want updated fields", updated)
	}

	rows, err := st.ListForbiddenWordBlocking(listOptions{})
	if err != nil {
		t.Fatalf("ListForbiddenWordBlocking() error = %v", err)
	}
	if len(rows) != 1 || rows[0].ID != rule.ID {
		t.Fatalf("ListForbiddenWordBlocking() = %+v, want one updated rule", rows)
	}
	total, err := st.CountForbiddenWordBlocking(listOptions{})
	if err != nil || total != 1 {
		t.Fatalf("CountForbiddenWordBlocking() = %d, %v, want 1, nil", total, err)
	}

	if err := st.DeleteForbiddenWordBlocking(rule.ID); err != nil {
		t.Fatalf("DeleteForbiddenWordBlocking() error = %v", err)
	}
	if err := st.DeleteForbiddenWordBlocking(rule.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("DeleteForbiddenWordBlocking(missing) error = %v, want record not found", err)
	}
}
