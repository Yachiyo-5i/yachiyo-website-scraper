package templatex

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestRenderReplacesStringVars(t *testing.T) {
	got, err := Render("/works/{code}?page={page}", map[string]string{
		"code": "SSIS-001",
		"page": "2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "/works/SSIS-001?page=2" {
		t.Fatalf("unexpected render result: %q", got)
	}
}

func TestRenderStringReportsMissingVariables(t *testing.T) {
	_, err := RenderString("/{site}/{code}/{missing}", map[string]interface{}{
		"site": "javbus",
	})
	if err == nil {
		t.Fatal("expected missing variable error")
	}
	message := err.Error()
	if !strings.Contains(message, "code") || !strings.Contains(message, "missing") {
		t.Fatalf("missing variable error should list missing keys, got %q", message)
	}
}

func TestPlaceholdersReturnsUniqueTemplateKeys(t *testing.T) {
	got := Placeholders("/forum.php?fid={fid}&filter={filter}&again={fid}&env={env.TOKEN}")
	want := []string{"fid", "filter", "env.TOKEN"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected placeholders:\nwant: %#v\n got: %#v", want, got)
	}
}

func TestRenderStringReadsEnvironmentVariables(t *testing.T) {
	t.Setenv("SCRAPER_TEST_TOKEN", "secret-token")

	got, err := RenderString("Bearer {env.SCRAPER_TEST_TOKEN}", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Bearer secret-token" {
		t.Fatalf("unexpected env render result: %q", got)
	}
}

func TestRenderStringMissingEnvironmentVariableRendersEmptyString(t *testing.T) {
	const name = "SCRAPER_TEST_TOKEN_EMPTY"
	oldValue, hadValue := os.LookupEnv(name)
	os.Unsetenv(name)
	t.Cleanup(func() {
		if hadValue {
			os.Setenv(name, oldValue)
		} else {
			os.Unsetenv(name)
		}
	})

	got, err := RenderString("Bearer {env.SCRAPER_TEST_TOKEN_EMPTY}", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Bearer " {
		t.Fatalf("unexpected missing env render result: %q", got)
	}
}

func TestRenderAnyRecursivelyRendersObjectsAndLists(t *testing.T) {
	input := map[string]interface{}{
		"title": "{title}",
		"tags":  []interface{}{"{primary}", "static"},
		"meta": map[string]interface{}{
			"count": "{count}",
			"label": "count={count}",
		},
		"unchanged": 42,
	}
	got, err := RenderAny(input, map[string]interface{}{
		"title":   "Example",
		"primary": "drama",
		"count":   3,
	})
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]interface{}{
		"title": "Example",
		"tags":  []interface{}{"drama", "static"},
		"meta": map[string]interface{}{
			"count": 3,
			"label": "count=3",
		},
		"unchanged": 42,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected rendered value:\nwant: %#v\n got: %#v", want, got)
	}
}

func TestRenderAnyStringifiesListsInsideLargerStrings(t *testing.T) {
	got, err := RenderAny("actors={actors}", map[string]interface{}{
		"actors": []string{"Alice", "Bob"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "actors=Alice,Bob" {
		t.Fatalf("unexpected list render result: %#v", got)
	}
}

func TestRenderAnyWholePlaceholderKeepsValueType(t *testing.T) {
	got, err := RenderAny("{count}", map[string]interface{}{"count": 7})
	if err != nil {
		t.Fatal(err)
	}
	if got != 7 {
		t.Fatalf("whole placeholder should keep original type, got %#v (%T)", got, got)
	}
}

func TestRenderAnyWholePlaceholderRequiresVariable(t *testing.T) {
	_, err := RenderAny("{missing}", map[string]interface{}{})
	if err == nil {
		t.Fatal("expected missing variable error")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Fatalf("unexpected error: %v", err)
	}
}
