package rabbitclient

import (
	"reflect"
	"testing"
)

func Test_getListQueuesToDelete(t *testing.T) {
	curr := map[string]string{"foo": "foo", "bar": "bar", "fuzz": "fuzz"}
	wanted := map[string]string{"baz": "baz", "fuzz": "fuzz"}

	want := []string{"foo", "bar"}

	got := getListQueuesToDelete(curr, wanted)

	if !reflect.DeepEqual(got, want) {
		t.Fatalf(`Got %v, Wanted %v`, got, want)
	}
}
