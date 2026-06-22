package wfmarket

import "testing"

func TestCredentialsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	in := Credentials{Email: "player@example.com", Password: "s3cr3t-p@ss"}
	if err := SaveCredentials(dir, in); err != nil {
		t.Fatal(err)
	}
	out, ok := LoadCredentials(dir)
	if !ok {
		t.Fatal("expected saved credentials")
	}
	if out != in {
		t.Errorf("round-trip mismatch: %+v != %+v", out, in)
	}
}

func TestPasswordNotStoredInPlaintext(t *testing.T) {
	if obfuscate("hunter2") == "hunter2" {
		t.Error("password stored in plain text")
	}
	if deobfuscate(obfuscate("hunter2")) != "hunter2" {
		t.Error("obfuscation not reversible")
	}
}

func TestLoadMissing(t *testing.T) {
	if _, ok := LoadCredentials(t.TempDir()); ok {
		t.Error("expected no credentials in empty dir")
	}
}
