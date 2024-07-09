package rabbitclient

import (
	"reflect"
	"testing"
)

func Test_buildConsoleEnpoint(t *testing.T) {
	t.Setenv("RABBIT_HOST", "aws.com")
	t.Setenv("RABBIT_CONSOLE_PORT", "1234")

	want := "aws.com:1234"
	got := buildConsoleEndpoint()

	if got != want {
		t.Fatalf(`Got %v, Wanted %v`, got, want)
	}
}

func Test_buildRabbitEndpoint(t *testing.T) {
	t.Setenv("RABBIT_HOST", "aws.com")
	t.Setenv("RABBIT_PORT", "1234")

	want := "aws.com:1234"
	got := buildRabbitEndpoint()

	if got != want {
		t.Fatalf(`Got %v, Wanted %v`, got, want)
	}
}

func Test_buildAdminUser(t *testing.T) {
	t.Setenv("RABBIT_ADMIN_USERNAME", "admin")
	t.Setenv("RABBIT_ADMIN_PASSWORD", "password")

	want := RabbitUser{
		Username: "admin",
		Password: "password",
	}

	got := buildAdminUser()

	if got != want {
		t.Fatalf(`Got %v, Wanted %v`, got, want)
	}
}

func Test_getListQueuesToDelete(t *testing.T) {
	curr := map[string]string{"foo": "foo", "bar": "bar", "fuzz": "fuzz"}
	wanted := map[string]string{"baz": "baz", "fuzz": "fuzz"}

	want := []string{"foo", "bar"}

	got := getListQueuesToDelete(curr, wanted)

	if !reflect.DeepEqual(got, want) {
		t.Fatalf(`Got %v, Wanted %v`, got, want)
	}
}
