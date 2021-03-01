package config

import (
	"os"
	"testing"
)

func TestLoadConfigs(t *testing.T) {
	os.Setenv("WWC_PORT", "1234")
	have := loadConfigs()
	want := Configuration{
		Port: "1234",
	}

	if have != want {
		t.Errorf("have %#v, want %#v", have, want)
	}
}
