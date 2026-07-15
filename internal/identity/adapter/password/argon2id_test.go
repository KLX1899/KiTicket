package password

import "testing"

func TestArgon2id(t *testing.T) {
	hasher := Argon2id{}
	encoded, err := hasher.Hash("a-long-demo-password")
	if err != nil {
		t.Fatal(err)
	}
	valid, err := hasher.Verify("a-long-demo-password", encoded)
	if err != nil || !valid {
		t.Fatalf("expected password to verify: valid=%v err=%v", valid, err)
	}
	valid, err = hasher.Verify("different-password", encoded)
	if err != nil {
		t.Fatal(err)
	}
	if valid {
		t.Fatal("different password verified")
	}
}
