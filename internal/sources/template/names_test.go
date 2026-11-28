package template

import (
	"reflect"
	"testing"
)

func TestNamesListsTagIdentifiersOnce(t *testing.T) {
	got := Names("{cam1.kbps}{if cam2.online and not cam1.online}x{{cam9.state}}{end} {now:hm}")
	want := []string{"cam1.kbps", "cam1.online", "cam2.online", "now"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%v want %v", got, want)
	}
}
