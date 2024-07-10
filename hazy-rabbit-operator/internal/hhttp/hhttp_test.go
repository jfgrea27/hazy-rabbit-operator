package hhttp

import (
	"testing"
)

func Test_BuildHost(t *testing.T) {
	want := "aws.com:1234"
	got := BuildHost("aws.com", "1234")

	if got != want {
		t.Fatalf(`Got %v, Wanted %v`, got, want)
	}

	want = "google.com"
	got = BuildHost("google.com", "")
	if got != want {
		t.Fatalf(`Got %v, Wanted %v`, got, want)
	}
}
