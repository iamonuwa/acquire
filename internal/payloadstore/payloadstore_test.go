package payloadstore

import (
	"context"
	"testing"
)

var _ Store = (*R2)(nil)

func TestKeys(t *testing.T) {
	if got := RawKey("nlpdp-sa-criteria", "abc"); got != "raw/nlpdp-sa-criteria/abc" {
		t.Errorf("RawKey = %q", got)
	}
	if got := NormKey("nlpdp-sa-criteria", "abc"); got != "norm/nlpdp-sa-criteria/abc" {
		t.Errorf("NormKey = %q", got)
	}
}

func TestRequireSecrets(t *testing.T) {
	for _, k := range requiredEnv {
		t.Setenv(k, "")
	}
	if err := RequireSecrets(); err == nil {
		t.Error("expected error when R2_* env is unset")
	}
	for _, k := range requiredEnv {
		t.Setenv(k, "x")
	}
	if err := RequireSecrets(); err != nil {
		t.Errorf("all env set, got %v", err)
	}
}

type memStore struct{ objects map[string][]byte }

func (m *memStore) Put(_ context.Context, key string, body []byte) error {
	m.objects[key] = append([]byte(nil), body...)
	return nil
}

func TestStore_RerunWritesIdenticalKey(t *testing.T) {
	var s Store = &memStore{objects: map[string][]byte{}}
	ctx := context.Background()
	key := RawKey("src", "hash123")
	if err := s.Put(ctx, key, []byte("payload")); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(ctx, key, []byte("payload")); err != nil {
		t.Fatal(err)
	}
	ms := s.(*memStore)
	if len(ms.objects) != 1 {
		t.Errorf("rerun created %d objects, want 1 (content-addressed key is stable)", len(ms.objects))
	}
	if string(ms.objects[key]) != "payload" {
		t.Errorf("stored %q, want payload", ms.objects[key])
	}
}
