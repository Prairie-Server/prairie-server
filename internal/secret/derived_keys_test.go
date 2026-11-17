package secret

import (
	"bytes"
	"testing"
)

func TestBackupMACKeysAreDistinctAndStable(t *testing.T) {
	t.Parallel()

	master := testKey(t)
	manifest, err := BackupManifestKey(master)
	if err != nil {
		t.Fatalf("BackupManifestKey: %v", err)
	}
	journal, err := RestoreJournalKey(master)
	if err != nil {
		t.Fatalf("RestoreJournalKey: %v", err)
	}
	if len(manifest) != 32 || len(journal) != 32 {
		t.Fatalf("key lengths = %d, %d, want 32", len(manifest), len(journal))
	}
	if bytes.Equal(manifest, journal) {
		t.Fatal("manifest and journal keys must be domain-separated")
	}
	again, err := BackupManifestKey(master)
	if err != nil {
		t.Fatalf("BackupManifestKey again: %v", err)
	}
	if !bytes.Equal(manifest, again) {
		t.Fatal("BackupManifestKey should be deterministic")
	}
}

func TestDerivedKeysRejectShortMasterKey(t *testing.T) {
	t.Parallel()

	short := make([]byte, MinMasterKeyLen-1)
	if _, err := BackupManifestKey(short); err == nil {
		t.Error("BackupManifestKey accepted a short key")
	}
	if _, err := RestoreJournalKey(short); err == nil {
		t.Error("RestoreJournalKey accepted a short key")
	}
	if _, err := KeyCheckValue(short); err == nil {
		t.Error("KeyCheckValue accepted a short key")
	}
	if _, err := BackupPassphrase(short); err == nil {
		t.Error("BackupPassphrase accepted a short key")
	}
}

func TestDeriveHMACKeyRejectsNonPositiveLength(t *testing.T) {
	t.Parallel()

	for _, n := range []int{0, -1} {
		if _, err := DeriveHMACKey(testKey(t), "test", n); err == nil {
			t.Errorf("DeriveHMACKey(outLen=%d) succeeded", n)
		}
	}
}
