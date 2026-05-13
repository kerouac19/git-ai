package handler

import (
	"testing"

	"git-ai-server/internal/model"
)

func TestUserToSubjectUsesDatabaseOrg(t *testing.T) {
	user := &model.User{
		ID:          "user-1",
		Username:    "alice",
		Email:       "alice@example.com",
		DisplayName: "Alice",
		Role:        "admin",
		OrgID:       "org-1",
		OrgName:     "Engineering",
	}

	subject := userToSubject(user)

	if subject.PersonalOrgID != "org-1" {
		t.Fatalf("PersonalOrgID = %q, want org-1", subject.PersonalOrgID)
	}
	if len(subject.Orgs) != 1 {
		t.Fatalf("len(Orgs) = %d, want 1", len(subject.Orgs))
	}
	if subject.Orgs[0].OrgID != "org-1" {
		t.Fatalf("Orgs[0].OrgID = %q, want org-1", subject.Orgs[0].OrgID)
	}
	if subject.Orgs[0].OrgName != "Engineering" {
		t.Fatalf("Orgs[0].OrgName = %q, want Engineering", subject.Orgs[0].OrgName)
	}
	if subject.Orgs[0].Role != "admin" {
		t.Fatalf("Orgs[0].Role = %q, want admin", subject.Orgs[0].Role)
	}
}
