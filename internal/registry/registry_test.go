package registry

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/PHPDevsr/opsweave/internal/workflow"
)

const fixture = "apiVersion: opsweave.io/v1\nkind: Workflow\nmetadata: {name: example}\nspec:\n  steps:\n    - id: first\n      uses: test.echo\n      with: {message: hello}\n"

func fixtureCatalog() Catalog {
	return Catalog{"test.echo": {ID: "test.echo", APIVersion: "opsweave.io/v1", Version: "1", Inputs: Field{Type: "object", Properties: map[string]Field{"message": {Type: "string", Required: true}}}, Outputs: map[string]Field{"body": {Type: "string"}}}}
}

func TestPrepareCatalogValidation(t *testing.T) {
	t.Parallel()
	prepared, err := Prepare(t.Context(), []Source{{[]byte(fixture)}}, fixtureCatalog())
	if err != nil || len(prepared) != 1 || prepared[0].Name != "example" || prepared[0].Bindings["test.echo"] != "1" {
		t.Fatalf("prepare = %+v, %v", prepared, err)
	}
	again, err := Prepare(t.Context(), []Source{{[]byte(fixture)}}, fixtureCatalog())
	if err != nil || !reflect.DeepEqual(again, prepared) {
		t.Fatal("normalization is not deterministic")
	}
	if !strings.Contains(string(prepared[0].Normalized), `"timeout":"1h0m0s"`) {
		t.Fatal("duration normalization missing")
	}
	for _, source := range []string{
		strings.Replace(fixture, "message: hello", "message: 42", 1),
		strings.Replace(fixture, "message: hello", "other: secret-marker", 1),
		strings.Replace(fixture, "message: hello", "", 1),
		fixture + "      if: 'true'\n",
		"[secret-marker",
	} {
		if _, err := Prepare(t.Context(), []Source{{[]byte(source)}}, fixtureCatalog()); err == nil || strings.Contains(err.Error(), "secret-marker") {
			t.Fatalf("unsafe or missing error = %v", err)
		}
	}
	if _, err := Prepare(t.Context(), []Source{{[]byte(fixture)}}, nil); err == nil {
		t.Fatal("empty production catalog accepted")
	}
	if _, err := Prepare(t.Context(), []Source{{[]byte(fixture)}, {[]byte(fixture)}}, fixtureCatalog()); err == nil {
		t.Fatal("duplicate workflow name accepted")
	}
	for _, sources := range [][]Source{nil, make([]Source, 257), {{make([]byte, (16<<20)+1)}}} {
		if _, err := Prepare(t.Context(), sources, fixtureCatalog()); err == nil {
			t.Fatal("invalid batch accepted")
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Prepare(ctx, []Source{{[]byte(fixture)}}, fixtureCatalog()); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestCredentialsAndDescriptors(t *testing.T) {
	t.Parallel()
	for _, mutation := range []func(*Descriptor){
		func(d *Descriptor) { d.Version = "" },
		func(d *Descriptor) { d.APIVersion = "future" },
		func(d *Descriptor) { d.ID = "other.action" },
		func(d *Descriptor) { d.Capabilities = []string{"unsupported"} },
		func(d *Descriptor) { d.Inputs.Type = "unknown" },
		func(d *Descriptor) { d.Outputs["body"] = Field{Type: "unknown"} },
		func(d *Descriptor) { d.Inputs.Properties["message"] = Field{Type: "string", Credential: true} },
	} {
		catalog := fixtureCatalog()
		d := catalog["test.echo"]
		mutation(&d)
		catalog["test.echo"] = d
		if _, err := Prepare(t.Context(), []Source{{[]byte(fixture)}}, catalog); err == nil {
			t.Fatal("invalid descriptor or inline credential accepted")
		}
	}
	catalog := fixtureCatalog()
	d := catalog["test.echo"]
	d.Inputs.Properties["message"] = Field{Type: "string", Credential: true}
	catalog["test.echo"] = d
	secret := strings.Replace(fixture, "message: hello", "message: {$ref: secrets.prod.token}", 1)
	if _, err := Prepare(t.Context(), []Source{{[]byte(secret)}}, catalog); err == nil {
		t.Fatal("secret capability enabled early")
	}
	output := fixture + "    - id: next\n      uses: test.echo\n      with: {message: {$ref: steps.first.outputs.body}}\n"
	if _, err := Prepare(t.Context(), []Source{{[]byte(output)}}, fixtureCatalog()); err == nil {
		t.Fatal("output capability enabled early")
	}
	if _, err := Prepare(t.Context(), []Source{{[]byte(strings.Replace(output, "outputs.body", "outputs.absent", 1))}}, fixtureCatalog()); err == nil {
		t.Fatal("missing output accepted")
	}
}

func TestReadSourcesOrderingAndLimits(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for name, data := range map[string]string{"b.yml": "b", "a.yaml": "a", "ignored.txt": "ignored"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "nested.yaml"), 0700); err != nil {
		t.Fatal(err)
	}
	sources, err := ReadSources(t.Context(), dir)
	if err != nil || len(sources) != 2 || string(sources[0].Data) != "a" || string(sources[1].Data) != "b" {
		t.Fatalf("sources = %+v, %v", sources, err)
	}
	if _, err := ReadSources(t.Context(), t.TempDir()); err == nil {
		t.Fatal("empty directory accepted")
	}
	if _, err := ReadSources(t.Context(), filepath.Join(dir, "missing")); err == nil {
		t.Fatal("missing input accepted")
	}
	if err := os.WriteFile(filepath.Join(dir, "large.yaml"), []byte(strings.Repeat("x", (1<<20)+1)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadSources(t.Context(), dir); err == nil {
		t.Fatal("oversized input accepted")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := ReadSources(ctx, dir); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestSchemaTypes(t *testing.T) {
	for _, test := range []struct {
		value any
		field Field
		want  bool
	}{
		{true, Field{Type: "boolean"}, true}, {"true", Field{Type: "boolean"}, false},
		{int64(1), Field{Type: "integer"}, true}, {1.2, Field{Type: "integer"}, false},
		{1.2, Field{Type: "number"}, true}, {int64(1), Field{Type: "number"}, true}, {"1", Field{Type: "number"}, false},
		{[]any{"a", "b"}, Field{Type: "array", Items: &Field{Type: "string"}}, true},
		{[]any{true}, Field{Type: "array", Items: &Field{Type: "string"}}, false},
		{"wrong", Field{Type: "object"}, false}, {true, Field{Type: "array", Items: &Field{Type: "string"}}, false},
	} {
		if got := matches(test.value, test.field, nil, 0); got != test.want {
			t.Fatalf("schema matching = %v, want %v", got, test.want)
		}
	}
	cycle := &Field{Type: "array"}
	cycle.Items = cycle
	if schemaValid(*cycle, 0) {
		t.Fatal("cyclic catalog schema accepted")
	}
}

func TestSensitiveOutputReferences(t *testing.T) {
	field := Field{Type: "object", Properties: map[string]Field{"token": {Type: "string", Sensitive: true}}}
	steps := map[string]Descriptor{"first": {Outputs: map[string]Field{"body": field}}}
	ref := workflow.Reference{Kind: "output", Step: "first", Output: "body"}
	if matches(ref, field, steps, 0) {
		t.Fatal("nested sensitive output could bypass credential rules")
	}
	if matches(ref, Field{Type: "string", Credential: true}, steps, 0) {
		t.Fatal("output could supply a credential")
	}
}

func TestNormalizationPreservesReferenceObjects(t *testing.T) {
	source := fixture + "    - id: next\n      uses: test.echo\n      with: {values: [{$ref: steps.first.outputs.body}, {$ref: secrets.prod.token}]}\n"
	d, err := workflow.Parse(t.Context(), []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	data, err := Normalize(d)
	if err != nil || !strings.Contains(string(data), `"$ref":"steps.first.outputs.body"`) || !strings.Contains(string(data), `"$ref":"secrets.prod.token"`) {
		t.Fatalf("normalized references = %s, %v", data, err)
	}
}

func TestReadSourcesRejectsSymlinks(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "target.yaml")
	if err := os.WriteFile(target, []byte(fixture), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.yaml")
	if err := os.Symlink(target, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("Windows does not permit test symlink creation: %v", err)
		}
		t.Fatalf("create test symlink: %v", err)
	}
	for _, path := range []string{link, dir} {
		if _, err := ReadSources(t.Context(), path); err == nil {
			t.Fatal("workflow symlink followed")
		}
	}
}

func FuzzPrepare(f *testing.F) {
	for _, source := range []string{fixture, "[]", fixture + "      retry: {attempts: 2}\n"} {
		f.Add([]byte(source))
	}
	f.Fuzz(func(t *testing.T, source []byte) {
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		defer cancel()
		result, err := Prepare(ctx, []Source{{source}}, fixtureCatalog())
		if err != nil {
			if len(err.Error()) > 200 {
				t.Fatal("unbounded validation error")
			}
			return
		}
		if len(result) != 1 || result[0].Bindings["test.echo"] != "1" {
			t.Fatal("preflight returned an unbound candidate")
		}
	})
}
