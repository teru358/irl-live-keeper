package sources

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"slices"
	"testing"

	"github.com/teru358/irl-live-keeper/internal/sources/template"
)

func TestHTMLCases(t *testing.T) {
	var table struct {
		Defs  map[string]struct{ Type, Format string }
		Cases []struct {
			ID, Mode, Input, Expect string
			Values                  map[string]any
			HTML                    *string
			Refs                    []string
			Warnings                []string
			Error                   struct {
				Code         string
				Line, Column int
			}
		}
	}
	b, e := os.ReadFile("html_cases.json")
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(b, &table); e != nil {
		t.Fatal(e)
	}
	defs := map[string]template.Definition{}
	types := map[string]template.Type{"number": template.Number, "string": template.String, "bool": template.Bool, "seconds": template.Seconds, "clock": template.Clock}
	for name, d := range table.Defs {
		defs[name] = template.Definition{Type: types[d.Type], Format: d.Format}
	}
	for _, c := range table.Cases {
		t.Run(c.ID, func(t *testing.T) {
			var p *HTMLProgram
			var err error
			if c.Mode == "save" {
				p, err = CompileHTML(c.Input, defs)
			} else {
				err = ValidateHTML(c.Input, c.Mode)
			}
			if c.Expect == "error" {
				var te *template.Error
				if !errors.As(err, &te) {
					t.Fatalf("expected %s; got %v", c.Error.Code, err)
				}
				if te.Message.Code != c.Error.Code {
					t.Fatalf("code %s want %s", te.Message.Code, c.Error.Code)
				}
				if c.Mode == "save" && (te.Line != c.Error.Line || te.Column != c.Error.Column) {
					t.Fatalf("position %d:%d want %d:%d", te.Line, te.Column, c.Error.Line, c.Error.Column)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if p == nil {
				return
			}
			slices.Sort(c.Refs)
			if got := p.AssetReferences(); !slices.Equal(got, c.Refs) {
				t.Fatalf("refs %v want %v", got, c.Refs)
			}
			if c.HTML != nil {
				values := map[string]template.Value{}
				for name, raw := range c.Values {
					v := template.Value{Type: defs[name].Type, Defined: true}
					switch x := raw.(type) {
					case string:
						v.String = x
					case bool:
						v.Bool = x
					case float64:
						v.Number = x
					}
					values[name] = v
				}
				if got := p.Render(values); got != *c.HTML {
					t.Fatalf("HTML %q want %q", got, *c.HTML)
				}
			}
			if c.Warnings != nil {
				available := map[string]bool{}
				for _, r := range c.Refs {
					if r != "/source/files/missing.png" {
						available[r] = true
					}
				}
				if got := p.Warnings(available); !reflect.DeepEqual(got, c.Warnings) {
					t.Fatalf("warnings %v want %v", got, c.Warnings)
				}
			}
		})
	}
}

func TestHTMLRegressionPositions(t *testing.T) {
	for _, c := range []struct {
		input, code string
		column      int
	}{
		{"<style>日本語{}</style>{missing}", "template.unknown", 22},
		{"<span style=\"width:{n\x00}\"></span>", "template.html_nul", 22},
		{"<style>x{a:url('#x'/*\\*/)}</style>", "template.css_escape", 22},
	} {
		t.Run(c.code, func(t *testing.T) {
			_, err := CompileHTML(c.input, map[string]template.Definition{"n": {Type: template.Number}})
			var te *template.Error
			if !errors.As(err, &te) || te.Message.Code != c.code || te.Column != c.column {
				t.Fatalf("got %#v want %s at %d", err, c.code, c.column)
			}
		})
	}
}

func TestHTMLNumericReferenceRequiresDigits(t *testing.T) {
	for _, input := range []string{"<img src=\"&#+65;\">", "<img src=\"&#x+41;\">"} {
		err := ValidateHTML(input, "rendered")
		var te *template.Error
		if !errors.As(err, &te) || te.Message.Code != "template.html_lex" {
			t.Fatalf("got %v for %q", err, input)
		}
	}
}

func TestHTMLTemplateErrorBeforeLaterHTML(t *testing.T) {
	_, err := CompileHTML("{missing}<script></script>", nil)
	var te *template.Error
	if !errors.As(err, &te) || te.Message.Code != "template.unknown" || te.Column != 2 {
		t.Fatalf("got %#v", err)
	}
}

func TestHTMLBranchCannotEnterAttribute(t *testing.T) {
	_, err := CompileHTML("{if cam1.online}<span class=\"{end}\"></span>", map[string]template.Definition{"cam1.online": {Type: template.Bool}})
	var te *template.Error
	if !errors.As(err, &te) || te.Message.Code != "template.html_branch" || te.Column != 1 {
		t.Fatalf("got %#v", err)
	}
}

func TestHTMLNULBeforeUnclosedLiteralEOF(t *testing.T) {
	for _, input := range []string{"<!--\x00", "<style>\x00"} {
		err := ValidateHTML(input, "rendered")
		var te *template.Error
		if !errors.As(err, &te) || te.Message.Code != "template.html_nul" {
			t.Fatalf("%q: %v", input, err)
		}
	}
}

func TestHTMLTemplatePositionAndConditionPriority(t *testing.T) {
	for _, c := range []struct{ input, code string }{
		{"<table>{missing}</table>", "template.html_nesting"},
		{"{if missing}<script></script>{end}", "template.unknown"},
	} {
		_, err := CompileHTML(c.input, nil)
		var te *template.Error
		if !errors.As(err, &te) || te.Message.Code != c.code {
			t.Fatalf("%q: %v", c.input, err)
		}
	}
}
