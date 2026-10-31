package goai

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestRepairJSON_UnicodeEscapeBackslashParity(t *testing.T) {
	for backslashes := 0; backslashes <= 6; backslashes++ {
		for digits := 0; digits <= 4; digits++ {
			t.Run(fmt.Sprintf("slashes_%d_digits_%d", backslashes, digits), func(t *testing.T) {
				hex := "1234"[:digits]
				input := `{"value":"prefix` + strings.Repeat(`\`, backslashes) + "u" + hex
				want := "prefix" + strings.Repeat(`\`, backslashes/2)
				if backslashes%2 == 0 {
					want += "u" + hex
				} else if digits == 4 {
					want += "\u1234"
				}

				var got map[string]string
				if err := json.Unmarshal([]byte(repairJSON(input)), &got); err != nil {
					t.Fatalf("repairJSON(%q) produced invalid JSON: %v", input, err)
				}
				if got["value"] != want {
					t.Errorf("partial value = %q, want %q", got["value"], want)
				}
			})
		}
	}
}
