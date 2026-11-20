package auth

import (
	"encoding/base64"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestPasswordHashVerifiesOnlyTheRightPassword(t *testing.T) {
	hash, hashError := HashPassword("correct horse battery")
	if hashError != nil {
		t.Fatal(hashError)
	}
	if !VerifyPassword("correct horse battery", hash) {
		t.Fatal("the original password must verify")
	}
	if VerifyPassword("correct horse batterz", hash) || VerifyPassword("", hash) {
		t.Fatal("a different password must not verify")
	}
	other, _ := HashPassword("correct horse battery")
	if other == hash {
		t.Fatal("two hashes of one password must differ by salt")
	}
	for _, malformed := range []string{"", "plain", "pbkdf2-sha256$x$a$b", "pbkdf2-sha256$600000$!!$!!", "md5$1$a$b", "pbkdf2-sha256$99999999999$AAAA$AAAA"} {
		if VerifyPassword("correct horse battery", malformed) {
			t.Fatalf("malformed hash %q verified", malformed)
		}
	}
}

func TestValidatePassword(t *testing.T) {
	for _, tc := range []struct {
		password string
		valid    bool
	}{
		{password: "short", valid: false},
		{password: "aaaaaaaaaaaaaaaa", valid: false},
		{password: strings.Repeat("ab", 129), valid: false},
		{password: "a-long-enough-passphrase", valid: true},
		{password: "mật khẩu dài đủ để hợp lệ", valid: true},
		{password: "correct horse battery staple", valid: true},
	} {
		if err := ValidatePassword(tc.password); (err == nil) != tc.valid {
			t.Errorf("ValidatePassword(%q) = %v, want valid=%t", tc.password, err, tc.valid)
		}
	}
}

func TestTemporaryPasswordsAreValidAndDistinct(t *testing.T) {
	first, _ := NewTemporaryPassword()
	second, _ := NewTemporaryPassword()
	if first == second || ValidatePassword(first) != nil {
		t.Fatalf("temporary passwords %q and %q must be distinct and acceptable", first, second)
	}
}

func TestRandomTokenHasRequestedEntropyAndIsURLSafe(t *testing.T) {
	token, tokenError := RandomToken(32)
	if tokenError != nil {
		t.Fatal(tokenError)
	}
	decoded, decodeError := base64.RawURLEncoding.DecodeString(token)
	if decodeError != nil || len(decoded) != 32 {
		t.Fatalf("token %q must be 32 bytes of unpadded URL-safe base64", token)
	}
}

func TestRolesGrantScopes(t *testing.T) {
	want := map[string][]string{
		RoleUser:  {"read", "write"},
		RoleOwner: {"admin", "read", "replay", "write"},
	}
	for role, scopes := range want {
		if got := ScopesForRole(role); !slices.Equal(got, scopes) {
			t.Errorf("ScopesForRole(%q) = %v, want %v", role, got, scopes)
		}
	}
	if ScopesForRole("root") != nil || ScopesForRole("admin") != nil || ScopesForRole("") != nil {
		t.Fatal("an unknown role must grant nothing")
	}
	if slices.Contains(ScopesForRole(RoleUser), ScopeAdmin) || slices.Contains(ScopesForRole(RoleUser), ScopeReplay) {
		t.Fatal("a user must not manage the project or replay requests")
	}
}

func TestOnlyOwnersManageMembership(t *testing.T) {
	cases := []struct {
		actor, target string
		want          bool
	}{
		{RoleOwner, RoleOwner, true}, {RoleOwner, RoleUser, true},
		{RoleUser, RoleUser, false}, {RoleUser, RoleOwner, false},
		{RoleOwner, "root", false}, {"", RoleUser, false},
	}
	for _, testCase := range cases {
		if got := CanManageRole(testCase.actor, testCase.target); got != testCase.want {
			t.Errorf("CanManageRole(%q, %q) = %t, want %t", testCase.actor, testCase.target, got, testCase.want)
		}
	}
}

func TestActorLabels(t *testing.T) {
	if (Actor{UserID: "u-1"}).Label() != "user:u-1" || (Actor{}).Label() != "system" {
		t.Fatal("unexpected actor labels")
	}
	if !(Actor{}).IsZero() || (Actor{UserID: "u"}).IsZero() {
		t.Fatal("IsZero must be true only for no actor")
	}
	if got := (Principal{UserID: "u"}).Actor(); got.UserID != "u" {
		t.Fatalf("actor = %+v", got)
	}
}

func TestIngestTokenNameAndExpiry(t *testing.T) {
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	params, err := NormalizeCreateIngestTokenParams(CreateIngestTokenParams{Name: " checkout-api "}, now)
	if err != nil || params.Name != "checkout-api" || !params.ExpiresAt.Equal(now.Add(365*24*time.Hour)) {
		t.Fatalf("params = %+v, error = %v; an unset expiry means one year", params, err)
	}
	for _, bad := range []CreateIngestTokenParams{
		{Name: ""}, {Name: "-lead"}, {Name: "ok", ExpiresAt: now.Add(-time.Hour)}, {Name: "ok", ExpiresAt: now.Add(4 * 365 * 24 * time.Hour)},
	} {
		if _, err := NormalizeCreateIngestTokenParams(bad, now); !errors.Is(err, ErrInvalidIdentity) {
			t.Errorf("%+v: error = %v, want ErrInvalidIdentity", bad, err)
		}
	}
}

func TestNormalizeIdentityInput(t *testing.T) {
	user, err := NormalizeCreateUserParams(CreateUserParams{Email: "  Alice@Example.COM ", DisplayName: " Alice ", Password: "a-long-enough-passphrase"})
	if err != nil || user.Email != "alice@example.com" || user.DisplayName != "Alice" {
		t.Fatalf("user = %+v, error = %v", user, err)
	}
	for _, bad := range []CreateUserParams{
		{Email: "not-an-email", DisplayName: "A", Password: "a-long-enough-passphrase"},
		{Email: "a@b.c", DisplayName: " ", Password: "a-long-enough-passphrase"},
		{Email: "a@b.c", DisplayName: "A", Password: "short"},
	} {
		if _, err := NormalizeCreateUserParams(bad); !errors.Is(err, ErrInvalidIdentity) {
			t.Errorf("%+v: error = %v, want ErrInvalidIdentity", bad, err)
		}
	}
	project, err := NormalizeCreateProjectParams(CreateProjectParams{Slug: " Checkout-API ", Name: " Checkout "})
	if err != nil || project.Slug != "checkout-api" || project.Name != "Checkout" || project.EnvironmentName != "production" {
		t.Fatalf("project = %+v, error = %v", project, err)
	}
	for _, slug := range []string{"", "-lead", "has space", "UPPER_snake", strings.Repeat("a", 64)} {
		if _, err := NormalizeCreateProjectParams(CreateProjectParams{Slug: slug, Name: "n"}); !errors.Is(err, ErrInvalidIdentity) {
			t.Errorf("slug %q: error = %v, want ErrInvalidIdentity", slug, err)
		}
	}
}
