package github

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fakeGh = `#!/bin/sh
echo "$@" >> "$GH_TEST_DIR/calls"
if [ -f "$GH_TEST_DIR/fail" ]; then
  echo "HTTP 404: Not Found (https://api.github.com/repos/x/y/hooks)" >&2
  exit 1
fi
case "$*" in
  *--paginate*) /bin/cat "$GH_TEST_DIR/hooks.json" ;;
  *"-X POST"*) /bin/cat > "$GH_TEST_DIR/post_body" ;;
esac
`

type fake struct{ dir string }

func newFake(t *testing.T, hooks string) fake {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	os.Mkdir(bin, 0755)
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(fakeGh), 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "hooks.json"), []byte(hooks), 0644)
	t.Setenv("PATH", bin)
	t.Setenv("GH_TEST_DIR", dir)
	return fake{dir}
}

func (f fake) calls() []string {
	data, _ := os.ReadFile(filepath.Join(f.dir, "calls"))
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

func (f fake) called(part string) bool {
	for _, c := range f.calls() {
		if strings.Contains(c, part) {
			return true
		}
	}
	return false
}

func TestAddHookCreatesWebhook(t *testing.T) {
	f := newFake(t, `[{"id":7,"config":{"url":"https://other.example.com/"}}]`)

	if err := AddHook("hope61/app", "https://hook.example.com/", "s3cret"); err != nil {
		t.Fatal(err)
	}

	if !f.called("api --paginate repos/hope61/app/hooks") || !f.called("api -X POST repos/hope61/app/hooks --input -") {
		t.Fatalf("unexpected gh calls: %v", f.calls())
	}
	if f.called("s3cret") {
		t.Fatal("the secret must not be passed as an argument")
	}

	var body struct {
		Name   string            `json:"name"`
		Active bool              `json:"active"`
		Events []string          `json:"events"`
		Config map[string]string `json:"config"`
	}
	data, _ := os.ReadFile(filepath.Join(f.dir, "post_body"))
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatalf("bad body %q: %v", data, err)
	}
	if body.Name != "web" || !body.Active || len(body.Events) != 1 || body.Events[0] != "push" ||
		body.Config["url"] != "https://hook.example.com/" || body.Config["secret"] != "s3cret" || body.Config["content_type"] != "json" {
		t.Fatalf("wrong webhook body: %+v", body)
	}
}

func TestAddHookSkipsExisting(t *testing.T) {
	f := newFake(t, `[{"id":7,"config":{"url":"https://hook.example.com/"}}]`)

	err := AddHook("hope61/app", "https://hook.example.com/", "s3cret")
	if err == nil || !strings.Contains(err.Error(), "hope61/app already has a webhook for https://hook.example.com/") {
		t.Fatalf("got %v", err)
	}
	if f.called("POST") {
		t.Fatal("a duplicate webhook was created")
	}
}

func TestRemoveHookAcrossPages(t *testing.T) {
	f := newFake(t, `[{"id":1,"config":{"url":"https://hook.example.com/"}},{"id":2,"config":{"url":"https://other.example.com/"}}]
[{"id":3,"config":{"url":"https://hook.example.com/"}}]`)

	if err := RemoveHook("hope61/app", "https://hook.example.com/"); err != nil {
		t.Fatal(err)
	}
	if !f.called("api -X DELETE repos/hope61/app/hooks/1") || !f.called("api -X DELETE repos/hope61/app/hooks/3") {
		t.Fatalf("matching hooks not deleted: %v", f.calls())
	}
	if f.called("hooks/2") {
		t.Fatal("a webhook with another url was deleted")
	}
}

func TestRemoveHookWhenNone(t *testing.T) {
	f := newFake(t, `[]`)

	err := RemoveHook("hope61/app", "https://hook.example.com/")
	if err == nil || !strings.Contains(err.Error(), "hope61/app has no webhook for https://hook.example.com/") {
		t.Fatalf("got %v", err)
	}
	if f.called("DELETE") {
		t.Fatal("nothing should be deleted")
	}
}

func TestGhErrors(t *testing.T) {
	t.Run("gh fails", func(t *testing.T) {
		f := newFake(t, `[]`)
		os.WriteFile(filepath.Join(f.dir, "fail"), nil, 0644)
		err := AddHook("hope61/app", "https://hook.example.com/", "s3cret")
		if err == nil || !strings.Contains(err.Error(), "github: HTTP 404: Not Found") {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("bad json", func(t *testing.T) {
		newFake(t, `not json`)
		err := RemoveHook("hope61/app", "https://hook.example.com/")
		if err == nil || !strings.Contains(err.Error(), "unexpected response from github") {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("gh not installed", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		err := AddHook("hope61/app", "https://hook.example.com/", "s3cret")
		if err == nil || !strings.Contains(err.Error(), "gh CLI not found") {
			t.Fatalf("got %v", err)
		}
	})
}
