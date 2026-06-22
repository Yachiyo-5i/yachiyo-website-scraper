package gfriends

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultCachePathUsesExecutableDirectory(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(filepath.Dir(exe), "cache", "gfriends", "Filetree.json")
	if got := DefaultCachePath(); got != want {
		t.Fatalf("unexpected default cache path:\nwant %s\n got %s", want, got)
	}
}

func TestLookupReturnsImageURLFromCachedFiletree(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "cache", "gfriends", "Filetree.json")
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cachePath, []byte(`{
  "Information": {
    "Timestamp": 1770463399.6879015
  },
  "Content": {
    "0-Hand-Storage": {
      "Alice.jpg": "Alice.jpg?t=1700000000"
    },
    "8-GRAPHIS": {
      "Alice.jpg": "AI-Fix-Alice.jpg?t=1700000001",
      "Bob.jpg": "Bob.jpg?t=1700000002"
    }
  }
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	client := NewClient(Options{
		CachePath: cachePath,
		TTL:       0,
		BaseURL:   "https://cdn.example.test/gh/gfriends/gfriends@master",
	})

	got, ok := client.Lookup(context.Background(), "Alice")
	if !ok {
		t.Fatal("expected Alice to be found")
	}
	want := "https://cdn.example.test/gh/gfriends/gfriends@master/Content/8-GRAPHIS/AI-Fix-Alice.jpg?t=1700000001"
	if got != want {
		t.Fatalf("unexpected image url:\nwant %s\n got %s", want, got)
	}
}

func TestLookupFallsBackToStaleCacheWhenDownloadFails(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "cache", "gfriends", "Filetree.json")
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cachePath, []byte(`{
  "Information": {
    "Timestamp": 1
  },
  "Content": {
    "0-Hand-Storage": {
      "Alice.jpg": "Alice.jpg?t=1700000000"
    }
  }
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	client := NewClient(Options{
		CachePath: cachePath,
		TTL:       -1,
		IndexURL:  "http://127.0.0.1:1/Filetree.json",
		BaseURL:   "https://cdn.example.test/gh/gfriends/gfriends@master",
	})

	got, ok := client.Lookup(context.Background(), "Alice")
	if !ok {
		t.Fatal("expected stale cache to be used")
	}
	want := "https://cdn.example.test/gh/gfriends/gfriends@master/Content/0-Hand-Storage/Alice.jpg?t=1700000000"
	if got != want {
		t.Fatalf("unexpected image url:\nwant %s\n got %s", want, got)
	}
}

func TestLookupDoesNotRetryFailedLoadWithinProcess(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(Options{
		CachePath: filepath.Join(t.TempDir(), "cache", "gfriends", "Filetree.json"),
		IndexURL:  server.URL + "/Filetree.json",
		BaseURL:   "https://cdn.example.test/gh/gfriends/gfriends@master",
	})

	if _, ok := client.Lookup(context.Background(), "Alice"); ok {
		t.Fatal("expected first lookup to miss")
	}
	if _, ok := client.Lookup(context.Background(), "Bob"); ok {
		t.Fatal("expected second lookup to miss")
	}
	if requests != 1 {
		t.Fatalf("expected one failed download attempt, got %d", requests)
	}
}
