package downloader

import (
	"path/filepath"
	"testing"
	"time"
)

func TestAcquireLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".1.22.0.lock")

	release, err := Acquire(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}

	// A second acquisition has to wait for the first one to be released.
	done := make(chan error, 1)
	go func() {
		rel, err := Acquire(path, 5*time.Second)
		if err == nil {
			rel()
		}
		done <- err
	}()

	select {
	case err := <-done:
		t.Fatalf("the lock was handed out twice (%v)", err)
	case <-time.After(300 * time.Millisecond):
	}

	release()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Acquire: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the lock was not handed out after it was released")
	}
}

func TestAcquireLockTimeout(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".1.22.0.lock")
	release, err := Acquire(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	if _, err := Acquire(path, 200*time.Millisecond); err == nil {
		t.Error("Acquire did not time out")
	}
}
